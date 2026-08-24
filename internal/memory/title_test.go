package memory

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeSessionTitle(t *testing.T) {
	invalid := string([]byte{'a', 0xff, 'b'})
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "blank controls and format", input: " \t\n\x1b\u200b ", want: ""},
		{name: "unicode whitespace", input: "  hello\u00a0\u2003world  ", want: "hello world"},
		{name: "control and format removal", input: "safe\n\t\x1b[31m\u200btitle", want: "safe[31mtitle"},
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
