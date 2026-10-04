package subagents

import (
	"time"

	"github.com/davidadel66/evie/internal/composition"
)

// BuildReceiptForTest is the build child receipt the supervisor admits for
// a parent with the given receipt.
func BuildReceiptForTest(template, parent composition.Receipt) (composition.Receipt, error) {
	return buildReceipt(template, parent)
}

// SetClockForTest replaces the clock that measures children's time budgets.
// The hard deadline remains a real-time context deadline.
func SetClockForTest(s *Supervisor, now func() time.Time) { s.now = now }

// SetSettleWindowForTest bounds how long an owner retries its attempt's
// terminal write before handing the attempt to recovery.
func SetSettleWindowForTest(s *Supervisor, d time.Duration) { s.settleWindow = d }

// PendingForTest reports how many decided outcomes are waiting to be recorded.
func PendingForTest(s *Supervisor) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.unsettled)
}
