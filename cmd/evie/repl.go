// The terminal frontend: an agent.Events implementation over the smooth
// printer plus the y/N approval gate. All terminal I/O for the agent
// lives here — the loop itself is internal/agent and never prints.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/tools"
)

// smoothPrinter decouples token arrival from display so bursty network
// chunks render as steady typing. Deltas go into a channel; a printer
// goroutine drains them into a buffer and prints a few characters per
// tick — more when the buffer is deep, so it catches up instead of
// lagging. Call the returned onDelta from the stream, then done() to
// flush the tail and stop the printer.
func smoothPrinter() (onDelta func(string), done func()) {
	ch := make(chan string, 64)
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		var buf []rune
		ticker := time.NewTicker(12 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case s, ok := <-ch:
				if !ok {
					fmt.Print(string(buf))
					return
				}
				buf = append(buf, []rune(s)...)
			case <-ticker.C:
				if len(buf) == 0 {
					continue
				}
				n := 1 + len(buf)/20
				if n > len(buf) {
					n = len(buf)
				}
				fmt.Print(string(buf[:n]))
				buf = buf[n:]
			}
		}
	}()

	return func(s string) { ch <- s },
		func() { close(ch); <-finished }
}

// replEvents renders a turn to the terminal. A smooth printer is started
// lazily on the first delta of each assistant message and flushed when
// the message completes, so pacing never leaks across messages.
type replEvents struct {
	deltaIn func(string)
	flush   func()
}

func (r *replEvents) Delta(text string) {
	if r.deltaIn == nil {
		r.deltaIn, r.flush = smoothPrinter()
	}
	r.deltaIn(text)
}

// Reasoning streams thinking in dim grey through the same smooth printer
// as content — one channel, one goroutine. The prefix goes out once with
// the colour escape so the whole block reads as a scratchpad, and
// ReasoningDone resets the colour, flushes, and leaves a blank line
// before whatever comes next.
func (r *replEvents) Reasoning(text string) {
	if r.deltaIn == nil {
		r.deltaIn, r.flush = smoothPrinter()
		r.deltaIn("\x1b[90mthinking…\n")
	}
	r.deltaIn(text)
}

func (r *replEvents) ReasoningDone() {
	if r.deltaIn == nil {
		return
	}
	r.deltaIn("\x1b[0m")
	r.flush()
	r.deltaIn, r.flush = nil, nil
	fmt.Print("\n\n")
}

func (r *replEvents) AssistantDone(content string) {
	if r.deltaIn != nil {
		r.flush()
		r.deltaIn, r.flush = nil, nil
	}
	if content != "" {
		fmt.Println()
	}
}

func (r *replEvents) ToolCall(id, name, args string) {
	fmt.Printf("[calling %s]\n", name)
}

func (r *replEvents) ToolResult(id, content string, isErr bool) {}

// runREPL is the outer loop: one prompt, one Send, repeat. Turn failures
// print and return to the prompt rather than killing the session.
func runREPL(session *agent.Session) {
	scanner := bufio.NewScanner(os.Stdin)

	// approve is the terminal half of the write gate: gated tools show
	// what they're about to run and wait for a y/yes before executing.
	// It shares the REPL's scanner — stdin has exactly one reader.
	approve := func(name, args string) tools.Decision {
		fmt.Printf("\n[%s wants to run]\n%s\napprove? [y/N] ", name, args)
		if !scanner.Scan() {
			return tools.Declined
		}
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if answer == "y" || answer == "yes" {
			return tools.Approved
		}
		return tools.Declined
	}

	ev := &replEvents{}
	for {
		fmt.Print("< ")
		if !scanner.Scan() {
			break
		}
		if err := session.Send(scanner.Text(), ev, approve); err != nil {
			fmt.Printf("request failed: %v\n", err)
		}
	}
}
