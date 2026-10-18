package agent

import (
	"os"
	"testing"
)

// Every test in the package checks the turn's shortcuts (reused history
// derivations, incrementally loaded events, running request sizes) against
// the original derivations, so each existing scenario also proves them exact.
func TestMain(m *testing.M) {
	verifyReuse = true
	os.Exit(m.Run())
}
