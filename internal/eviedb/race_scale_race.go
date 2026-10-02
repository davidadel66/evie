//go:build race

package eviedb

// The race detector slows instrumented code by roughly 5-10x, so wall-clock
// work budgets would otherwise expire on correct code under `go test -race`.
// Only race-instrumented test binaries are affected.
const raceTimeScale = 10
