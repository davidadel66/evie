# How LLM calls work — running checklist

Review-mode walkthrough of the provider→harness path. Each section is
confirmed understood when David can restate it.

## The map (confirmed)

One turn: `Session.Send` appends the user message → builds `ChatRequest` →
`ChatStream` streams fragments live via `StreamHandlers` while assembling →
assembled message appended to transcript → tool calls? execute + loop :
`Events.AssistantDone`, done.

- `Message` / `ChatRequest` / `ChatResponse` — wire structs; struct IS the JSON (tags do the translation).
- `agent.Client` interface — consumer-side, one method, so tests script fakes.
- `agent.Events` interface — session's only line to a frontend (REPL, SSE).

## Transport: internal/openrouter/client.go (confirmed)

Three transformations:

1. **struct → wire**: `r.Stream = true` (the method owns the flag), one
   `json.Marshal`, plain `net/http`, non-200 body folded into the error.
   - `defer resp.Body.Close()` goes *after* the err check: defer evaluates
     the receiver immediately (nil → panic), and early defer = every return
     path closes the body.
   - Every error wrapped `fmt.Errorf("...: %w", err)` + zero value return.
2. **body → chunks**: `bufio.Scanner` over SSE lines; gates: skip
   non-`data:` (keepalives), break on `[DONE]`, else `json.Unmarshal` one
   line into `streamChunk`.
   - `scanner.Buffer(..., 1MB)`: default line cap is 64KB
     (`bufio.MaxScanTokenSize`); one chunk carrying tool-call args can
     exceed it and silently kill the scan.
3. **chunks → Message**: three accumulators — `Content` (`+=`), `Reasoning`
   (`+=`), `ToolCalls` (glued by index, parallel calls interleave) — plus
   `details []json.RawMessage` collected in-loop, marshalled once
   *after* the loop (guard `len > 0` keeps `omitempty` honest).
   - Rule learned: loop collects, finishing steps run once after.

Design contract: streaming and non-streaming return the same assembled
shape, so the harness never cares how bytes arrived.

## Harness: internal/agent/agent.go (pending)

## Reasoning feature, stage 1 (in progress)

- [x] Message.Reasoning / ReasoningDetails fields
- [x] streamChunk delta parsing
- [x] accumulation + post-loop marshal
- [x] StreamHandlers struct (why: named fields at call sites, extends
      without breaking callers)
- [x] ChatRequest.Reasoning *ReasoningConfig (why pointer: nil+omitempty
      omits the key; a struct value can't — off→nil keeps the wire identical
      to before the feature)
- [x] tests against testdata/kimi-reasoning-stream.txt (baseURL seam:
      constructor defaults prod, same-package test overrides)
