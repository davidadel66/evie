package agent

import (
	"os"
	"testing"
)

// Every test in the package checks the turn's reused history derivations
// against fresh ones, so each existing scenario also proves the reuse exact.
func TestMain(m *testing.M) {
	verifyRequestHistory = true
	os.Exit(m.Run())
}
