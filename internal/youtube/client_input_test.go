package youtube

// Stage 2 acceptance tests derived from
// cmd/evie/docs/active/youtube-transcripts.spec.md before implementation.

import (
	"net/http"
	"strings"
	"testing"
)

var _ func(*http.Client) *Client = NewClient

func TestParseVideoInputAcceptsOnlyDeclaredForms(t *testing.T) {
	const id = "dQw4w9WgXcQ"
	const canonical = "https://www.youtube.com/watch?v=" + id
	tests := []struct {
		name  string
		input string
	}{
		{"bare ID", id},
		{"watch", "https://www.youtube.com/watch?v=" + id},
		{"watch without www and extra query", "http://youtube.com/watch?feature=share&v=" + id + "&t=3"},
		{"short URL", "https://youtu.be/" + id},
		{"shorts", "https://www.youtube.com/shorts/" + id},
		{"live", "https://youtube.com/live/" + id},
		{"embed", "https://www.youtube.com/embed/" + id},
		{"privacy embed", "https://youtube-nocookie.com/embed/" + id},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotURL, err := parseVideoInput(tc.input)
			if err != nil {
				t.Fatalf("parseVideoInput(%q): %v", tc.input, err)
			}
			if gotID != id || gotURL != canonical {
				t.Errorf("parseVideoInput(%q) = (%q, %q), want (%q, %q)", tc.input, gotID, gotURL, id, canonical)
			}
		})
	}
}

func TestParseVideoInputRejectsMalformedOrUnsupportedInputs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"ten-character ID", "dQw4w9WgXc"},
		{"twelve-character ID", "dQw4w9WgXcQx"},
		{"invalid ID punctuation", "dQw4w9WgX!Q"},
		{"ID hidden in text", "prefix-dQw4w9WgXcQ-suffix"},
		{"wrong host", "https://example.com/watch?v=dQw4w9WgXcQ"},
		{"host suffix attack", "https://www.youtube.com.evil.test/watch?v=dQw4w9WgXcQ"},
		{"userinfo", "https://user:pass@www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{"unsupported scheme", "ftp://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{"playlist", "https://www.youtube.com/playlist?list=PL123"},
		{"search", "https://www.youtube.com/results?search_query=dQw4w9WgXcQ"},
		{"channel as video", "https://www.youtube.com/channel/UCaaaaaaaaaaaaaaaaaaaaaa"},
		{"watch missing v", "https://www.youtube.com/watch?t=3"},
		{"ID in fragment", "https://www.youtube.com/watch#v=dQw4w9WgXcQ"},
		{"extra short path", "https://youtu.be/dQw4w9WgXcQ/more"},
		{"lookalike short host", "https://www.youtu.be/dQw4w9WgXcQ"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseVideoInput(tc.input)
			if err == nil {
				t.Fatalf("parseVideoInput(%q) succeeded", tc.input)
			}
			assertActionableInputError(t, err, "video")
		})
	}
}

func TestParseChannelInputAcceptsOnlyHandleAndStableIDForms(t *testing.T) {
	const id = "UCaaaaaaaaaaaaaaaaaaaaaa"
	tests := []struct {
		name       string
		input      string
		wantID     string
		wantHandle string
	}{
		{"bare minimum handle", "@abc", "", "@abc"},
		{"bare maximum handle", "@" + strings.Repeat("a", 30), "", "@" + strings.Repeat("a", 30)},
		{"handle URL", "https://www.youtube.com/@EvieTest", "", "@EvieTest"},
		{"handle videos URL", "http://youtube.com/@EvieTest/videos", "", "@EvieTest"},
		{"bare channel ID", id, id, ""},
		{"channel ID URL", "https://youtube.com/channel/" + id, id, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotHandle, err := parseChannelInput(tc.input)
			if err != nil {
				t.Fatalf("parseChannelInput(%q): %v", tc.input, err)
			}
			if gotID != tc.wantID || gotHandle != tc.wantHandle {
				t.Errorf("parseChannelInput(%q) = (%q, %q), want (%q, %q)", tc.input, gotID, gotHandle, tc.wantID, tc.wantHandle)
			}
		})
	}
}

func TestParseChannelInputRejectsUnsupportedAndAmbiguousForms(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"two-character handle", "@ab"},
		{"thirty-one-character handle", "@" + strings.Repeat("a", 31)},
		{"handle with slash", "@abc/def"},
		{"handle with whitespace", "@abc def"},
		{"short channel ID", "UCaaaaaaaaaaaaaaaaaaaaa"},
		{"long channel ID", "UCaaaaaaaaaaaaaaaaaaaaaaa"},
		{"bad channel ID character", "UCaaaaaaaaaaaaaaaaaaaaa!"},
		{"legacy custom URL", "https://youtube.com/c/evie"},
		{"legacy user URL", "https://youtube.com/user/evie"},
		{"playlist", "https://youtube.com/playlist?list=PL123"},
		{"search", "https://youtube.com/results?search_query=evie"},
		{"video URL", "https://youtube.com/watch?v=dQw4w9WgXcQ"},
		{"wrong host", "https://example.com/@evie"},
		{"host suffix attack", "https://youtube.com.evil.test/@evie"},
		{"userinfo", "https://user@youtube.com/@evie"},
		{"scheme missing", "youtube.com/@evie"},
		{"extra handle path", "https://youtube.com/@evie/about"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseChannelInput(tc.input)
			if err == nil {
				t.Fatalf("parseChannelInput(%q) succeeded", tc.input)
			}
			assertActionableInputError(t, err, "channel")
		})
	}
}

func TestNormalizeLanguage(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "en"},
		{" EN_us ", "en-us"},
		{"pt-BR", "pt-br"},
		{"zh_Hant_TW", "zh-hant-tw"},
	} {
		if got := normalizeLanguage(tc.input); got != tc.want {
			t.Errorf("normalizeLanguage(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func assertActionableInputError(t *testing.T, err error, subject string) {
	t.Helper()
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, subject) {
		t.Errorf("error %q does not identify the bad %s input", err, subject)
	}
	if !strings.Contains(message, "accept") && !strings.Contains(message, "support") && !strings.Contains(message, "invalid") {
		t.Errorf("error %q does not explain accepted/supported input", err)
	}
}
