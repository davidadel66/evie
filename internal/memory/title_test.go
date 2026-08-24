package memory

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestProjectDisplayLabelUsesSafeNameOrTimestampedUTCFallback(t *testing.T) {
	createdAt := time.Date(2026, 8, 24, 16, 5, 6, 123456789, time.FixedZone("test", -4*60*60))
	if got := ProjectDisplayLabel(" Pro\nject\u200b ", createdAt); got != "Pro ject" {
		t.Fatalf("safe project label = %q", got)
	}
	const want = "Untitled project — 2026-08-24T20:05:06.123456789Z"
	if got := ProjectDisplayLabel("\x1b\u200b", createdAt); got != want {
		t.Fatalf("fallback project label = %q, want %q", got, want)
	}
}

func TestNormalizeSessionTitle(t *testing.T) {
	invalid := string([]byte{'a', 0xff, 'b'})
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "blank controls and format", input: " \t\n\x1b\u200b ", want: ""},
		{name: "unicode whitespace", input: "  hello\u00a0\u2003world  ", want: "hello world"},
		{name: "control and format removal", input: "safe\n\t\x1b[31m\u200btitle", want: "safe [31mtitle"},
		{name: "invalid UTF-8 replacement", input: invalid, want: "a\uFFFDb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeSessionTitle(tt.input); got != tt.want {
				t.Fatalf("NormalizeSessionTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeSessionTitleRuneLimit(t *testing.T) {
	for _, n := range []int{79, 80, 81, 120} {
		t.Run(string(rune(n)), func(t *testing.T) {
			got := NormalizeSessionTitle(strings.Repeat("界", n))
			wantRunes := n
			if n > SessionTitleRuneLimit {
				wantRunes = SessionTitleRuneLimit
			}
			if utf8.RuneCountInString(got) != wantRunes {
				t.Fatalf("runes = %d, want %d", utf8.RuneCountInString(got), wantRunes)
			}
			if n > SessionTitleRuneLimit && !strings.HasSuffix(got, "…") {
				t.Fatalf("truncated title = %q, want ellipsis", got)
			}
		})
	}
}

func TestSessionTitleCandidateEligibility(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		role      EventRole
		parentID  EventID
		content   string
		want      string
	}{
		{name: "qualifying root user", eventType: EventUserMessage, role: RoleUser, content: " first\n title ", want: "first title"},
		{name: "blank root user", eventType: EventUserMessage, role: RoleUser, content: " \t\n"},
		{name: "assistant event", eventType: EventAssistantMessage, role: RoleAssistant, content: "assistant"},
		{name: "wrong user role", eventType: EventUserMessage, role: RoleAssistant, content: "wrong role"},
		{name: "child user event", eventType: EventUserMessage, role: RoleUser, parentID: "parent", content: "child"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SessionTitleCandidate(tt.eventType, tt.role, tt.parentID, tt.content); got != tt.want {
				t.Fatalf("candidate=%q, want %q", got, tt.want)
			}
		})
	}
}
