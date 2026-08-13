package youtube

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

const (
	testVideoID   = "dQw4w9WgXcQ"
	testChannelID = "UCaaaaaaaaaaaaaaaaaaaaaa"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func TestDecodeEmbeddedJSONDoesNotStopAtBracesInsideStrings(t *testing.T) {
	tests := []struct {
		name, source, marker string
	}{
		{"assignment", `var ytInitialData = {"text":"contains } and }; safely","nested":{"ok":true}}; trailing`, "var ytInitialData"},
		{"call", `before ytcfg.set({"text":"contains \"quoted }; brace }","nested":{"ok":true}}); after`, "ytcfg.set"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got struct {
				Text   string `json:"text"`
				Nested struct {
					OK bool `json:"ok"`
				} `json:"nested"`
			}
			if err := decodeEmbeddedJSON([]byte(tc.source), tc.marker, &got); err != nil {
				t.Fatalf("decodeEmbeddedJSON: %v", err)
			}
			if !got.Nested.OK || !strings.Contains(got.Text, "};") {
				t.Errorf("decoded value = %#v", got)
			}
		})
	}
}

func TestDecodeEmbeddedJSONRejectsMissingMarkerAndMalformedObject(t *testing.T) {
	for _, tc := range []struct{ name, source, marker string }{
		{"missing marker", `{}`, "ytInitialData"},
		{"missing object", `ytInitialData = null`, "ytInitialData"},
		{"malformed object", `ytInitialData = {"open": true`, "ytInitialData"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dst any
			if err := decodeEmbeddedJSON([]byte(tc.source), tc.marker, &dst); err == nil {
				t.Fatal("decodeEmbeddedJSON succeeded")
			}
		})
	}
}

func TestParseWatchPageExtractsInnertubeKey(t *testing.T) {
	got, err := parseWatchPage(fixture(t, "watch.html"))
	if err != nil {
		t.Fatalf("parseWatchPage: %v", err)
	}
	assertReflectedString(t, got, []string{"APIKey", "InnertubeAPIKey"}, "test-api-key")
	if _, err := parseWatchPage([]byte(`<html>no ytcfg here</html>`)); err == nil {
		t.Fatal("watch page without INNERTUBE_API_KEY succeeded")
	}
}

func TestParseWatchPageIgnoresNonObjectAndJavaScriptYTCfgCalls(t *testing.T) {
	body := []byte(`
		ytcfg.set('EMERGENCY_BASE_URL', '/error_204');
		ytcfg.set({"INNERTUBE_API_KEY":"usable-key"});
		ytcfg.set("initialInnerWidth", window.innerWidth);
		ytcfg.set({"CSI_SERVICE_NAME": 'youtube', "yt_ad": '1',});
	`)
	got, err := parseWatchPage(body)
	if err != nil {
		t.Fatalf("parseWatchPage mixed ytcfg calls: %v", err)
	}
	if got.APIKey != "usable-key" {
		t.Errorf("API key = %q, want usable-key", got.APIKey)
	}
}

func TestParseWatchPageIgnoresQuotedAndCommentedYTCfgMarkers(t *testing.T) {
	body := []byte(`
		const quoted = "ytcfg.set({\"INNERTUBE_API_KEY\":\"quoted-fake\"})";
		// ytcfg.set({"INNERTUBE_API_KEY":"line-comment-fake"});
		ytcfg.set({"INNERTUBE_API_KEY":"real-key"});
		/* ytcfg.set({"INNERTUBE_API_KEY":"block-comment-fake"}); */
		const template = ` + "`ytcfg.set({\"INNERTUBE_API_KEY\":\"template-fake\"})`" + `;
	`)
	got, err := parseWatchPage(body)
	if err != nil {
		t.Fatalf("parseWatchPage lexical scan: %v", err)
	}
	if got.APIKey != "real-key" {
		t.Errorf("API key = %q, want real-key", got.APIKey)
	}
}

func TestParseAndroidPlayerMetadataAndCaptionSelection(t *testing.T) {
	body := fixture(t, "player-tracks.json")
	tests := []struct {
		name, language, wantLanguage, wantSource string
	}{
		{"exact manual beats generated", "en", "en", "manual"},
		{"exact generated is selected", "fr", "fr", "generated"},
		{"regional exact match", "en-US", "en-us", "manual"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePlayerResponse(body, testVideoID, tc.language)
			if err != nil {
				t.Fatalf("parsePlayerResponse: %v", err)
			}
			assertReflectedString(t, got, []string{"VideoID", "YoutubeID", "YouTubeID"}, testVideoID)
			assertReflectedString(t, got, []string{"Title"}, "Fixture video")
			assertReflectedString(t, got, []string{"ChannelID", "ChannelYoutubeID", "ChannelYouTubeID"}, testChannelID)
			assertReflectedString(t, got, []string{"ChannelName", "Author"}, "Fixture Channel")
			assertReflectedString(t, got, []string{"ChannelHandle", "Handle"}, "@FixtureChannel")
			assertReflectedString(t, got, []string{"ChannelURL", "ProfileURL", "OwnerProfileURL"}, "https://www.youtube.com/@FixtureChannel")
			assertReflectedString(t, got, []string{"PublishedAt", "PublishDate"}, "2026-08-01")
			assertReflectedInt(t, got, []string{"DurationSeconds", "Duration"}, 123)
			assertReflectedString(t, got, []string{"LanguageCode", "Language"}, tc.wantLanguage)
			assertReflectedString(t, got, []string{"Source", "CaptionSource"}, tc.wantSource)
		})
	}
}

func TestParseAndroidPlayerUsesLexicalVariantOrderBeforeSourcePreference(t *testing.T) {
	body := mutatedPlayer(t, func(value map[string]any) {
		tracks := value["captions"].(map[string]any)["playerCaptionsTracklistRenderer"].(map[string]any)["captionTracks"].([]any)
		value["captions"].(map[string]any)["playerCaptionsTracklistRenderer"].(map[string]any)["captionTracks"] = tracks[2:]
	})
	got, err := parsePlayerResponse(body, testVideoID, "en")
	if err != nil {
		t.Fatalf("parsePlayerResponse: %v", err)
	}
	assertReflectedString(t, got, []string{"LanguageCode", "Language"}, "en-gb")
	assertReflectedString(t, got, []string{"Source", "CaptionSource"}, "generated")
}

func TestParseAndroidPlayerClassifiesTerminalOutcomes(t *testing.T) {
	tests := []struct {
		name, language, kind, detail string
		mutate                       func(map[string]any)
	}{
		{"no captions", "en", "no_captions", "caption", func(v map[string]any) { delete(v, "captions") }},
		{"language unavailable lists tracks", "de", "language_unavailable", "en", func(map[string]any) {}},
		{"unavailable preserves reason", "en", "unavailable", "private fixture reason", func(v map[string]any) {
			v["playabilityStatus"] = map[string]any{"status": "ERROR", "reason": "Private fixture reason"}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePlayerResponse(mutatedPlayer(t, tc.mutate), testVideoID, tc.language)
			if err == nil {
				t.Fatal("parsePlayerResponse succeeded")
			}
			kind, detail, cached := terminalErrorParts(t, err)
			if kind != tc.kind {
				t.Errorf("terminal kind = %q, want %q", kind, tc.kind)
			}
			if !strings.Contains(strings.ToLower(detail), tc.detail) {
				t.Errorf("terminal detail = %q, want it to contain %q", detail, tc.detail)
			}
			if tc.kind == "language_unavailable" {
				lower := strings.ToLower(detail)
				for _, want := range []string{"manual", "generated", "en", "fr"} {
					if !strings.Contains(lower, want) {
						t.Errorf("language-unavailable detail %q does not list %q tracks", detail, want)
					}
				}
			}
			if cached {
				t.Error("fresh parser error reports that it came from cache")
			}
		})
	}
}

func TestParseAndroidPlayerKeepsNonTerminalFailuresDistinct(t *testing.T) {
	tests := []struct {
		name string
		want []string
		body func(*testing.T) []byte
	}{
		{"age restriction", []string{"age", "sign in"}, func(t *testing.T) []byte {
			return mutatedPlayer(t, func(v map[string]any) {
				v["playabilityStatus"] = map[string]any{"status": "LOGIN_REQUIRED", "reason": "Sign in to confirm your age"}
			})
		}},
		{"bot block", []string{"bot"}, func(t *testing.T) []byte {
			return mutatedPlayer(t, func(v map[string]any) {
				v["playabilityStatus"] = map[string]any{"status": "LOGIN_REQUIRED", "reason": "Sign in to confirm you're not a bot"}
			})
		}},
		{"active live", []string{"live"}, func(t *testing.T) []byte {
			return mutatedPlayer(t, func(v map[string]any) {
				details(v)["isLiveContent"] = true
				microformat(v)["liveBroadcastDetails"] = map[string]any{"isLiveNow": true}
			})
		}},
		{"upcoming", []string{"upcoming"}, func(t *testing.T) []byte {
			return mutatedPlayer(t, func(v map[string]any) {
				v["playabilityStatus"] = map[string]any{"status": "LIVE_STREAM_OFFLINE", "reason": "This live event will begin soon"}
				microformat(v)["liveBroadcastDetails"] = map[string]any{"isUpcoming": true}
			})
		}},
		{"mismatched ID", []string{"mismatch", "video id"}, func(t *testing.T) []byte {
			return mutatedPlayer(t, func(v map[string]any) { details(v)["videoId"] = "XXXXXXXXXXX" })
		}},
		{"unknown status", []string{"status", "mystery"}, func(t *testing.T) []byte {
			return mutatedPlayer(t, func(v map[string]any) { v["playabilityStatus"] = map[string]any{"status": "MYSTERY"} })
		}},
		{"malformed JSON", []string{"json", "format"}, func(*testing.T) []byte { return []byte(`{"playabilityStatus":`) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePlayerResponse(tc.body(t), testVideoID, "en")
			if err == nil {
				t.Fatal("parsePlayerResponse succeeded")
			}
			message := strings.ToLower(err.Error())
			if !containsAny(message, tc.want...) {
				t.Errorf("error %q does not classify %s", err, tc.name)
			}
			if strings.Contains(message, "no captions") {
				t.Errorf("error %q falsely classifies %s as no captions", err, tc.name)
			}
		})
	}
}

func TestParseAndroidPlayerTreatsUnknownErrorReasonsAsFormatDrift(t *testing.T) {
	for _, status := range []string{"ERROR", "UNPLAYABLE"} {
		_, err := parsePlayerResponse(mutatedPlayer(t, func(v map[string]any) {
			v["playabilityStatus"] = map[string]any{"status": status, "reason": "Unrecognized fixture condition"}
		}), testVideoID, "en")
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "format") {
			t.Errorf("%s unknown reason error = %v, want format drift", status, err)
		}
		var terminal *TerminalError
		if errors.As(err, &terminal) {
			t.Errorf("%s unknown reason became terminal: %#v", status, terminal)
		}
	}
}

func TestParseAndroidPlayerValidatesRemoteMetadata(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"malformed channel ID", func(v map[string]any) { details(v)["channelId"] = "not-a-uc-id" }},
		{"malformed publish date", func(v map[string]any) { microformat(v)["publishDate"] = "08/01/2026" }},
		{"foreign profile URL", func(v map[string]any) { microformat(v)["ownerProfileUrl"] = "https://example.com/@FixtureChannel" }},
		{"profile URL userinfo", func(v map[string]any) { microformat(v)["ownerProfileUrl"] = "https://user@youtube.com/@FixtureChannel" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePlayerResponse(mutatedPlayer(t, tc.mutate), testVideoID, "en")
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "format") {
				t.Errorf("invalid remote metadata error = %v", err)
			}
		})
	}

	if _, err := parsePlayerResponse(mutatedPlayer(t, func(v map[string]any) {
		microformat(v)["publishDate"] = ""
		microformat(v)["ownerProfileUrl"] = ""
	}), testVideoID, "en"); err != nil {
		t.Errorf("optional empty publish/profile metadata rejected: %v", err)
	}
}

func TestJSON3CaptionURLReplacesFormatAndPreservesSignature(t *testing.T) {
	raw := "https://www.youtube.com/api/timedtext?v=" + testVideoID + "&fmt=vtt&sig=a%2Bb%2Fc&expire=123"
	got, err := json3CaptionURL(raw)
	if err != nil {
		t.Fatalf("json3CaptionURL: %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if values := parsed.Query()["fmt"]; len(values) != 1 || values[0] != "json3" {
		t.Errorf("fmt values = %v, want [json3]", values)
	}
	if parsed.Query().Get("sig") != "a+b/c" || parsed.Query().Get("expire") != "123" {
		t.Errorf("signed query parameters changed: %q", parsed.RawQuery)
	}
}

func TestJSON3CaptionURLRejectsPOTokenAndUnsafeHosts(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"xpe", "https://www.youtube.com/api/timedtext?exp=xpe", "po"},
		{"xpv", "https://youtube.com/api/timedtext?exp=xpv", "token"},
		{"non-HTTPS", "http://www.youtube.com/api/timedtext", "https"},
		{"foreign host", "https://example.com/api/timedtext", "host"},
		{"suffix attack", "https://youtube.com.evil.test/api/timedtext", "host"},
		{"userinfo", "https://user@youtube.com/api/timedtext", "credential"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := json3CaptionURL(tc.raw)
			if err == nil {
				t.Fatal("json3CaptionURL succeeded")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestParseJSON3PreservesCuesAndEventBoundaries(t *testing.T) {
	got, err := parseJSON3(fixture(t, "captions.json3"))
	if err != nil {
		t.Fatalf("parseJSON3: %v", err)
	}
	want := "Hello world\n[Music]\nKeep cues"
	if got != want {
		t.Errorf("parseJSON3 = %q, want %q", got, want)
	}
}

func TestParseJSON3RejectsEmptyMalformedAndTextlessBodies(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"empty", nil},
		{"non-JSON", []byte("<html>token required</html>")},
		{"no events", []byte(`{"wireMagic":"pb3"}`)},
		{"formatting-only events", []byte(`{"events":[{"segs":[{"utf8":"   "},{}]}]}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := parseJSON3(tc.body); err == nil {
				t.Fatalf("parseJSON3 succeeded with %q", got)
			} else {
				message := strings.ToLower(err.Error())
				if !containsAny(message, "format", "token", "empty", "json", "text") {
					t.Errorf("error %q does not identify caption format/PO-token failure", err)
				}
			}
		})
	}
}

func TestParseInitialChannelPageScopesVideosTabDeduplicatesAndKeepsOrder(t *testing.T) {
	page, err := parseInitialChannelPage(fixture(t, "channel-initial.html"))
	if err != nil {
		t.Fatalf("parseInitialChannelPage: %v", err)
	}
	assertReflectedString(t, page, []string{"ChannelID", "YoutubeID", "YouTubeID"}, testChannelID)
	assertReflectedString(t, page, []string{"ChannelName", "Title", "Name"}, "Fixture Channel")
	assertReflectedString(t, page, []string{"APIKey", "InnertubeAPIKey"}, "browse-key")
	assertReflectedString(t, page, []string{"ClientName"}, "WEB")
	assertReflectedString(t, page, []string{"ClientVersion"}, "2.20260812.00.00")
	assertReflectedString(t, page, []string{"VisitorData"}, "visitor-page")
	assertReflectedString(t, page, []string{"Continuation", "ContinuationToken", "NextToken"}, "token-one")
	assertReflectedString(t, page, []string{"ClickTrackingParams", "ClickTracking"}, "click-one")
	assertVideoOrder(t, page, []string{"AAAAAAAAAAA", "BBBBBBBBBBB"})
	assertVideoTitles(t, page, []string{"Current entry", "Legacy entry"})
}

func TestParseInitialChannelPageRequiresStableIdentityAndSelectedVideosShape(t *testing.T) {
	var root map[string]any
	if err := decodeEmbeddedJSON(fixture(t, "channel-initial.html"), "var ytInitialData", &root); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	for _, tc := range []struct{ name string }{
		{"missing channel ID"},
		{"empty channel title"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var copyRoot map[string]any
			original, _ := json.Marshal(root)
			_ = json.Unmarshal(original, &copyRoot)
			copyMetadata := copyRoot["metadata"].(map[string]any)["channelMetadataRenderer"].(map[string]any)
			if tc.name == "missing channel ID" {
				delete(copyMetadata, "externalId")
			} else {
				copyMetadata["title"] = ""
			}
			encoded, _ := json.Marshal(copyRoot)
			body := append([]byte(`ytcfg.set({"INNERTUBE_API_KEY":"k","INNERTUBE_CONTEXT":{"client":{"clientName":"WEB","clientVersion":"1"}}}); var ytInitialData = `), encoded...)
			body = append(body, ';')
			if _, err := parseInitialChannelPage(body); err == nil {
				t.Fatal("parseInitialChannelPage succeeded")
			}
		})
	}
	for _, body := range []string{
		`ytcfg.set({"INNERTUBE_CONTEXT":{"client":{"clientName":"WEB","clientVersion":"1"}}}); var ytInitialData = {};`,
		`ytcfg.set({"INNERTUBE_API_KEY":"k","INNERTUBE_CONTEXT":{"client":{"clientVersion":"1"}}}); var ytInitialData = {};`,
		`ytcfg.set({"INNERTUBE_API_KEY":"k","INNERTUBE_CONTEXT":{"client":{"clientName":"WEB"}}}); var ytInitialData = {};`,
	} {
		if _, err := parseInitialChannelPage([]byte(body)); err == nil {
			t.Errorf("page missing required ytcfg value succeeded: %s", body)
		}
	}
}

func TestParseInitialChannelPageRejectsForeignProfileURL(t *testing.T) {
	var root map[string]any
	if err := decodeEmbeddedJSON(fixture(t, "channel-initial.html"), "var ytInitialData", &root); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	root["metadata"].(map[string]any)["channelMetadataRenderer"].(map[string]any)["vanityChannelUrl"] = "https://example.com/@FixtureChannel"
	encoded, err := json.Marshal(root)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	body := append([]byte(`ytcfg.set({"INNERTUBE_API_KEY":"k","INNERTUBE_CONTEXT":{"client":{"clientName":"WEB","clientVersion":"1"}}}); var ytInitialData = `), encoded...)
	if _, err := parseInitialChannelPage(body); err == nil || !strings.Contains(strings.ToLower(err.Error()), "format") {
		t.Errorf("foreign channel profile URL error = %v", err)
	}
}

func TestParseContinuationSupportsAllDeclaredEnvelopes(t *testing.T) {
	for _, tc := range []struct{ fixture, wantID, wantVisitor, wantToken string }{
		{"continuation-actions.json", "CCCCCCCCCCC", "visitor-actions", "token-two"},
		{"continuation-endpoints.json", "DDDDDDDDDDD", "visitor-endpoints", ""},
		{"continuation-legacy.json", "EEEEEEEEEEE", "visitor-legacy", ""},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			page, err := parseContinuationResponse(fixture(t, tc.fixture), map[string]struct{}{})
			if err != nil {
				t.Fatalf("parseContinuationResponse: %v", err)
			}
			assertVideoOrder(t, page, []string{tc.wantID})
			assertReflectedString(t, page, []string{"VisitorData"}, tc.wantVisitor)
			assertReflectedString(t, page, []string{"Continuation", "ContinuationToken", "NextToken"}, tc.wantToken)
			if tc.wantToken != "" {
				assertReflectedString(t, page, []string{"ClickTrackingParams", "ClickTracking"}, "click-two")
			}
		})
	}
}

func TestParseContinuationRejectsRepeatedTokensAndUnknownShapes(t *testing.T) {
	seen := map[string]struct{}{"token-two": {}}
	if _, err := parseContinuationResponse(fixture(t, "continuation-actions.json"), seen); err == nil || !strings.Contains(strings.ToLower(err.Error()), "repeat") {
		t.Errorf("repeated continuation error = %v", err)
	}
	if _, err := parseContinuationResponse([]byte(`{"responseContext":{}}`), map[string]struct{}{}); err == nil {
		t.Fatal("unrecognized continuation shape was treated as end-of-channel")
	}
}

func mutatedPlayer(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(fixture(t, "player-tracks.json"), &value); err != nil {
		t.Fatalf("decode player fixture: %v", err)
	}
	mutate(value)
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode player fixture: %v", err)
	}
	return body
}

func details(value map[string]any) map[string]any { return value["videoDetails"].(map[string]any) }
func microformat(value map[string]any) map[string]any {
	return value["microformat"].(map[string]any)["playerMicroformatRenderer"].(map[string]any)
}

func terminalErrorParts(t *testing.T, err error) (string, string, bool) {
	t.Helper()
	var candidate any = err
	for candidate != nil {
		kind, kindOK := reflectedString(candidate, []string{"Kind"})
		detail, detailOK := reflectedString(candidate, []string{"Detail"})
		cached, cacheOK := reflectedBool(candidate, []string{"Cached", "FromCache"})
		if kindOK && detailOK && cacheOK {
			return kind, detail, cached
		}
		candidate = errors.Unwrap(candidate.(error))
	}
	t.Fatalf("%T does not expose typed terminal kind, detail, and cache source", err)
	return "", "", false
}

func assertReflectedString(t *testing.T, value any, names []string, want string) {
	t.Helper()
	got, ok := reflectedString(value, names)
	if !ok {
		t.Fatalf("%T has no string field/method among %v", value, names)
	}
	if strings.ToLower(got) != strings.ToLower(want) {
		t.Errorf("%v = %q, want %q", names, got, want)
	}
}

func assertReflectedInt(t *testing.T, value any, names []string, want int64) {
	t.Helper()
	v, ok := reflectedValue(value, names)
	if !ok {
		t.Fatalf("%T has no integer field/method among %v", value, names)
	}
	if v.Kind() >= reflect.Int && v.Kind() <= reflect.Int64 && v.Int() == want {
		return
	}
	t.Errorf("%v = %v, want %d", names, v.Interface(), want)
}

func reflectedString(value any, names []string) (string, bool) {
	v, ok := reflectedValue(value, names)
	if ok && v.Kind() == reflect.String {
		return v.String(), true
	}
	return "", false
}

func reflectedBool(value any, names []string) (bool, bool) {
	v, ok := reflectedValue(value, names)
	if ok && v.Kind() == reflect.Bool {
		return v.Bool(), true
	}
	return false, false
}

func reflectedValue(value any, names []string) (reflect.Value, bool) {
	v := reflect.ValueOf(value)
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.IsValid() && v.Kind() == reflect.Struct {
		for _, name := range names {
			if field := v.FieldByName(name); field.IsValid() && field.CanInterface() {
				return field, true
			}
		}
	}
	m := reflect.ValueOf(value)
	for _, name := range names {
		method := m.MethodByName(name)
		if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
			return method.Call(nil)[0], true
		}
	}
	return reflect.Value{}, false
}

func assertVideoOrder(t *testing.T, page any, want []string) {
	t.Helper()
	v, ok := reflectedValue(page, []string{"Videos", "Entries", "Items"})
	if !ok || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) {
		t.Fatalf("%T does not carry a video collection", page)
	}
	var got []string
	for i := 0; i < v.Len(); i++ {
		id, ok := reflectedString(v.Index(i).Interface(), []string{"VideoID", "YoutubeID", "YouTubeID", "ID"})
		if !ok {
			t.Fatalf("video %d has no ID", i)
		}
		got = append(got, id)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("video order = %v, want %v", got, want)
	}
}

func assertVideoTitles(t *testing.T, page any, want []string) {
	t.Helper()
	v, ok := reflectedValue(page, []string{"Videos", "Entries", "Items"})
	if !ok || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) {
		t.Fatalf("%T does not carry a video collection", page)
	}
	var got []string
	for i := 0; i < v.Len(); i++ {
		title, ok := reflectedString(v.Index(i).Interface(), []string{"Title"})
		if !ok {
			t.Fatalf("video %d has no title", i)
		}
		got = append(got, title)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("video titles = %v, want %v", got, want)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
