//go:build !race

package eviedb

// raceTimeScale stretches wall-clock work budgets in race-instrumented
// builds (see race_scale_race.go). Ordinary builds use the budgets as written.
const raceTimeScale = 1
