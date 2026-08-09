# web-search — decisions

Shipped 2026-08-03, same day as web_fetch and by the same autopilot
pipeline (spec review → independent spec-derived tests → staged build →
fresh-context code review). The spec carries the decisions with
reasoning; this records what the build added.

## Provider

Brave Search API. Free tier (2,000 queries/month, 1 req/s) is beyond
evie's usage; the key rate-limits rather than bills. Rejected: LLM-tuned
search (Tavily-style) — it summarizes before the model sees results, the
same middleman raw-`web_fetch` rejected; and scraping google/DDG result
pages — ToS violation that breaks unpredictably on bot detection.

## What the spec review caught (13 findings, pre-code)

The three that mattered: a const/var contradiction that would have made
every server test uncompilable; every server-backed test needing a fake
key via `t.Setenv` (or the suite depends on the developer's shell); and
the double-decode trap — `x/net/html`'s `Tokenizer.Text()` already
entity-decodes, so adding `UnescapeString` on top turns a legitimate
`&amp;amp;` into `&`.

## The latent bug this feature flushed out

`main.go`'s `godotenv.Load("../../.env")` was cwd-relative and had never
loaded the repo `.env` from the repo root — every key (Plaid, OpenRouter)
silently came from the shell environment instead. `BRAVE_API_KEY`, living
only in `.env`, exposed it. Fixed with two sequential `Load` calls
(".env", then "../../.env") — two calls because godotenv's variadic form
aborts on the first missing file. Accepted residual: from the repo root
the second call probes `~/code/.env`; if that ever exists it loads, capped
by godotenv's no-override rule.

## What the code review caught

- **Timeout mid-body was mislabeled.** The `DeadlineExceeded` unwrap
  existed only on `client.Do`; a server that sends headers fast and
  trickles the body would surface "read brave response: context deadline
  exceeded" instead of the promised timeout message. Same unwrap now on
  the read path.
- **Empty title/URL rendered orphan lines.** A title that strips to
  nothing now falls back to the URL as the label; the URL line renders
  only when it exists and isn't already the title. Pinned by a test.
- The dep-pruning in `go.mod`/`go.sum` (`OpenRouterTeam/go-sdk`, testify,
  etc.) belongs to web_fetch's `go mod tidy`, not this feature — commits
  should attribute it there.

## Known gaps, shipped deliberately

- Zero-results and missing-field unit fixtures are synthetic (blessed in
  the spec); the checked-in `testdata/brave-search.json` is a real capture
  (52KB, query "golang html parser", count=5).
- Negative `count` clamps to 1 rather than defaulting to 10 — the
  test-writer's reading of the clamp table, kept.
- No paging, no verticals (news/images/videos), no search operators
  beyond quoted phrases, no retry loops, no caching — spec's Out of
  scope.
- 401/403-means-bad-key is assumed, not verified against Brave; anything
  surprising lands in the generic status branch, which is a fine failure
  mode.
