// What each of the diff's colouring workers runs: the diff library's own
// worker, @pierre/diffs (Apache-2.0), which T3 Code's diff panel runs too,
// started by `holdPool` in diff-view.ts. The server builds it beside the
// page, with which it shares its chunks, and it loads the highlighter only
// when it first colours. The library's ready-made worker-portable.js, which
// evie ran before, is 1.3 MB, most of it two WebAssembly programs for
// highlighters evie does not use: evie colours with Shiki's JavaScript
// engine.
import "@pierre/diffs/worker/worker.js"
