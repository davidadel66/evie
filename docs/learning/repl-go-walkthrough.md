# Go walkthrough: `cmd/evie/repl.go`

A guided tour of the terminal frontend, focused on the Go mechanics it uses.
Read it next to the file — sections go top to bottom.

## One package, many files

`repl.go` starts with `package main`, same as `main.go`. In Go, a directory is
one package; every file in it shares one namespace, no imports needed between
them. That's why `main.go` can call `runREPL` and `repl.go` can be split out
without any ceremony. The file-level comment convention still applies per file,
but the *package* doc comment lives on exactly one of them (main.go here).

## `smoothPrinter` — a function that returns functions

```go
func smoothPrinter() (onDelta func(string), done func())
```

Functions are first-class values in Go. `smoothPrinter` builds two of them and
hands them back; both are **closures** — they capture the variables (`ch`,
`finished`) from the enclosing call. Each call to `smoothPrinter()` creates a
*fresh* `ch` and `finished`, so two printers never share state. This is Go's
lightweight alternative to defining a struct with methods when the state is
small and private: the state lives in captured variables instead of fields.

Named return values (`onDelta func(string)`) are used here purely as
documentation — the signature tells you what each returned function is for.

### The channel

```go
ch := make(chan string, 64)
```

A **buffered channel**: sends don't block until 64 items are queued. That's the
whole point here — the network goroutine delivering tokens should almost never
wait on the terminal. If the buffer fills, sends block, which applies
backpressure instead of growing memory without bound. Unbuffered (`make(chan
string)`) would force the stream and the printer into lockstep; a buffer
decouples their rhythms.

`finished := make(chan struct{})` is the other idiom: a channel used purely as
a signal, never carrying data. `struct{}` is the empty type — zero bytes — the
conventional "I only care about the event" element type. Nobody sends on it;
it's *closed* (see below), and closing is the signal.

### The goroutine

```go
go func() { ... }()
```

`go` starts the function on a new goroutine — a few KB of stack, scheduled by
the runtime, thousands are normal. Note it's defined and invoked in one
expression (the trailing `()`); it also closes over `ch` and `finished`.

`defer close(finished)` — `defer` runs when the *function* returns, however it
returns. Deferring the close means "whenever this goroutine ends, signal it,"
with no way to forget a return path.

### `select` — waiting on multiple things

```go
select {
case s, ok := <-ch:      // a token arrived (or the channel closed)
case <-ticker.C:         // 12ms passed
}
```

`select` blocks until one of its cases is ready; if several are ready it picks
randomly. It's the concurrency workhorse: this loop is simultaneously "drain
tokens as they arrive" and "wake up every 12ms to print," with no locks and no
polling.

The two-value receive `s, ok := <-ch` is how you detect closure: `ok` is false
once the channel is closed *and drained*. That's the shutdown path — print
whatever is buffered and return (which fires the deferred `close(finished)`).

`time.NewTicker(12 * time.Millisecond)` delivers on its channel `ticker.C`
every interval. `defer ticker.Stop()` releases its timer — tickers leak if you
abandon them running.

### Runes, not bytes

```go
var buf []rune
buf = append(buf, []rune(s)...)
```

A Go `string` is bytes; UTF-8 characters can span several. Slicing a string at
an arbitrary byte index can cut a character in half — printing `buf[:n]` on
bytes could emit garbage mid-emoji. Converting to `[]rune` makes each element
one Unicode code point, so any slice boundary is print-safe. The `...` spreads
the slice into `append` as individual elements.

The pacing math (`n := 1 + len(buf)/20`) is adaptive: the deeper the backlog,
the more characters per tick, so the printer catches up instead of lagging
behind a fast stream forever.

### The handshake in `done`

```go
return func(s string) { ch <- s },
    func() { close(ch); <-finished }
```

`close(ch)` tells the goroutine "no more tokens"; `<-finished` then *blocks
until the goroutine confirms it has printed everything*. Without that receive,
the caller could print the next prompt while the tail of the answer was still
being flushed. This close-then-wait pair is the standard clean-shutdown
handshake for a worker goroutine.

Also a rule this code respects: **the sender closes the channel**, never the
receiver. Sending on a closed channel panics; only the side that knows no more
sends are coming can close safely.

## `replEvents` — satisfying an interface without saying so

```go
type replEvents struct {
    deltaIn func(string)
    flush   func()
}
```

Struct fields can hold functions — here the live printer's two ends, or `nil`
when no message is streaming.

`replEvents` implements `agent.Events`, but notice: **it never declares that**.
Go interfaces are satisfied *structurally* — define the right method set and
you implement the interface, no `implements` keyword. The compiler checks at
the point of use (`session.Send(..., ev, ...)`), where `*replEvents` must fit
the `agent.Events` parameter. This is why `internal/agent` could define the
interface it *needs* (consumer side) and this package can satisfy it without
importing anything extra.

### Pointer receivers

```go
func (r *replEvents) Delta(text string) {
```

The receiver is `*replEvents` (pointer), not `replEvents` (value), because the
methods *mutate* `r.deltaIn`/`r.flush`. With a value receiver each call would
get a copy and the mutation would vanish. Rule of thumb: if any method needs a
pointer receiver, give the whole type pointer receivers — and note that the
method set of `*replEvents` includes those methods, which is why `runREPL`
passes `&replEvents{}` (the `&` matters).

### Lazy initialization

```go
if r.deltaIn == nil {
    r.deltaIn, r.flush = smoothPrinter()
}
```

A printer is started on the *first* delta of each message and torn down in
`AssistantDone` (reset to `nil`). Two things this buys: messages that stream
nothing (tool-only turns) never start a goroutine, and pacing state can't leak
from one message into the next. Comparing a func field to `nil` is the normal
"is it set" test.

`func (r *replEvents) ToolResult(id, content string, isErr bool) {}` — an
empty method body is deliberate and idiomatic: the interface demands the
method; this frontend just has nothing to render for it (the REPL never showed
tool results). The unused parameters are fine — Go only rejects unused *local
variables*, not parameters.

## `runREPL`

### `bufio.Scanner`

```go
scanner := bufio.NewScanner(os.Stdin)
```

`os.Stdin` is a raw byte stream; `bufio.Scanner` chops it into lines (its
default split function). `scanner.Scan()` reads the next line, returning false
on EOF (Ctrl-D) or error — hence `if !scanner.Scan() { break }` as the exit
condition, and `scanner.Text()` for the line just read.

One scanner is created and shared with the approver closure. That's a real
constraint, not convenience: stdin has one read position; two buffered readers
would silently steal bytes from each other. The comment in the code says
exactly this — comments earn their place by stating constraints the code can't.

### The approver closure

```go
approve := func(name, args string) bool { ... }
```

Again a closure over `scanner`. Its shape (`func(name, args string) bool`)
matches what `Send` expects — plain function values as dependency injection,
no interface needed when one function is the whole contract.

`strings.ToLower(strings.TrimSpace(...))` before comparing means `" Y "` and
`"yes"` both pass; defaulting to *false* on EOF or anything unrecognized is the
gate failing closed.

### The loop body

```go
if err := session.Send(scanner.Text(), ev, approve); err != nil {
    fmt.Printf("request failed: %v\n", err)
}
```

The `if` with an init statement scopes `err` to the `if` — the standard Go
error-check shape. Note what this loop *doesn't* do anymore: no message slice,
no request building, no tool dispatch. All of that moved behind `Send`; the
REPL's whole job is I/O, which is the separation the extraction was for.

One `ev` is reused across turns — safe because its lazy-init cycle returns it
to the zero state after every assistant message.

## Concepts index

- closures & functions as values — `smoothPrinter`, `approve`
- buffered vs unbuffered channels, backpressure — `ch`
- signal-only channels, `chan struct{}`, close-as-broadcast — `finished`
- goroutines, `go func(){}()` — the printer
- `select`, ticker loops — the pacing loop
- two-value receive (`s, ok := <-ch`) — detecting closed channels
- `defer` for cleanup — `close(finished)`, `ticker.Stop()`
- runes vs bytes — `[]rune` buffer
- structural (implicit) interface satisfaction — `replEvents` vs `agent.Events`
- pointer vs value receivers, method sets — `*replEvents`
- lazy init via nil func fields — `Delta`
- `bufio.Scanner`, single-reader stdin — `runREPL`
- fail-closed defaults — the approver
