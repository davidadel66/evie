package subagents

import "time"

// SetClockForTest replaces the clock that measures children's time budgets.
// The hard deadline remains a real-time context deadline.
func SetClockForTest(s *Supervisor, now func() time.Time) { s.now = now }
