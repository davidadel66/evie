# Changelog

Newest first. The format is in the explorer README. A heading inside a fence
is not an entry:

```text
## 2026-12-31 — Not an entry
```

## 2026-10-20 — Add the tool's run command
Branch: feat/run
Touches: cmd/tool, internal/alpha
Sources: doc:docs/guide.md#getting-started, #12

The tool gains a run command that starts alpha.
It reads its widgets first.

A second paragraph says why.

## 2026-10-20 — Retire the old store
Branch: chore/retire-store
Touches: internal/gone
Sources: adr:0001

The old store is removed; its history stays readable.

## 2026-10-02 — Start the fixture
Branch: master
Touches: internal/alpha, scripts/build
Sources: #1, code:internal/alpha/alpha.go

The first entry.
