package agent

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	// DefaultTurnStepLimit bounds the model responses in one turn, including
	// the final tool-free response. Transport retries of the same response
	// do not count.
	DefaultTurnStepLimit = 100
	TurnStepLimitEnv     = "EVIE_TURN_STEP_LIMIT"
)

// ErrStepLimitExceeded reports a turn whose final tool-free response still
// asked for tools.
var ErrStepLimitExceeded = errors.New("agent: turn reached its step limit without a final answer")

func resolveTurnStepLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultTurnStepLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 0, fmt.Errorf("%s requires a positive integer", TurnStepLimitEnv)
	}
	return limit, nil
}

// ValidateTurnConfiguration lets startup reject an invalid step limit before
// creating sessions. Constructors also retain the error for embedded callers.
func ValidateTurnConfiguration() error {
	_, err := resolveTurnStepLimit(os.Getenv(TurnStepLimitEnv))
	return err
}

// stepLimitNote is the harness instruction sent as the last message of the
// final tool-free request.
func stepLimitNote(limit int) string {
	return fmt.Sprintf("Evie harness notice: this turn has reached its limit of %d model responses, "+
		"so tools are no longer available. Answer the user now using only what you already have. "+
		"Say briefly what you could not finish.", limit)
}
