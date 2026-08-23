package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// fetchURL drives the tool the way the dispatcher does — through raw
// arguments JSON — so the unmarshalling is exercised too. No *testing.T,
// because the redirect-loop test calls it from a watchdog goroutine.
func fetchURL(raw string) (string, error) {
	args, _ := json.Marshal(map[string]string{"url": raw})
	return webFetch(context.Background(), string(args))
}

// mustParseURL parses a URL for a table row; a bad literal is a bug in the
// test, not a failure of the code under test.
func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("test setup: url.Parse(%q): %v", raw, err)
	}
	return u
}

// toMarkdown runs htmlToMarkdown and fails on error, so the assertions read
// as one-liners.
func toMarkdown(t *testing.T, html, pageURL string) string {
	t.Helper()
	got, err := htmlToMarkdown(strings.NewReader(html), mustParseURL(t, pageURL))
	if err != nil {
		t.Fatalf("htmlToMarkdown returned error: %v", err)
	}
	return got
}

// hasLine reports whether s contains a line equal to want, ignoring
// surrounding whitespace. strings.Contains cannot tell "# Alpha" from
// "## Alpha" — the second contains the first — so heading levels have to be
// checked a line at a time.
func hasLine(s, want string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

// hasIndentedLine is hasLine with leading whitespace kept significant, for
// list nesting where the indentation is the assertion.
func hasIndentedLine(s, want string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimRight(line, " \t\r") == want {
			return true
		}
	}
	return false
}

// firstLine returns the first non-empty line, which is where <title> is
// supposed to land.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// normalizeURL
// ---------------------------------------------------------------------------

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		want       string // expected u.String(), when no error
		wantErr    bool
		errMustSay string // substring the error message has to carry
	}{
		// Scheme handling.
		{name: "bare domain with no scheme is rejected", in: "example.com", wantErr: true},
		{name: "http is upgraded to https", in: "http://example.com", want: "https://example.com"},
		{name: "https is left alone", in: "https://example.com/docs", want: "https://example.com/docs"},
		{name: "surrounding whitespace is trimmed", in: "  https://example.com/docs  ", want: "https://example.com/docs"},

		// The loopback/private exemption from the upgrade.
		{name: "localhost is not upgraded", in: "http://localhost:8080", want: "http://localhost:8080"},
		{name: "loopback IP is not upgraded", in: "http://127.0.0.1:9000", want: "http://127.0.0.1:9000"},
		{name: "private IP is not upgraded", in: "http://192.168.1.10", want: "http://192.168.1.10"},

		// Rejections. The scheme check runs before the host check, so a
		// file:// URL must complain about the scheme and not about the host —
		// the message is what the model reads to fix its own call.
		{name: "file scheme is rejected as a scheme problem", in: "file:///etc/passwd", wantErr: true, errMustSay: "scheme"},
		{name: "data scheme is rejected", in: "data:text/html,x", wantErr: true},
		{name: "empty is rejected", in: "", wantErr: true},
		{name: "whitespace-only is rejected", in: "   ", wantErr: true},
		{name: "no host is rejected", in: "https:///path", wantErr: true, errMustSay: "host"},
		{name: "over-length is rejected", in: "https://example.com/" + strings.Repeat("a", maxURLLength), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeURL(tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeURL(%q) = %v, want an error", tt.in, got)
				}
				if tt.errMustSay != "" && !strings.Contains(strings.ToLower(err.Error()), tt.errMustSay) {
					t.Errorf("normalizeURL(%q) error = %v, want it to mention %q", tt.in, err, tt.errMustSay)
				}
				return
			}

			if err != nil {
				t.Fatalf("normalizeURL(%q) returned error: %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.in, got.String(), tt.want)
			}
		})
	}

	// Credentials in the URL would be turned into an Authorization header by
	// http.Client and echoed back in redirect and error messages, printing
	// the token to the model provider.
	t.Run("userinfo is stripped", func(t *testing.T) {
		got, err := normalizeURL("https://u:p@example.com/x")
		if err != nil {
			t.Fatalf("normalizeURL returned error: %v", err)
		}
		if got.User != nil {
			t.Errorf("User = %v, want nil", got.User)
		}
		if strings.Contains(got.String(), "u:p") {
			t.Errorf("normalized URL %q still carries credentials", got.String())
		}
	})
}

// ---------------------------------------------------------------------------
// isLocalHost
// ---------------------------------------------------------------------------

func TestIsLocalHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"api.localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"10.1.2.3", true},
		{"172.16.0.1", true},
		{"192.168.1.10", true},
		{"169.254.10.1", true},

		{"example.com", false},
		{"8.8.8.8", false},
		{"172.32.0.1", false}, // outside RFC1918's 172.16/12
		{"notlocalhost.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := isLocalHost(tt.host); got != tt.want {
				t.Errorf("isLocalHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sameHost
// ---------------------------------------------------------------------------

func TestSameHost(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "identical", a: "https://example.com/a", b: "https://example.com/b", want: true},
		{name: "www added", a: "https://example.com/", b: "https://www.example.com/", want: true},
		{name: "www removed", a: "https://www.example.com/", b: "https://example.com/", want: true},

		// The load-bearing one: a port change is not a trust boundary, so a
		// dev server bouncing 3000 -> 8080 must be followed.
		{name: "different port is still the same host", a: "http://localhost:3000/", b: "http://localhost:8080/", want: true},

		{name: "different subdomain", a: "https://example.com/", b: "https://api.example.com/", want: false},
		{name: "different TLD", a: "https://example.com/", b: "https://example.org/", want: false},
		{name: "www of a different host", a: "https://www.example.com/", b: "https://www.evil.com/", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sameHost(mustParseURL(t, tt.a), mustParseURL(t, tt.b))
			if got != tt.want {
				t.Errorf("sameHost(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// htmlToMarkdown
// ---------------------------------------------------------------------------

func TestHTMLToMarkdownHeadings(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{name: "h1", html: "<h1>Alpha</h1>", want: "# Alpha"},
		{name: "h2", html: "<h2>Alpha</h2>", want: "## Alpha"},
		{name: "h3", html: "<h3>Alpha</h3>", want: "### Alpha"},
		{name: "h4", html: "<h4>Alpha</h4>", want: "#### Alpha"},
		{name: "h5", html: "<h5>Alpha</h5>", want: "##### Alpha"},
		{name: "h6", html: "<h6>Alpha</h6>", want: "###### Alpha"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toMarkdown(t, tt.html, "https://example.com/page")
			if !hasLine(got, tt.want) {
				t.Errorf("htmlToMarkdown(%q) = %q, want a line %q", tt.html, got, tt.want)
			}
		})
	}
}

func TestHTMLToMarkdown(t *testing.T) {
	const page = "https://example.com/docs/guide.html"

	// A heading's children are recursed, so a link inside a heading is still
	// a link the agent can follow.
	t.Run("a link inside a heading survives as a link", func(t *testing.T) {
		got := toMarkdown(t, `<h2>See <a href="https://example.com/api">the API</a></h2>`, page)
		if !hasLine(got, "## See [the API](https://example.com/api)") {
			t.Errorf("got %q, want the heading to keep its link", got)
		}
	})

	t.Run("a relative href is resolved against the page URL", func(t *testing.T) {
		got := toMarkdown(t, `<p><a href="../api/ref.html">Ref</a></p>`, page)
		if !strings.Contains(got, "[Ref](https://example.com/api/ref.html)") {
			t.Errorf("got %q, want the href resolved to an absolute URL", got)
		}
	})

	t.Run("an anchor without an href renders as its children", func(t *testing.T) {
		got := toMarkdown(t, `<p><a>plain</a></p>`, page)
		if !strings.Contains(got, "plain") {
			t.Errorf("got %q, want the anchor text", got)
		}
		if strings.Contains(got, "[plain]") {
			t.Errorf("got %q, want no link syntax for an href-less anchor", got)
		}
	})

	// script/style/noscript bodies are markup-shaped noise; a regexp
	// tag-stripper leaks them, a tree walk must not.
	t.Run("script style and noscript bodies are dropped", func(t *testing.T) {
		got := toMarkdown(t, `<html><head><style>body{color:crimson}</style></head>`+
			`<body><script>var x = "<div>trackerpixel</div>";</script>`+
			`<noscript>enablejs</noscript><p>real content</p></body></html>`, page)

		for _, leak := range []string{"crimson", "trackerpixel", "var x", "enablejs"} {
			if strings.Contains(got, leak) {
				t.Errorf("got %q, want %q dropped", got, leak)
			}
		}
		if !strings.Contains(got, "real content") {
			t.Errorf("got %q, want the real content kept", got)
		}
	})

	t.Run("head is skipped except for the title", func(t *testing.T) {
		got := toMarkdown(t, `<html><head><title>Page Title</title>`+
			`<meta name="description" content="hiddenmeta"></head>`+
			`<body><p>body text</p></body></html>`, page)

		if firstLine(got) != "# Page Title" {
			t.Errorf("first line = %q, want %q", firstLine(got), "# Page Title")
		}
		if strings.Contains(got, "hiddenmeta") {
			t.Errorf("got %q, want head metadata dropped", got)
		}
	})

	t.Run("a document with no title gets no leading heading", func(t *testing.T) {
		got := toMarkdown(t, `<html><body><p>just body</p></body></html>`, page)
		if strings.HasPrefix(strings.TrimSpace(got), "#") {
			t.Errorf("got %q, want no invented leading heading", got)
		}
		if strings.TrimSpace(got) != "just body" {
			t.Errorf("got %q, want %q", got, "just body")
		}
	})

	// Collapsing whitespace has to happen at walk time; a post-pass over the
	// finished string can no longer tell a <pre> region from prose.
	t.Run("runs of spaces and tabs collapse in prose", func(t *testing.T) {
		got := toMarkdown(t, "<p>a    b\t\tc</p>", page)
		if !strings.Contains(got, "a b c") {
			t.Errorf("got %q, want collapsed whitespace", got)
		}
	})

	t.Run("pre preserves internal whitespace and blank lines", func(t *testing.T) {
		got := toMarkdown(t, "<pre>line one\n    indented\n\nafter blank</pre>", page)
		if !strings.Contains(got, "line one\n    indented\n\nafter blank") {
			t.Errorf("got %q, want the pre body verbatim", got)
		}
		if !strings.Contains(got, "```") {
			t.Errorf("got %q, want a code fence", got)
		}
	})

	// <pre><code> is the universal code-block markup: it must yield one
	// fence, not a fence plus inline backticks around everything.
	t.Run("pre code produces one fence and no stray backticks", func(t *testing.T) {
		got := toMarkdown(t, "<pre><code>fmt.Println(\"hi\")\n</code></pre>", page)

		if n := strings.Count(got, "```"); n != 2 {
			t.Errorf("got %q, want exactly 2 fence markers, found %d", got, n)
		}
		if rest := strings.ReplaceAll(got, "```", ""); strings.Contains(rest, "`") {
			t.Errorf("got %q, want no backticks outside the fence", got)
		}
		if !strings.Contains(got, `fmt.Println("hi")`) {
			t.Errorf("got %q, want the code body", got)
		}
	})

	t.Run("inline code outside pre is backticked", func(t *testing.T) {
		got := toMarkdown(t, "<p>call <code>Println</code> now</p>", page)
		if !strings.Contains(got, "`Println`") {
			t.Errorf("got %q, want inline code backticked", got)
		}
	})

	// A body already containing ``` would close the fence early and turn the
	// rest of the page into "code".
	t.Run("a pre containing a fence gets a longer fence", func(t *testing.T) {
		got := toMarkdown(t, "<pre>here is ``` inside</pre>", page)
		if !strings.Contains(got, "````") {
			t.Errorf("got %q, want a lengthened fence", got)
		}
	})

	t.Run("nested lists indent two spaces per level", func(t *testing.T) {
		got := toMarkdown(t, "<ul><li>outer<ul><li>inner<ul><li>deepest</li></ul></li></ul></li></ul>", page)

		if !hasIndentedLine(got, "- outer") {
			t.Errorf("got %q, want an unindented %q", got, "- outer")
		}
		if !hasIndentedLine(got, "  - inner") {
			t.Errorf("got %q, want %q indented two spaces", got, "- inner")
		}
		if !hasIndentedLine(got, "    - deepest") {
			t.Errorf("got %q, want %q indented four spaces", got, "- deepest")
		}
	})

	t.Run("img with alt keeps the alt text", func(t *testing.T) {
		got := toMarkdown(t, `<p>before <img src="/d.png" alt="a diagram"> after</p>`, page)
		if !strings.Contains(got, "a diagram") {
			t.Errorf("got %q, want the alt text", got)
		}
	})

	t.Run("img without alt is dropped entirely", func(t *testing.T) {
		got := toMarkdown(t, `<p>before <img src="/spacer-pixel.png"> after</p>`, page)
		if strings.Contains(got, "spacer-pixel") {
			t.Errorf("got %q, want the alt-less image dropped", got)
		}
	})

	t.Run("a table row renders as pipe-separated cells", func(t *testing.T) {
		got := toMarkdown(t, "<table><tr><th>a</th><th>b</th></tr><tr><td>c</td><td>d</td></tr></table>", page)
		if !hasLine(got, "a | b") {
			t.Errorf("got %q, want a header row %q", got, "a | b")
		}
		if !hasLine(got, "c | d") {
			t.Errorf("got %q, want a body row %q", got, "c | d")
		}
	})

	// Real pages are not well-formed. The parser is the whole reason this
	// helper exists rather than a regexp.
	t.Run("malformed html with unclosed tags still yields text", func(t *testing.T) {
		got := toMarkdown(t, "<html><body><p>alpha<p>beta<div>gamma<ul><li>one<li>two<b>bold", page)
		for _, want := range []string{"alpha", "beta", "gamma", "one", "two", "bold"} {
			if !strings.Contains(got, want) {
				t.Errorf("got %q, want it to contain %q", got, want)
			}
		}
		if strings.Contains(got, "<p") || strings.Contains(got, "<div") {
			t.Errorf("got %q, want no raw tags", got)
		}
	})

	t.Run("entities are decoded", func(t *testing.T) {
		got := toMarkdown(t, "<p>Tom &amp; Jerry &lt;3 &quot;quoted&quot;</p>", page)
		if !strings.Contains(got, `Tom & Jerry <3 "quoted"`) {
			t.Errorf("got %q, want decoded entities", got)
		}
	})

	t.Run("three or more blank lines collapse to one", func(t *testing.T) {
		got := toMarkdown(t, "<p>a</p><div></div><div></div><div></div><p>b</p>", page)
		if strings.Contains(got, "\n\n\n") {
			t.Errorf("got %q, want runs of newlines collapsed", got)
		}
		if strings.HasPrefix(got, "\n") || strings.HasSuffix(got, "\n") {
			t.Errorf("got %q, want the result trimmed", got)
		}
	})
}

// ---------------------------------------------------------------------------
// extractText
// ---------------------------------------------------------------------------

func TestExtractText(t *testing.T) {
	page := mustParseURL(t, "https://example.com/doc")

	// Bodies that are handed back untouched, and the types that are refused.
	t.Run("passthrough and refusal", func(t *testing.T) {
		tests := []struct {
			name       string
			ct         string
			body       string
			want       string
			wantErr    bool
			errMustSay string
		}{
			{name: "application/json", ct: "application/json", body: `{"a":1,"b":[2,3]}`, want: `{"a":1,"b":[2,3]}`},
			{name: "text/plain", ct: "text/plain", body: "hello <b>world</b>", want: "hello <b>world</b>"},
			{name: "text/plain with charset", ct: "text/plain; charset=utf-8", body: "plain", want: "plain"},
			{name: "application/xml", ct: "application/xml", body: "<root><a>1</a></root>", want: "<root><a>1</a></root>"},
			{name: "any +json suffix", ct: "application/ld+json", body: `{"@context":"x"}`, want: `{"@context":"x"}`},
			{name: "any +xml suffix", ct: "application/atom+xml", body: "<feed/>", want: "<feed/>"},
			{name: "text/csv", ct: "text/csv", body: "a,b\n1,2", want: "a,b\n1,2"},

			// An absent or broken Content-Type is treated as text/plain, so an
			// HTML-looking body comes back verbatim rather than parsed.
			{name: "empty content type is text/plain", ct: "", body: "<p>raw</p>", want: "<p>raw</p>"},
			{name: "unparseable content type is text/plain", ct: "totally bogus", body: "<p>raw</p>", want: "<p>raw</p>"},

			{name: "image/png errors naming the type", ct: "image/png", body: "\x89PNG\r\n", wantErr: true, errMustSay: "image/png"},
			{name: "application/pdf errors naming the type", ct: "application/pdf", body: "%PDF-1.4", wantErr: true, errMustSay: "application/pdf"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := extractText(tt.ct, []byte(tt.body), page)

				if tt.wantErr {
					if err == nil {
						t.Fatalf("extractText(%q, ...) = %q, want an error", tt.ct, got)
					}
					if tt.errMustSay != "" && !strings.Contains(err.Error(), tt.errMustSay) {
						t.Errorf("error %v does not name the media type %q", err, tt.errMustSay)
					}
					return
				}

				if err != nil {
					t.Fatalf("extractText(%q, ...) returned error: %v", tt.ct, err)
				}
				if got != tt.want {
					t.Errorf("extractText(%q, ...) = %q, want %q", tt.ct, got, tt.want)
				}
			})
		}
	})

	// Types that must take the HTML path. application/xhtml+xml is the trap:
	// it matches the +xml passthrough rule too, so only the ordering keeps it
	// on the markdown path.
	t.Run("html path", func(t *testing.T) {
		const body = `<html><head><title>Doc</title></head><body><h2>Heading</h2><p>a &amp; b</p></body></html>`

		for _, ct := range []string{"text/html", "text/html; charset=utf-8", "application/xhtml+xml"} {
			t.Run(ct, func(t *testing.T) {
				got, err := extractText(ct, []byte(body), page)
				if err != nil {
					t.Fatalf("extractText(%q, ...) returned error: %v", ct, err)
				}
				if got == body {
					t.Fatalf("extractText(%q, ...) passed the body through untouched, want the HTML path", ct)
				}
				if !hasLine(got, "# Doc") {
					t.Errorf("got %q, want the title as a leading heading", got)
				}
				if !hasLine(got, "## Heading") {
					t.Errorf("got %q, want the h2 converted", got)
				}
				if !strings.Contains(got, "a & b") {
					t.Errorf("got %q, want the entity decoded", got)
				}
				if strings.Contains(got, "<h2") {
					t.Errorf("got %q, want no raw tags", got)
				}
			})
		}
	})
}

// ---------------------------------------------------------------------------
// capText
// ---------------------------------------------------------------------------

func TestCapText(t *testing.T) {
	t.Run("under the cap is unchanged", func(t *testing.T) {
		in := strings.Repeat("a", maxFetchOutput-1)
		if got := capText(in); got != in {
			t.Errorf("capText changed a %d-byte string (got %d bytes)", len(in), len(got))
		}
	})

	t.Run("exactly at the cap is unchanged", func(t *testing.T) {
		in := strings.Repeat("a", maxFetchOutput)
		if got := capText(in); got != in {
			t.Errorf("capText changed a string exactly at the cap")
		}
	})

	// Truncating in silence would have the model reason confidently about a
	// page it only half saw. The note has to say how much went missing and
	// where the rest lives.
	t.Run("over the cap is trimmed with a note and a spill file", func(t *testing.T) {
		const extra = 5000
		in := strings.Repeat("x", maxFetchOutput+extra)

		got := capText(in)

		if !strings.HasPrefix(got, in[:1000]) {
			t.Errorf("capText dropped the start of the text")
		}
		if len(got) <= maxFetchOutput {
			t.Errorf("result is %d bytes, want the capped text plus a note", len(got))
		}
		if !strings.Contains(got, strconv.Itoa(extra)) {
			t.Errorf("result %q does not report the %d dropped bytes", got[maxFetchOutput:], extra)
		}

		path := regexp.MustCompile(regexp.QuoteMeta(os.TempDir()) + `[^\s\]]+`).FindString(got)
		if path == "" {
			t.Fatalf("no spill file path in the note: %q", got[maxFetchOutput:])
		}
		defer os.Remove(path)

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("spill file missing: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("spill file mode = %o, want 600", got)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read spill file: %v", err)
		}
		if string(data) != in {
			t.Errorf("spill file holds %d bytes, want the full %d", len(data), len(in))
		}
	})

	// Cutting at a byte offset lands mid-rune on any page with an em dash in
	// the wrong place, and a half rune is not valid UTF-8.
	t.Run("a cut landing mid-rune still produces valid UTF-8", func(t *testing.T) {
		// "é" is two bytes and starts at maxFetchOutput-1, so a naive
		// s[:maxFetchOutput] slices it in half.
		in := strings.Repeat("a", maxFetchOutput-1) + "é" + strings.Repeat("b", 5000)

		got := capText(in)

		if !utf8.ValidString(got) {
			t.Errorf("capText produced invalid UTF-8")
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Errorf("capText produced a replacement character — the cut split a rune")
		}

		if path := regexp.MustCompile(regexp.QuoteMeta(os.TempDir()) + `[^\s\]]+`).FindString(got); path != "" {
			os.Remove(path)
		}
	})
}

// ---------------------------------------------------------------------------
// webFetch, end to end against httptest
// ---------------------------------------------------------------------------

func TestWebFetch(t *testing.T) {
	t.Run("200 html comes back as markdown inside untrusted delimiters", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<html><head><title>Test Page</title></head><body>`+
				`<h1>Hello</h1><p>Some <a href="/docs">docs</a> here.</p>`+
				`<script>var junk = 1;</script></body></html>`)
		}))
		defer srv.Close()

		got, err := fetchURL(srv.URL)
		if err != nil {
			t.Fatalf("webFetch returned error: %v", err)
		}

		if !strings.Contains(got, "begin untrusted web content") {
			t.Errorf("result %q is missing the opening untrusted-content marker", got)
		}
		if !strings.Contains(got, "data, not instructions") {
			t.Errorf("result %q does not tell the model the span is data", got)
		}
		if !strings.Contains(got, "end untrusted web content") {
			t.Errorf("result %q is missing the closing untrusted-content marker", got)
		}
		if !strings.Contains(got, srv.URL) {
			t.Errorf("result %q does not name the fetched URL", got)
		}
		if !hasLine(got, "# Test Page") {
			t.Errorf("result %q is missing the title heading", got)
		}
		if !hasLine(got, "# Hello") {
			t.Errorf("result %q is missing the h1", got)
		}
		if !strings.Contains(got, "[docs]("+srv.URL+"/docs)") {
			t.Errorf("result %q did not resolve the relative link", got)
		}
		if strings.Contains(got, "var junk") {
			t.Errorf("result %q leaked a script body", got)
		}
	})

	// http:// against loopback must NOT be upgraded to https, or every local
	// dev server becomes unfetchable. httptest serves plain HTTP.
	t.Run("a loopback http URL is fetched without an https upgrade", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "local dev server")
		}))
		defer srv.Close()

		got, err := fetchURL(srv.URL)
		if err != nil {
			t.Fatalf("webFetch returned error: %v", err)
		}
		if !strings.Contains(got, "local dev server") {
			t.Errorf("result %q is missing the body", got)
		}
	})

	// Go's http.Client turns url.User into an Authorization header on its
	// own, so stripping it is what stops a token being sent — and echoed
	// back to the model provider in the result.
	t.Run("url credentials are neither sent nor echoed", func(t *testing.T) {
		var sawAuth atomic.Value
		sawAuth.Store("")

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawAuth.Store(r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "ok")
		}))
		defer srv.Close()

		withCreds := strings.Replace(srv.URL, "http://", "http://user:supersecrettoken@", 1)

		got, err := fetchURL(withCreds)
		if err != nil {
			t.Fatalf("webFetch returned error: %v", err)
		}
		if auth := sawAuth.Load().(string); auth != "" {
			t.Errorf("server saw Authorization: %q, want none", auth)
		}
		if strings.Contains(got, "supersecrettoken") {
			t.Errorf("result echoed the credential back: %q", got)
		}
	})

	// Unlike bash, a failed fetch produced nothing worth reading, so it is a
	// Go error rather than a result.
	t.Run("404 is an error naming the status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()

		got, err := fetchURL(srv.URL)
		if err == nil {
			t.Fatalf("webFetch succeeded on a 404: %q", got)
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("error %v does not name the status", err)
		}
	})

	t.Run("a same-host redirect is followed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/end" {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, "<p>landed here</p>")
				return
			}
			http.Redirect(w, r, "/end", http.StatusFound)
		}))
		defer srv.Close()

		got, err := fetchURL(srv.URL + "/start")
		if err != nil {
			t.Fatalf("webFetch returned error: %v", err)
		}
		if !strings.Contains(got, "landed here") {
			t.Errorf("result %q — the redirect was not followed", got)
		}
	})

	// Ports are irrelevant to sameHost: 127.0.0.1:A -> 127.0.0.1:B is the
	// same host and must be followed rather than reported.
	t.Run("a redirect to another port on the same host is followed", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<p>other port</p>")
		}))
		defer target.Close()

		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/", http.StatusFound)
		}))
		defer origin.Close()

		got, err := fetchURL(origin.URL)
		if err != nil {
			t.Fatalf("webFetch returned error: %v", err)
		}
		if !strings.Contains(got, "other port") {
			t.Errorf("result %q — a port change was treated as a different host", got)
		}
	})

	// A URL the model chose is a decision it made; landing somewhere else
	// silently is not. example.invalid is guaranteed not to resolve, which
	// is exactly why it is safe here: nothing is ever requested from it.
	t.Run("a cross-host redirect is reported, not followed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://example.invalid/landing", http.StatusFound)
		}))
		defer srv.Close()

		got, err := fetchURL(srv.URL)
		if err != nil {
			t.Fatalf("webFetch returned error for a cross-host redirect: %v — it should be a result", err)
		}
		if !strings.Contains(got, "example.invalid") {
			t.Errorf("result %q does not name the redirect target", got)
		}
		if !strings.Contains(strings.ToLower(got), "redirect") {
			t.Errorf("result %q does not explain that this was a redirect", got)
		}
	})

	// A CheckRedirect that returns nil REPLACES Go's built-in hop limit, so
	// without an explicit count this loops until the process is killed. The
	// watchdog is the assertion.
	t.Run("a redirect loop stops at maxRedirects instead of hanging", func(t *testing.T) {
		var hits atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := hits.Add(1)
			http.Redirect(w, r, fmt.Sprintf("/hop/%d", n), http.StatusFound)
		}))
		defer srv.Close()

		type outcome struct {
			out string
			err error
		}
		done := make(chan outcome, 1)
		go func() {
			out, err := fetchURL(srv.URL + "/hop/0")
			done <- outcome{out, err}
		}()

		select {
		case res := <-done:
			if res.err == nil {
				t.Fatalf("webFetch succeeded on an endless redirect loop: %q", res.out)
			}
			if !strings.Contains(strings.ToLower(res.err.Error()), "redirect") {
				t.Errorf("error %v does not mention redirects", res.err)
			}
			if n := hits.Load(); n > maxRedirects+2 {
				t.Errorf("server was hit %d times, want the limit to bite around %d", n, maxRedirects)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("webFetch never returned after %d redirects — the hop limit is not enforced", hits.Load())
		}
	})

	// 304 and 300 carry no Location, so resp.Location() fails and they must
	// fall through to the non-2xx error path rather than being treated as a
	// redirect to nowhere.
	t.Run("a 304 with no Location takes the error path", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotModified)
		}))
		defer srv.Close()

		got, err := fetchURL(srv.URL)
		if err == nil {
			t.Fatalf("webFetch succeeded on a 304: %q", got)
		}
		if !strings.Contains(err.Error(), "304") {
			t.Errorf("error %v does not name the status", err)
		}
	})

	// Half an HTML tree parses into confidently wrong text, so an oversized
	// body is refused rather than truncated.
	t.Run("a body over maxFetchBytes is refused", func(t *testing.T) {
		big := bytes.Repeat([]byte("a"), maxFetchBytes+1)

		t.Run("with a declared Content-Length", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.Header().Set("Content-Length", strconv.Itoa(len(big)))
				w.Write(big)
			}))
			defer srv.Close()

			got, err := fetchURL(srv.URL)
			if err == nil {
				t.Fatalf("webFetch accepted a %d-byte body: %d bytes returned", len(big), len(got))
			}
		})

		// A server can lie or omit the length, so the LimitReader has to
		// stand on its own. Chunked encoding leaves ContentLength at -1.
		t.Run("with no declared Content-Length", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				flusher, ok := w.(http.Flusher)
				if !ok {
					return
				}
				chunk := bytes.Repeat([]byte("a"), 1<<20)
				for i := 0; i < 11; i++ {
					if _, err := w.Write(chunk); err != nil {
						return
					}
					flusher.Flush()
				}
			}))
			defer srv.Close()

			got, err := fetchURL(srv.URL)
			if err == nil {
				t.Fatalf("webFetch accepted an undeclared oversized body: %d bytes returned", len(got))
			}
		})
	})

	// A single-page app parses to nothing. The fix is a different URL, and
	// the error has to say so or the model just retries.
	t.Run("a page of only script gets the JavaScript hint", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><head></head><body><div id="root"></div>`+
				`<script>window.app = "everything is rendered here";</script></body></html>`)
		}))
		defer srv.Close()

		got, err := fetchURL(srv.URL)
		if err == nil {
			t.Fatalf("webFetch succeeded on a JavaScript-rendered page: %q", got)
		}
		if !strings.Contains(err.Error(), "JavaScript") {
			t.Errorf("error %v does not hint that the page is JavaScript-rendered", err)
		}
	})

	// The hint is HTML-only: an empty JSON body is not a JS problem.
	t.Run("an empty json body reports an empty response without the JS hint", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		got, err := fetchURL(srv.URL)
		if err == nil {
			t.Fatalf("webFetch succeeded on an empty body: %q", got)
		}
		if !strings.Contains(err.Error(), "empty response body") {
			t.Errorf("error %v does not report an empty body", err)
		}
		if strings.Contains(err.Error(), "JavaScript") {
			t.Errorf("error %v gives the JavaScript hint outside the HTML path", err)
		}
	})

	// fetchTimeout is a var precisely so this case does not take 30 seconds.
	t.Run("a hanging server times out", func(t *testing.T) {
		original := fetchTimeout
		fetchTimeout = 100 * time.Millisecond
		t.Cleanup(func() { fetchTimeout = original })

		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		defer srv.Close()
		defer close(release)

		start := time.Now()
		got, err := fetchURL(srv.URL)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatalf("webFetch succeeded against a hanging server: %q", got)
		}
		if elapsed > 5*time.Second {
			t.Errorf("webFetch took %s — fetchTimeout was not honoured", elapsed)
		}
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "timed out") && !strings.Contains(msg, "timeout") && !strings.Contains(msg, "deadline") {
			t.Errorf("error %v does not report a timeout", err)
		}
	})

	t.Run("malformed arguments error", func(t *testing.T) {
		if got, err := webFetch(context.Background(), "not json"); err == nil {
			t.Fatalf("webFetch succeeded on malformed arguments: %q", got)
		}
	})

	t.Run("an empty url errors", func(t *testing.T) {
		if got, err := fetchURL("   "); err == nil {
			t.Fatalf("webFetch succeeded on an empty url: %q", got)
		}
	})
}

// The registry is the only wiring web_fetch needs, and it is ungated on
// purpose — it cannot damage the filesystem.
func TestWebFetchIsRegisteredUngated(t *testing.T) {
	for _, tool := range all {
		if tool.Schema.Function.Name != "web_fetch" {
			continue
		}
		if tool.NeedsApproval {
			t.Errorf("web_fetch is gated, want it ungated like read_file")
		}
		if tool.Execute == nil {
			t.Fatal("web_fetch has no Execute function")
		}
		req := tool.Schema.Function.Parameters.Required
		if len(req) != 1 || req[0] != "url" {
			t.Errorf("required parameters = %v, want [url]", req)
		}
		if _, ok := tool.Schema.Function.Parameters.Properties["url"]; !ok {
			t.Errorf("schema has no url property")
		}
		return
	}
	t.Fatal("web_fetch is not in the tool registry")
}
