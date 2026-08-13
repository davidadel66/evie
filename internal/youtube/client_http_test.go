package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientUsesProvidedClientAndDefaultsToThirtySeconds(t *testing.T) {
	provided := &http.Client{Timeout: 73 * time.Millisecond}
	if got, ok := retainedHTTPTimeout(NewClient(provided)); !ok || got != provided.Timeout {
		t.Fatalf("NewClient(provided) timeout = %s (found=%t), want %s", got, ok, provided.Timeout)
	}
	if got, ok := retainedHTTPTimeout(NewClient(nil)); !ok || got != 30*time.Second {
		t.Errorf("NewClient(nil) timeout = %s (found=%t), want 30s", got, ok)
	}
}

func TestClientGetEnforcesBodyLimitAtTenMiB(t *testing.T) {
	const limit = 10 << 20
	for _, tc := range []struct {
		name    string
		size    int
		chunked bool
		wantErr bool
	}{
		{"exact boundary", limit, false, false},
		{"declared oversized", limit + 1, false, true},
		{"chunked oversized", limit + 1, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := bytes.Repeat([]byte("x"), tc.size)
				if tc.chunked {
					w.WriteHeader(http.StatusOK)
					flusher := w.(http.Flusher)
					for len(body) > 0 {
						n := 64 << 10
						if len(body) < n {
							n = len(body)
						}
						if _, err := w.Write(body[:n]); err != nil {
							return
						}
						flusher.Flush()
						body = body[n:]
					}
					return
				}
				w.Header().Set("Content-Length", fmt.Sprint(len(body)))
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			got, err := NewClient(srv.Client()).get(context.Background(), srv.URL, false)
			if tc.wantErr && err == nil {
				t.Fatalf("accepted %d-byte body (%d bytes returned)", tc.size, len(got))
			}
			if !tc.wantErr && (err != nil || len(got) != tc.size) {
				t.Fatalf("boundary body = %d bytes, error %v; want %d", len(got), err, tc.size)
			}
			if tc.wantErr && !containsAny(strings.ToLower(err.Error()), "10 mib", "10mb", "limit", "large") {
				t.Errorf("oversize error is not actionable: %v", err)
			}
		})
	}
}

func TestClientRetriesOnly429AndServerFailures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		retryAfter string
		failures   int
		wantHits   int64
		wantSleeps []time.Duration
	}{
		{"429 uses five and ten seconds", http.StatusTooManyRequests, "", 3, 3, []time.Duration{5 * time.Second, 10 * time.Second}},
		{"500 uses one and two seconds", http.StatusInternalServerError, "", 3, 3, []time.Duration{time.Second, 2 * time.Second}},
		{"valid Retry-After wins", http.StatusTooManyRequests, "17", 1, 2, []time.Duration{17 * time.Second}},
		{"Retry-After is capped", http.StatusServiceUnavailable, "999", 1, 2, []time.Duration{30 * time.Second}},
		{"invalid Retry-After falls back", http.StatusTooManyRequests, "invalid", 1, 2, []time.Duration{5 * time.Second}},
		{"ordinary 4xx is not retried", http.StatusBadRequest, "", 3, 1, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := int(hits.Add(1))
				if n <= tc.failures {
					if tc.retryAfter != "" {
						w.Header().Set("Retry-After", tc.retryAfter)
					}
					w.WriteHeader(tc.status)
					return
				}
				fmt.Fprint(w, "ok")
			}))
			defer srv.Close()

			client := NewClient(srv.Client())
			var sleeps []time.Duration
			client.sleep = func(ctx context.Context, delay time.Duration) error {
				sleeps = append(sleeps, delay)
				return nil
			}
			body, err := client.get(context.Background(), srv.URL, false)
			if hits.Load() != tc.wantHits {
				t.Errorf("requests = %d, want %d", hits.Load(), tc.wantHits)
			}
			if !equalDurations(sleeps, tc.wantSleeps) {
				t.Errorf("sleeps = %v, want %v", sleeps, tc.wantSleeps)
			}
			if tc.failures < int(tc.wantHits) && (err != nil || string(body) != "ok") {
				t.Errorf("eventual success = %q, %v", body, err)
			}
			if tc.failures >= int(tc.wantHits) && err == nil {
				t.Fatalf("status %d unexpectedly succeeded", tc.status)
			}
			if tc.status == http.StatusTooManyRequests && tc.failures >= int(tc.wantHits) {
				message := strings.ToLower(err.Error())
				if !strings.Contains(message, "rate limit") && !strings.Contains(message, "429") {
					t.Errorf("exhausted 429 error %q does not identify the rate limit", err)
				}
			}
		})
	}
}

func TestClientTimeoutHonorsHTTPClientAndOperationContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	for _, tc := range []struct {
		name   string
		client *http.Client
		ctx    func() (context.Context, context.CancelFunc)
	}{
		{"client timeout", &http.Client{Timeout: 50 * time.Millisecond}, func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) }},
		{"operation deadline", srv.Client(), func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), 50*time.Millisecond)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.ctx()
			defer cancel()
			start := time.Now()
			_, err := NewClient(tc.client).get(ctx, srv.URL, false)
			if err == nil {
				t.Fatal("hanging request succeeded")
			}
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Errorf("timeout took %s", elapsed)
			}
			if !containsAny(strings.ToLower(err.Error()), "timeout", "timed out", "deadline") {
				t.Errorf("error %q does not identify timeout", err)
			}
		})
	}
}

func TestClientDetectsConsentRecaptchaAndBotPagesSeparately(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"consent redirect form", `<form action="https://consent.youtube.com/save"><input name="continue"></form>`, "consent"},
		{"recaptcha", `<div class="g-recaptcha" data-sitekey="fixture"></div>`, "recaptcha"},
		{"bot confirmation", `<h1>Sign in to confirm you're not a bot</h1>`, "bot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
			defer srv.Close()
			_, err := NewClient(srv.Client()).get(context.Background(), srv.URL, false)
			if err == nil {
				t.Fatal("blocking page was accepted")
			}
			message := strings.ToLower(err.Error())
			if !strings.Contains(message, tc.want) {
				t.Errorf("error %q does not classify %s", err, tc.want)
			}
			for _, other := range []string{"consent", "recaptcha", "bot"} {
				if other != tc.want && strings.Contains(message, other) {
					t.Errorf("error %q collapses %s into %s", err, tc.want, other)
				}
			}
		})
	}
}

func TestClientDoesNotTreatRecaptchaScriptMetadataAsAChallenge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<script>var integrationName = "recaptcha";</script><main>normal YouTube page</main>`)
	}))
	defer srv.Close()

	body, err := NewClient(srv.Client()).get(context.Background(), srv.URL, false)
	if err != nil {
		t.Fatalf("normal page mentioning recaptcha metadata was blocked: %v", err)
	}
	if !strings.Contains(string(body), "normal YouTube page") {
		t.Fatalf("normal page body was lost: %q", body)
	}
}

func TestClientDetectsStructuralRecaptchaChallenges(t *testing.T) {
	for _, body := range []string{
		`<form action="https://www.google.com/recaptcha/api2/anchor"><input name="continue"></form>`,
		`<form action="/sorry/index"><input name="g-recaptcha-response"></form>`,
		`<iframe src="https://www.google.com/recaptcha/api2/anchor?k=fixture"></iframe>`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		_, err := NewClient(srv.Client()).get(context.Background(), srv.URL, false)
		srv.Close()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "recaptcha") {
			t.Errorf("structural challenge %q error = %v", body, err)
		}
	}
}

func TestClientDetectsRecaptchaChallengeURL(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://www.google.com/recaptcha/api2/anchor?k=fixture", nil)
	response := &http.Response{Request: request, Header: make(http.Header)}
	if err := detectBlockingResponse(response, []byte("challenge")); err == nil || !strings.Contains(strings.ToLower(err.Error()), "recaptcha") {
		t.Errorf("recaptcha challenge URL error = %v", err)
	}

	request = httptest.NewRequest(http.MethodGet, "https://www.youtube.com/watch?v=AAAAAAAAAAA&metadata=recaptcha", nil)
	response.Request = request
	if err := detectBlockingResponse(response, []byte(`<script>const provider = "recaptcha";</script>`)); err != nil {
		t.Errorf("incidental recaptcha URL/body metadata was blocked: %v", err)
	}
}

func TestClientReportsConsentRedirectWithoutFollowingIt(t *testing.T) {
	var followed atomic.Bool
	base := http.DefaultTransport
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "consent.youtube.com" {
			followed.Store(true)
			return nil, fmt.Errorf("consent destination must not be requested")
		}
		return base.RoundTrip(request)
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://consent.youtube.com/m?continue=youtube", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient(&http.Client{Transport: transport}).get(context.Background(), srv.URL, false)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "consent") {
		t.Fatalf("consent redirect error = %v", err)
	}
	if followed.Load() {
		t.Fatal("client followed consent redirect")
	}
}

func TestClientRestrictsCaptionHostsBeforeRequest(t *testing.T) {
	var allowedHits, forbiddenHits atomic.Int64
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { allowedHits.Add(1); fmt.Fprint(w, `{}`) }))
	defer allowed.Close()
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forbiddenHits.Add(1); fmt.Fprint(w, `{}`) }))
	defer forbidden.Close()

	allowedClient := allowed.Client()
	allowedClient.Transport = rewriteTransport{target: mustURL(t, allowed.URL), base: allowedClient.Transport}
	client := NewClient(allowedClient)
	if _, err := client.get(context.Background(), "https://www.youtube.com/api/timedtext", true); err != nil {
		t.Fatalf("YouTube caption host rejected: %v", err)
	}
	if _, err := client.get(context.Background(), forbidden.URL, true); err == nil {
		t.Fatal("foreign caption host accepted")
	}
	if allowedHits.Load() != 1 || forbiddenHits.Load() != 0 {
		t.Errorf("caption requests: allowed=%d forbidden=%d, want 1 and 0", allowedHits.Load(), forbiddenHits.Load())
	}
	for _, raw := range []string{
		"http://www.youtube.com/api/timedtext",
		"https://example.com/api/timedtext",
		"https://youtube.com.evil.test/api/timedtext",
		"https://user@youtube.com/api/timedtext",
	} {
		if _, err := client.get(context.Background(), raw, true); err == nil {
			t.Errorf("unsafe caption URL %q accepted", raw)
		}
	}
}

func TestClientRevalidatesEveryCaptionRedirect(t *testing.T) {
	for _, target := range []string{
		"http://www.youtube.com/api/timedtext",
		"https://example.com/api/timedtext",
		"https://user@www.youtube.com/api/timedtext",
	} {
		t.Run(target, func(t *testing.T) {
			var destinationHits atomic.Int64
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == "/start" {
					return &http.Response{
						StatusCode: http.StatusFound,
						Header:     http.Header{"Location": []string{target}},
						Body:       http.NoBody,
						Request:    request,
					}, nil
				}
				destinationHits.Add(1)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
			})
			client := NewClient(&http.Client{Transport: transport})
			_, err := client.get(context.Background(), "https://www.youtube.com/start", true)
			if err == nil {
				t.Fatalf("caption redirect to %q succeeded", target)
			}
			if destinationHits.Load() != 0 {
				t.Fatalf("unsafe caption redirect %q was requested", target)
			}
		})
	}
}

func TestClientFetchVideoUsesPinnedAndroidProtocolEndToEnd(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		switch r.URL.Path {
		case "/watch":
			if r.Method != http.MethodGet || r.URL.Query().Get("v") != testVideoID || r.URL.Query().Get("hl") != "en" {
				t.Errorf("watch request = %s %s", r.Method, r.URL.RequestURI())
			}
			if r.Header.Get("Accept-Language") != "en-US,en;q=0.9" || !strings.Contains(r.Header.Get("User-Agent"), "Evie") {
				t.Errorf("watch headers Accept-Language=%q User-Agent=%q", r.Header.Get("Accept-Language"), r.Header.Get("User-Agent"))
			}
			_, _ = w.Write(fixture(t, "watch.html"))
		case "/youtubei/v1/player":
			if r.Method != http.MethodPost || r.URL.Query().Get("key") != "test-api-key" || r.URL.Query().Get("prettyPrint") != "false" {
				t.Errorf("player request = %s %s", r.Method, r.URL.RequestURI())
			}
			if r.Header.Get("X-Youtube-Client-Version") != "20.10.38" || r.Header.Get("X-Youtube-Client-Name") == "" || !strings.Contains(strings.ToLower(r.Header.Get("User-Agent")), "android") {
				t.Errorf("Android headers name=%q version=%q user-agent=%q", r.Header.Get("X-Youtube-Client-Name"), r.Header.Get("X-Youtube-Client-Version"), r.Header.Get("User-Agent"))
			}
			var request struct {
				VideoID string `json:"videoId"`
				Context struct {
					Client struct {
						Name    string `json:"clientName"`
						Version string `json:"clientVersion"`
					} `json:"client"`
				} `json:"context"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode player body: %v", err)
			}
			if request.VideoID != testVideoID || request.Context.Client.Name != "ANDROID" || request.Context.Client.Version != "20.10.38" {
				t.Errorf("player body = %#v", request)
			}
			_, _ = w.Write(fixture(t, "player-tracks.json"))
		case "/api/timedtext":
			if r.URL.Query().Get("fmt") != "json3" || r.URL.Query().Get("sig") != "signed+value" {
				t.Errorf("caption query changed: %q", r.URL.RawQuery)
			}
			_, _ = w.Write(fixture(t, "captions.json3"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	httpClient := srv.Client()
	httpClient.Transport = rewriteTransport{target: mustURL(t, srv.URL), base: httpClient.Transport}
	client := NewClient(httpClient)
	got, err := client.fetchVideo(context.Background(), testVideoID, "en")
	if err != nil {
		t.Fatalf("fetchVideo: %v", err)
	}
	assertReflectedString(t, got, []string{"Text", "Transcript"}, "Hello world\n[Music]\nKeep cues")
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 {
		t.Errorf("requests = %v, want watch, player, caption", requests)
	}
}

func TestClientListChannelSendsPageContextAndStopsAtLimit(t *testing.T) {
	var browseHits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/@FixtureChannel/videos":
			if r.Method != http.MethodGet || r.URL.Query().Get("hl") != "en" {
				t.Errorf("channel page request = %s %s", r.Method, r.URL.RequestURI())
			}
			_, _ = w.Write(fixture(t, "channel-initial.html"))
		case "/youtubei/v1/browse":
			browseHits.Add(1)
			if r.Method != http.MethodPost || r.URL.Query().Get("key") != "browse-key" || r.URL.Query().Get("prettyPrint") != "false" {
				t.Errorf("browse request = %s %s", r.Method, r.URL.RequestURI())
			}
			if r.Header.Get("Origin") != "https://www.youtube.com" || r.Header.Get("X-Goog-Visitor-Id") != "visitor-page" || r.Header.Get("X-Youtube-Client-Version") != "2.20260812.00.00" {
				t.Errorf("browse headers origin=%q visitor=%q version=%q", r.Header.Get("Origin"), r.Header.Get("X-Goog-Visitor-Id"), r.Header.Get("X-Youtube-Client-Version"))
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode browse body: %v", err)
			}
			if body["continuation"] != "token-one" || body["clickTracking"] == nil {
				t.Errorf("browse body lacks continuation/click tracking: %#v", body)
			}
			_, _ = w.Write(fixture(t, "continuation-actions.json"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	httpClient := srv.Client()
	httpClient.Transport = rewriteTransport{target: mustURL(t, srv.URL), base: httpClient.Transport}
	client := NewClient(httpClient)
	got, err := client.listChannel(context.Background(), "@FixtureChannel", 3)
	if err != nil {
		t.Fatalf("listChannel: %v", err)
	}
	assertVideoOrder(t, got, []string{"AAAAAAAAAAA", "BBBBBBBBBBB", "CCCCCCCCCCC"})
	if browseHits.Load() != 1 {
		t.Errorf("browse requests = %d, want 1: client did not stop at positive limit", browseHits.Load())
	}
}

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func (r rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = r.target.Scheme
	clone.URL.Host = r.target.Host
	clone.Host = r.target.Host
	return r.base.RoundTrip(clone)
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return parsed
}

func retainedHTTPTimeout(client *Client) (time.Duration, bool) {
	v := reflect.ValueOf(client)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	typeOfHTTPClient := reflect.TypeOf((*http.Client)(nil))
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Type() == typeOfHTTPClient && !field.IsNil() {
			return time.Duration(field.Elem().FieldByName("Timeout").Int()), true
		}
	}
	return 0, false
}

func equalDurations(got, want []time.Duration) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
