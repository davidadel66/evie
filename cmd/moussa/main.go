// Command agent is the moussa agent harness — the nucleus of this repo.
// It runs an interactive REPL: user input goes to an LLM via OpenRouter,
// and any tool calls the model makes are executed against the registry in
// tools.go, with results fed back until the model answers in plain text.
//
// Every capability under internal/ (todo, finance, ...) is meant to be
// exposed to the model here as an AgentTool over time.
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/davidadel66/moussa/internal/openrouter"
	"github.com/davidadel66/moussa/internal/tools"

	"github.com/joho/godotenv"
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

// main runs the chat loop. Outer loop: one user turn. Inner loop: call the
// model, execute every tool call it requests, and go around again until the
// model responds with no tool calls — that response ends the turn. Request
// failures print and return to the prompt rather than killing the session.
func main() {
	_ = godotenv.Load("../../.env")
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := "moonshotai/kimi-k3"
	client, err := openrouter.NewClient(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	messages := []openrouter.Message{
		{
			Role:    "system",
			Content: "You're name is Eve - A helpful assistant to David with the intent to be his personal 'jarvis'",
		},
	}

	scanner := bufio.NewScanner(os.Stdin)

	// approve is the terminal half of the write gate: gated tools show
	// what they're about to run and wait for a y/yes before executing.
	// It shares the REPL's scanner — stdin has exactly one reader.
	approve := func(name, args string) bool {
		fmt.Printf("\n[%s wants to run]\n%s\napprove? [y/N] ", name, args)
		if !scanner.Scan() {
			return false
		}
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return answer == "y" || answer == "yes"
	}

	for {
		fmt.Print("< ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		userMsg := openrouter.Message{
			Role:    "user",
			Content: input,
		}

		messages = append(messages, userMsg)

		for {
			req := openrouter.ChatRequest{
				Model:    model,
				Messages: messages,
				Tools:    tools.Schemas(),
			}

			onDelta, done := smoothPrinter()
			res, err := client.ChatStream(req, onDelta)
			done()
			if err != nil {
				fmt.Printf("request failed: %v\n", err)
				break
			}

			if len(res.Choices) == 0 {
				fmt.Println("no response returned")
				break
			}

			messages = append(messages, res.Choices[0].Message)
			if res.Choices[0].Message.Content != "" {
				fmt.Println()
			}

			if len(res.Choices[0].Message.ToolCalls) == 0 {
				break
			}

			for _, toolCall := range res.Choices[0].Message.ToolCalls {
				fmt.Printf("[calling %s]\n", toolCall.Function.Name)
				messages = append(messages, tools.Execute(toolCall, approve))
			}

		}

	}
}
