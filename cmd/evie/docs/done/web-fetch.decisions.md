# web-fetch — decisions

Shipped 2026-08-03. `web_fetch` in `internal/tools/webfetch.go`, tests in
`internal/tools/webfetch_test.go`. Built via autopilot: spec-reviewed
spec, spec-derived failing tests from an independent agent, staged
implementation, fresh-context code review.

The spec (`web-fetch.spec.md`, same directory) carries the full decision
list with reasoning; this file records what changed *during* the build
and the gaps shipped knowingly.

## Decisions the spec review forced (before any code)

The fresh-context spec reviewer found 19 places an executor would have
guessed. Three changed the design:

- **Prompt injection named as the real new threat.** `web_fetch` imports
  attacker-authored text into a session where `bash` is ungated. The
  result is fenced in `[begin untrusted web content …]` delimiters —
  framing, not a fix; recorded as an accepted residual risk that makes
  ungated bash a slightly worse bet than before this tool existed.
- **URL credentials stripped.** Verified by spike: Go's http.Client
  silently sends `Authorization: Basic` from URL userinfo, and the URL is
  echoed into error/redirect messages. `normalizeURL` sets `u.User = nil`.
- **HTTPS upgrade exempts local/private addresses** — otherwise every
  `httptest` test and every real dev-server fetch breaks.

Two claims were spike-verified rather than assumed: a custom
`CheckRedirect` **replaces** Go's built-in 10-hop cap (a nil-returning
one looped forever), and `ErrUseLastResponse` hands back the 3xx with its
body still open.

## Decisions made during the build

- **Test-writer ambiguities resolved in the spec, not in code**: empty
  results are Go errors (both the JS-shell hint and the zero-byte case),
  and the cap note states the dropped byte count explicitly rather than
  only bash's "X of Y shown".
- **Table cell separators go between cells**, not after each — caught by
  the independent tests (`a | b | ` vs `a | b`).
- **`x/net` pinned at latest** (v0.57.0); `go mod tidy` had quietly
  resolved the old transitive pin v0.17.0.

## Known gaps, shipped deliberately

- **Lone-space lines in Markdown output.** Whitespace text nodes between
  block elements emit `" "` on their own line, and `\n\n \n\n` survives
  the newline collapse. Found by the stage-3 demo against the real
  pkg.go.dev page; David's verdict: harmless, leave it.
- **Nav chrome is not stripped.** The real pkg.go.dev page carries ~450
  lines of site navigation before the documentation. Readability-style
  content extraction is a different (and much bigger) feature.
- **Charset**: UTF-8 only; anything else passes through untranscoded.
- **Content-encoding**: only Go's transparent gzip; unrequested `br`
  yields garbage.
- **No JS rendering, no caching, no retries, no robots.txt, no SSRF
  denylist** — all argued in the spec's Out of scope.

## What the code review caught

Fresh-context reviewer, diff + spec only. Four real bugs, all fixed:

1. **Credential leak via redirect echo.** The input URL had userinfo
   stripped, but a cross-host `Location` header — attacker-controlled —
   was echoed verbatim, token included. Now `loc.User = nil` too. The
   lesson: sanitizing your input is not sanitizing your inputs; every
   string a server hands back travels the same path to the model
   provider.
2. **Spill-file collision.** Per-pid naming meant a session's second
   capped fetch silently overwrote the first, so the model would grep a
   stale path and confidently read the wrong page. Now `os.CreateTemp`
   per call. This was a *spec* bug faithfully implemented — the review
   caught what the spec review didn't.
3. **Divergent Content-Type defaults.** `extractText` treated a missing
   Content-Type as text/plain, but the JS-hint check defaulted the same
   header to HTML — an empty body with no Content-Type got "may be
   JavaScript-rendered". Two parses of one header must share a default.
4. **Nested `<pre>` dropped verbatim mode early** — inner block set
   `inPre = false` on exit while still inside the outer. Save/restore,
   not set/clear, for any recursion-carried flag.
