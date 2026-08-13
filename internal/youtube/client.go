package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	youtubeOrigin        = "https://www.youtube.com"
	maxResponseBytes     = 10 << 20
	androidClientName    = "ANDROID"
	androidClientID      = "3"
	androidClientVersion = "20.10.38"
	androidUserAgent     = "com.google.android.youtube/20.10.38 (Linux; U; Android 14) gzip"
	webUserAgent         = "Mozilla/5.0 (compatible; Evie/1.0; +https://github.com/davidadel66/evie)"
)

var (
	channelIDPattern = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	handlePattern    = regexp.MustCompile(`^@[A-Za-z0-9._-]{3,30}$`)
)

// TerminalError describes a cacheable YouTube outcome. Cached is false for
// errors produced by Client; the service sets it when returning stored state.
type TerminalError struct {
	Kind   string
	Detail string
	Cached bool
}

func (e *TerminalError) Error() string {
	if e.Detail == "" {
		return e.Kind
	}
	return e.Kind + ": " + e.Detail
}

// Client owns the bounded, retried HTTP protocol used to talk to YouTube.
type Client struct {
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

// NewClient retains the caller's transport and timeout. A nil client gets the
// package's 30-second network timeout.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	clientCopy := *httpClient
	originalRedirect := clientCopy.CheckRedirect
	clientCopy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if isConsentHost(req.URL.Hostname()) {
			return consentRedirectError{url: req.URL.String()}
		}
		if captionRequest, _ := req.Context().Value(captionRequestContextKey{}).(bool); captionRequest {
			if err := validateCaptionRequestURL(req.URL.String()); err != nil {
				return fmt.Errorf("unsafe YouTube caption redirect: %w", err)
			}
		}
		if originalRedirect != nil {
			return originalRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &Client{
		httpClient: &clientCopy,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

type consentRedirectError struct{ url string }

type captionRequestContextKey struct{}

func (e consentRedirectError) Error() string { return "YouTube consent redirect to " + e.url }

type watchPage struct {
	APIKey string
}

type videoFetch struct {
	VideoID         string
	Title           string
	ChannelID       string
	ChannelName     string
	ChannelHandle   string
	ChannelURL      string
	PublishedAt     string
	DurationSeconds int64
	LanguageCode    string
	LanguageName    string
	Source          string
	CaptionURL      string
	Text            string
}

type videoFetchError struct {
	metadata videoFetch
	err      error
}

func (e *videoFetchError) Error() string { return e.err.Error() }
func (e *videoFetchError) Unwrap() error { return e.err }

type channelVideo struct {
	VideoID string
	Title   string
}

type channelPage struct {
	ChannelID           string
	ChannelName         string
	ChannelHandle       string
	ChannelURL          string
	APIKey              string
	ClientName          string
	ClientVersion       string
	VisitorData         string
	Context             map[string]any
	Videos              []channelVideo
	Continuation        string
	ClickTrackingParams string
}

type captionText struct {
	SimpleText string `json:"simpleText"`
	Runs       []struct {
		Text string `json:"text"`
	} `json:"runs"`
}

type captionTrack struct {
	BaseURL      string      `json:"baseUrl"`
	Name         captionText `json:"name"`
	LanguageCode string      `json:"languageCode"`
	Kind         string      `json:"kind"`
}

func parseVideoInput(input string) (string, string, error) {
	if videoIDPattern.MatchString(input) {
		return input, canonicalVideoURL(input), nil
	}
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", invalidVideoInput()
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", "", invalidVideoInput()
	}
	if parsed.User != nil || parsed.Fragment != "" || parsed.Port() != "" {
		return "", "", invalidVideoInput()
	}

	host := strings.ToLower(parsed.Hostname())
	var id string
	switch host {
	case "youtube.com", "www.youtube.com":
		switch {
		case parsed.Path == "/watch":
			values := parsed.Query()["v"]
			if len(values) != 1 {
				return "", "", invalidVideoInput()
			}
			id = values[0]
		case strings.HasPrefix(parsed.Path, "/shorts/"):
			if parsed.RawQuery != "" {
				return "", "", invalidVideoInput()
			}
			id = parsedSinglePathValue(parsed.Path, "/shorts/")
		case strings.HasPrefix(parsed.Path, "/live/"):
			if parsed.RawQuery != "" {
				return "", "", invalidVideoInput()
			}
			id = parsedSinglePathValue(parsed.Path, "/live/")
		case strings.HasPrefix(parsed.Path, "/embed/"):
			if parsed.RawQuery != "" {
				return "", "", invalidVideoInput()
			}
			id = parsedSinglePathValue(parsed.Path, "/embed/")
		}
	case "youtu.be":
		if parsed.RawQuery != "" {
			return "", "", invalidVideoInput()
		}
		id = parsedSinglePathValue(parsed.Path, "/")
	case "youtube-nocookie.com", "www.youtube-nocookie.com":
		if strings.HasPrefix(parsed.Path, "/embed/") && parsed.RawQuery == "" {
			id = parsedSinglePathValue(parsed.Path, "/embed/")
		}
	}
	if !videoIDPattern.MatchString(id) {
		return "", "", invalidVideoInput()
	}
	return id, canonicalVideoURL(id), nil
}

func parseChannelInput(input string) (string, string, error) {
	if channelIDPattern.MatchString(input) {
		return input, "", nil
	}
	if handlePattern.MatchString(input) {
		return "", input, nil
	}
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", invalidChannelInput()
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.Port() != "" {
		return "", "", invalidChannelInput()
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "youtube.com" && host != "www.youtube.com" {
		return "", "", invalidChannelInput()
	}
	parts := strings.Split(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/")
	if len(parts) == 1 {
		handle, err := url.PathUnescape(parts[0])
		if err == nil && handlePattern.MatchString(handle) {
			return "", handle, nil
		}
	}
	if len(parts) == 2 {
		first, firstErr := url.PathUnescape(parts[0])
		second, secondErr := url.PathUnescape(parts[1])
		if firstErr == nil && secondErr == nil {
			if handlePattern.MatchString(first) && second == "videos" {
				return "", first, nil
			}
			if first == "channel" && channelIDPattern.MatchString(second) {
				return second, "", nil
			}
		}
	}
	return "", "", invalidChannelInput()
}

func invalidVideoInput() error {
	return errors.New("invalid video input: accepted forms are an 11-character video ID or a supported YouTube watch, youtu.be, shorts, live, or embed URL")
}

func invalidChannelInput() error {
	return errors.New("invalid channel input: accepted forms are @handle, a UC channel ID, or a supported YouTube handle/channel URL")
}

func parsedSinglePathValue(path, prefix string) string {
	value := strings.TrimPrefix(path, prefix)
	if value == path || value == "" || strings.Contains(value, "/") {
		return ""
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return ""
	}
	return decoded
}

func canonicalVideoURL(id string) string {
	return youtubeOrigin + "/watch?v=" + id
}

func decodeEmbeddedJSON(source []byte, marker string, dst any) error {
	start := bytes.Index(source, []byte(marker))
	if start < 0 {
		return fmt.Errorf("YouTube format drift: missing %q marker", marker)
	}
	remainder := source[start+len(marker):]
	object := bytes.IndexByte(remainder, '{')
	if object < 0 {
		return fmt.Errorf("YouTube format drift: %q has no JSON object", marker)
	}
	for _, b := range remainder[:object] {
		if !isJSONMarkerSeparator(b) {
			return fmt.Errorf("YouTube format drift: %q is not followed by a JSON object", marker)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(remainder[object:]))
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("YouTube format drift: decode JSON after %q: %w", marker, err)
	}
	return nil
}

func isJSONMarkerSeparator(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '=', '(':
		return true
	default:
		return false
	}
}

func parseWatchPage(body []byte) (watchPage, error) {
	config, err := parseYTConfig(body)
	if err != nil {
		return watchPage{}, err
	}
	apiKey, _ := config["INNERTUBE_API_KEY"].(string)
	if apiKey == "" {
		return watchPage{}, errors.New("YouTube format drift: watch page is missing INNERTUBE_API_KEY")
	}
	return watchPage{APIKey: apiKey}, nil
}

func parseYTConfig(body []byte) (map[string]any, error) {
	const marker = "ytcfg.set"
	merged := make(map[string]any)
	foundObject := false
	for _, index := range javascriptMarkerOffsets(body, marker) {
		remainder := body[index+len(marker):]
		argument := bytes.TrimLeft(remainder, " \t\r\n(")
		if len(argument) == 0 || argument[0] != '{' {
			continue
		}
		var next map[string]any
		if err := json.NewDecoder(bytes.NewReader(argument)).Decode(&next); err != nil {
			// YouTube also emits JavaScript object literals with single quotes
			// and trailing commas. They are not configuration JSON we can safely
			// interpret; required values are validated by each caller below.
			continue
		}
		foundObject = true
		mergeJSONObjects(merged, next)
	}
	if !foundObject {
		return nil, errors.New("YouTube format drift: missing decodable ytcfg.set object configuration")
	}
	return merged, nil
}

func javascriptMarkerOffsets(source []byte, marker string) []int {
	var offsets []int
	for i := 0; i < len(source); {
		switch source[i] {
		case '\'', '"', '`':
			i = skipJavaScriptQuoted(source, i, source[i])
		case '/':
			if i+1 < len(source) && source[i+1] == '/' {
				i += 2
				for i < len(source) && source[i] != '\n' && source[i] != '\r' {
					i++
				}
			} else if i+1 < len(source) && source[i+1] == '*' {
				i += 2
				for i+1 < len(source) && (source[i] != '*' || source[i+1] != '/') {
					i++
				}
				if i+1 < len(source) {
					i += 2
				}
			} else {
				i++
			}
		default:
			if bytes.HasPrefix(source[i:], []byte(marker)) &&
				(i == 0 || !isJavaScriptIdentifierByte(source[i-1])) &&
				(i+len(marker) == len(source) || !isJavaScriptIdentifierByte(source[i+len(marker)])) {
				offsets = append(offsets, i)
				i += len(marker)
				continue
			}
			i++
		}
	}
	return offsets
}

func skipJavaScriptQuoted(source []byte, start int, quote byte) int {
	for i := start + 1; i < len(source); i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		if source[i] == quote {
			return i + 1
		}
	}
	return len(source)
}

func isJavaScriptIdentifierByte(value byte) bool {
	return value == '_' || value == '$' || value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func mergeJSONObjects(dst, src map[string]any) {
	for key, value := range src {
		if srcObject, ok := value.(map[string]any); ok {
			if dstObject, ok := dst[key].(map[string]any); ok {
				mergeJSONObjects(dstObject, srcObject)
				continue
			}
		}
		dst[key] = value
	}
}

func parsePlayerResponse(body []byte, requestedVideoID, requestedLanguage string) (videoFetch, error) {
	var response struct {
		PlayabilityStatus struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"playabilityStatus"`
		VideoDetails struct {
			VideoID       string `json:"videoId"`
			Title         string `json:"title"`
			ChannelID     string `json:"channelId"`
			Author        string `json:"author"`
			LengthSeconds string `json:"lengthSeconds"`
			IsLiveContent bool   `json:"isLiveContent"`
		} `json:"videoDetails"`
		Microformat struct {
			Renderer struct {
				PublishDate     string `json:"publishDate"`
				OwnerProfileURL string `json:"ownerProfileUrl"`
				LiveDetails     struct {
					IsLiveNow  bool `json:"isLiveNow"`
					IsUpcoming bool `json:"isUpcoming"`
				} `json:"liveBroadcastDetails"`
			} `json:"playerMicroformatRenderer"`
		} `json:"microformat"`
		Captions struct {
			Tracklist struct {
				Tracks []captionTrack `json:"captionTracks"`
			} `json:"playerCaptionsTracklistRenderer"`
		} `json:"captions"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return videoFetch{}, fmt.Errorf("YouTube player JSON format error: %w", err)
	}
	status := strings.TrimSpace(response.PlayabilityStatus.Status)
	reason := strings.TrimSpace(response.PlayabilityStatus.Reason)
	if status != "OK" {
		return videoFetch{}, classifyPlayabilityError(status, reason, response.Microformat.Renderer.LiveDetails.IsUpcoming)
	}
	if response.VideoDetails.VideoID != requestedVideoID {
		return videoFetch{}, fmt.Errorf("YouTube format drift: player video ID mismatch: got %q, want %q", response.VideoDetails.VideoID, requestedVideoID)
	}
	if response.VideoDetails.IsLiveContent && response.Microformat.Renderer.LiveDetails.IsLiveNow {
		return videoFetch{}, errors.New("active live stream has no stable transcript yet")
	}
	if response.Microformat.Renderer.LiveDetails.IsUpcoming {
		return videoFetch{}, errors.New("upcoming stream has no transcript yet")
	}
	if response.VideoDetails.Title == "" || response.VideoDetails.ChannelID == "" || response.VideoDetails.Author == "" {
		return videoFetch{}, errors.New("YouTube format drift: player response is missing required video/channel metadata")
	}
	if !channelIDPattern.MatchString(response.VideoDetails.ChannelID) {
		return videoFetch{}, fmt.Errorf("YouTube format drift: invalid channel ID %q", response.VideoDetails.ChannelID)
	}
	duration, err := strconv.ParseInt(response.VideoDetails.LengthSeconds, 10, 64)
	if err != nil || duration < 0 {
		return videoFetch{}, fmt.Errorf("YouTube format drift: invalid video duration %q", response.VideoDetails.LengthSeconds)
	}
	profileURL := response.Microformat.Renderer.OwnerProfileURL
	publishDate := response.Microformat.Renderer.PublishDate
	if publishDate != "" {
		parsedDate, err := time.Parse("2006-01-02", publishDate)
		if err != nil || parsedDate.Format("2006-01-02") != publishDate {
			return videoFetch{}, fmt.Errorf("YouTube format drift: invalid publish date %q", publishDate)
		}
	}
	if profileURL != "" {
		if err := validateYouTubeProfileURL(profileURL); err != nil {
			return videoFetch{}, fmt.Errorf("YouTube format drift: invalid channel profile URL %q", profileURL)
		}
	}
	metadata := videoFetch{
		VideoID:         response.VideoDetails.VideoID,
		Title:           response.VideoDetails.Title,
		ChannelID:       response.VideoDetails.ChannelID,
		ChannelName:     response.VideoDetails.Author,
		ChannelHandle:   handleFromProfileURL(profileURL),
		ChannelURL:      profileURL,
		PublishedAt:     publishDate,
		DurationSeconds: duration,
	}
	if len(response.Captions.Tracklist.Tracks) == 0 {
		return videoFetch{}, &videoFetchError{
			metadata: metadata,
			err:      &TerminalError{Kind: "no_captions", Detail: "captions are disabled or absent"},
		}
	}
	track, source, ok := selectCaptionTrack(response.Captions.Tracklist.Tracks, requestedLanguage)
	if !ok {
		return videoFetch{}, &videoFetchError{
			metadata: metadata,
			err: &TerminalError{
				Kind:   "language_unavailable",
				Detail: unavailableLanguageDetail(requestedLanguage, response.Captions.Tracklist.Tracks),
			},
		}
	}
	metadata.LanguageCode = normalizeLanguage(track.LanguageCode)
	metadata.LanguageName = textFromValue(track.Name)
	metadata.Source = source
	metadata.CaptionURL = track.BaseURL
	return metadata, nil
}

func classifyPlayabilityError(status, reason string, upcoming bool) error {
	lower := strings.ToLower(reason)
	if strings.Contains(lower, "not a bot") {
		return fmt.Errorf("YouTube bot block: %s", detailOrStatus(reason, status))
	}
	if strings.Contains(lower, "private") || strings.Contains(lower, "deleted") || strings.Contains(lower, "removed") || strings.Contains(lower, "unavailable") {
		return &TerminalError{Kind: "unavailable", Detail: detailOrStatus(reason, status)}
	}
	if status == "LOGIN_REQUIRED" || strings.Contains(lower, "age") || strings.Contains(lower, "sign in") {
		return fmt.Errorf("YouTube age/sign in restriction: %s", detailOrStatus(reason, status))
	}
	if upcoming || status == "LIVE_STREAM_OFFLINE" {
		return fmt.Errorf("upcoming live stream: %s", detailOrStatus(reason, status))
	}
	return fmt.Errorf("YouTube format drift: unknown playability status %q: %s", status, reason)
}

func detailOrStatus(detail, status string) string {
	if detail != "" {
		return detail
	}
	return status
}

func selectCaptionTrack(tracks []captionTrack, requested string) (captionTrack, string, bool) {
	requested = normalizeLanguage(requested)
	codes := []string{requested}
	if !strings.Contains(requested, "-") {
		var variants []string
		for _, track := range tracks {
			code := normalizeLanguage(track.LanguageCode)
			if strings.HasPrefix(code, requested+"-") {
				variants = append(variants, code)
			}
		}
		codes = append(codes, sortedUnique(variants)...)
	}
	for _, code := range codes {
		for _, source := range []string{"manual", "generated"} {
			for _, track := range tracks {
				if normalizeLanguage(track.LanguageCode) == code && captionSource(track.Kind) == source {
					return track, source, true
				}
			}
		}
	}
	return captionTrack{}, "", false
}

func unavailableLanguageDetail(requested string, tracks []captionTrack) string {
	var available []string
	for _, track := range tracks {
		available = append(available, normalizeLanguage(track.LanguageCode)+" ("+captionSource(track.Kind)+")")
	}
	return fmt.Sprintf("requested language %q is unavailable; available tracks: %s", normalizeLanguage(requested), strings.Join(sortedUnique(available), ", "))
}

func textFromValue(value captionText) string {
	if value.SimpleText != "" {
		return value.SimpleText
	}
	var text strings.Builder
	for _, run := range value.Runs {
		text.WriteString(run.Text)
	}
	return text.String()
}

func handleFromProfileURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	part := strings.Trim(parsed.Path, "/")
	if handlePattern.MatchString(part) {
		return part
	}
	return ""
}

func validateYouTubeProfileURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "https") ||
		parsed.User != nil || parsed.Port() != "" || !isYouTubeHost(parsed.Hostname()) {
		return errors.New("profile URL must be credential-free HTTPS on a YouTube host")
	}
	return nil
}

func json3CaptionURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", errors.New("invalid caption URL format")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", errors.New("caption URL must use HTTPS")
	}
	if parsed.User != nil {
		return "", errors.New("caption URL must not contain credentials")
	}
	if !isYouTubeHost(parsed.Hostname()) {
		return "", fmt.Errorf("caption URL host %q is not a YouTube host", parsed.Hostname())
	}
	query := parsed.Query()
	for _, exp := range query["exp"] {
		if exp = strings.ToLower(exp); exp == "xpe" || exp == "xpv" {
			return "", errors.New("caption subtitle PO token is required")
		}
	}
	query.Set("fmt", "json3")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func parseJSON3(body []byte) (string, error) {
	if len(body) == 0 {
		return "", errors.New("empty caption response; subtitle PO token or format change may be required")
	}
	var response struct {
		Events []struct {
			Segments []struct {
				Text string `json:"utf8"`
			} `json:"segs"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("caption JSON format/PO-token failure: %w", err)
	}
	var events []string
	for _, event := range response.Events {
		var text strings.Builder
		for _, segment := range event.Segments {
			text.WriteString(segment.Text)
		}
		if value := strings.TrimSpace(text.String()); value != "" {
			events = append(events, value)
		}
	}
	if len(events) == 0 {
		return "", errors.New("caption JSON contains no text events; subtitle format or PO token may have changed")
	}
	return strings.Join(events, "\n"), nil
}

func parseInitialChannelPage(body []byte) (channelPage, error) {
	config, err := parseYTConfig(body)
	if err != nil {
		return channelPage{}, err
	}
	apiKey, _ := config["INNERTUBE_API_KEY"].(string)
	contextValue, _ := config["INNERTUBE_CONTEXT"].(map[string]any)
	clientValue, _ := contextValue["client"].(map[string]any)
	clientName, _ := clientValue["clientName"].(string)
	clientVersion, _ := clientValue["clientVersion"].(string)
	visitorData, _ := clientValue["visitorData"].(string)
	if visitorData == "" {
		visitorData, _ = config["VISITOR_DATA"].(string)
	}
	if apiKey == "" || clientName == "" || clientVersion == "" {
		return channelPage{}, errors.New("YouTube format drift: channel page is missing required Innertube API key/client context")
	}
	var initial map[string]any
	if err := decodeEmbeddedJSON(body, "var ytInitialData", &initial); err != nil {
		return channelPage{}, err
	}
	metadata := objectAt(initial, "metadata", "channelMetadataRenderer")
	channelID := stringAt(metadata, "externalId")
	channelName := strings.TrimSpace(stringAt(metadata, "title"))
	if !channelIDPattern.MatchString(channelID) || channelName == "" {
		return channelPage{}, errors.New("YouTube format drift: channel metadata is missing a stable UC ID or non-empty title")
	}
	items, ok := selectedVideosItems(initial)
	if !ok {
		return channelPage{}, errors.New("YouTube format drift: selected Videos tab content was not recognized")
	}
	videos, continuation, click := parseChannelItems(items)
	channelURL := youtubeOrigin + "/channel/" + channelID
	vanityURL := stringAt(metadata, "vanityChannelUrl")
	if vanityURL != "" {
		if err := validateYouTubeProfileURL(vanityURL); err != nil {
			return channelPage{}, fmt.Errorf("YouTube format drift: invalid channel profile URL %q", vanityURL)
		}
	}
	return channelPage{
		ChannelID:           channelID,
		ChannelName:         channelName,
		ChannelHandle:       handleFromProfileURL(vanityURL),
		ChannelURL:          channelURL,
		APIKey:              apiKey,
		ClientName:          clientName,
		ClientVersion:       clientVersion,
		VisitorData:         visitorData,
		Context:             contextValue,
		Videos:              videos,
		Continuation:        continuation,
		ClickTrackingParams: click,
	}, nil
}

func selectedVideosItems(initial map[string]any) ([]any, bool) {
	tabs, ok := arrayAt(initial, "contents", "twoColumnBrowseResultsRenderer", "tabs")
	if !ok {
		return nil, false
	}
	for _, rawTab := range tabs {
		tab := objectAt(asObject(rawTab), "tabRenderer")
		selected, _ := tab["selected"].(bool)
		if !selected || !strings.EqualFold(stringAt(tab, "title"), "Videos") {
			continue
		}
		return arrayAt(tab, "content", "richGridRenderer", "contents")
	}
	return nil, false
}

func parseContinuationResponse(body []byte, seen map[string]struct{}) (channelPage, error) {
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return channelPage{}, fmt.Errorf("YouTube continuation JSON format error: %w", err)
	}
	var items []any
	recognized := false
	for _, envelope := range []string{"onResponseReceivedActions", "onResponseReceivedEndpoints"} {
		if entries, ok := response[envelope].([]any); ok {
			for _, entry := range entries {
				if values, ok := arrayAt(asObject(entry), "appendContinuationItemsAction", "continuationItems"); ok {
					recognized = true
					items = append(items, values...)
				}
			}
		}
	}
	if contents, ok := arrayAt(response, "continuationContents", "richGridContinuation", "contents"); ok {
		recognized = true
		items = append(items, contents...)
	}
	if !recognized {
		return channelPage{}, errors.New("YouTube format drift: unrecognized continuation response shape")
	}
	videos, continuation, click := parseChannelItems(items)
	if continuation != "" {
		if _, exists := seen[continuation]; exists {
			return channelPage{}, fmt.Errorf("YouTube continuation token repeated: %q", continuation)
		}
	}
	responseContext := objectAt(response, "responseContext")
	visitor := stringAt(responseContext, "visitorData")
	if visitor == "" {
		visitor = stringAt(objectAt(responseContext, "webResponseContextExtensionData", "ytConfigData"), "visitorData")
	}
	return channelPage{Videos: videos, Continuation: continuation, ClickTrackingParams: click, VisitorData: visitor}, nil
}

func parseChannelItems(items []any) ([]channelVideo, string, string) {
	seen := make(map[string]struct{})
	var videos []channelVideo
	var continuation, click string
	for _, rawItem := range items {
		item := asObject(rawItem)
		content := objectAt(item, "richItemRenderer", "content")
		if len(content) == 0 {
			content = item
		}
		var video channelVideo
		if renderer := objectAt(content, "lockupViewModel"); len(renderer) > 0 {
			video.VideoID = stringAt(renderer, "contentId")
			video.Title = stringAt(objectAt(renderer, "metadata", "lockupMetadataViewModel", "title"), "content")
		} else if renderer := objectAt(content, "videoRenderer"); len(renderer) > 0 {
			video.VideoID = stringAt(renderer, "videoId")
			video.Title = rendererText(renderer["title"])
		}
		if videoIDPattern.MatchString(video.VideoID) {
			if _, exists := seen[video.VideoID]; !exists {
				seen[video.VideoID] = struct{}{}
				videos = append(videos, video)
			}
		}
		endpoint := objectAt(item, "continuationItemRenderer", "continuationEndpoint")
		if len(endpoint) > 0 {
			continuation = stringAt(objectAt(endpoint, "continuationCommand"), "token")
			click = stringAt(endpoint, "clickTrackingParams")
		}
	}
	return videos, continuation, click
}

func (c *Client) fetchVideo(ctx context.Context, videoID, language string) (videoFetch, error) {
	language = normalizeLanguage(language)
	watchURL := canonicalVideoURL(videoID) + "&hl=en"
	body, err := c.get(ctx, watchURL, false)
	if err != nil {
		return videoFetch{}, fmt.Errorf("fetch YouTube watch page: %w", err)
	}
	watch, err := parseWatchPage(body)
	if err != nil {
		return videoFetch{}, err
	}
	requestBody := map[string]any{
		"videoId": videoID,
		"context": map[string]any{"client": map[string]any{
			"clientName":        androidClientName,
			"clientVersion":     androidClientVersion,
			"androidSdkVersion": 34,
		}},
	}
	playerURL := youtubeOrigin + "/youtubei/v1/player?key=" + url.QueryEscape(watch.APIKey) + "&prettyPrint=false"
	headers := http.Header{
		"Content-Type":             {"application/json"},
		"User-Agent":               {androidUserAgent},
		"X-Youtube-Client-Name":    {androidClientID},
		"X-Youtube-Client-Version": {androidClientVersion},
	}
	body, err = c.postJSON(ctx, playerURL, requestBody, headers)
	if err != nil {
		return videoFetch{}, fmt.Errorf("fetch YouTube Android player: %w", err)
	}
	result, err := parsePlayerResponse(body, videoID, language)
	if err != nil {
		return videoFetch{}, err
	}
	captionURL, err := json3CaptionURL(result.CaptionURL)
	if err != nil {
		return videoFetch{}, err
	}
	body, err = c.get(ctx, captionURL, true)
	if err != nil {
		return videoFetch{}, fmt.Errorf("fetch YouTube captions: %w", err)
	}
	result.Text, err = parseJSON3(body)
	if err != nil {
		return videoFetch{}, err
	}
	return result, nil
}

func (c *Client) listChannel(ctx context.Context, input string, limit int) (channelPage, error) {
	channelID, handle, err := parseChannelInput(input)
	if err != nil {
		return channelPage{}, err
	}
	path := "/channel/" + channelID
	if handle != "" {
		path = "/" + handle
	}
	body, err := c.get(ctx, youtubeOrigin+path+"/videos?hl=en", false)
	if err != nil {
		return channelPage{}, fmt.Errorf("fetch YouTube channel page: %w", err)
	}
	result, err := parseInitialChannelPage(body)
	if err != nil {
		return channelPage{}, err
	}
	if channelID != "" && result.ChannelID != channelID {
		return channelPage{}, fmt.Errorf("YouTube channel ID mismatch: got %q, want %q", result.ChannelID, channelID)
	}
	if limit > 0 && len(result.Videos) >= limit {
		result.Videos = result.Videos[:limit]
		return result, nil
	}
	seenVideos := make(map[string]struct{}, len(result.Videos))
	for _, video := range result.Videos {
		seenVideos[video.VideoID] = struct{}{}
	}
	seenTokens := make(map[string]struct{})
	for result.Continuation != "" {
		token := result.Continuation
		if _, exists := seenTokens[token]; exists {
			return channelPage{}, fmt.Errorf("YouTube continuation token repeated: %q", token)
		}
		seenTokens[token] = struct{}{}
		requestBody := map[string]any{
			"context":      result.Context,
			"continuation": token,
		}
		if result.ClickTrackingParams != "" {
			requestBody["clickTracking"] = map[string]any{"clickTrackingParams": result.ClickTrackingParams}
		}
		headers := http.Header{
			"Content-Type":             {"application/json"},
			"Origin":                   {youtubeOrigin},
			"User-Agent":               {webUserAgent},
			"X-Youtube-Client-Name":    {result.ClientName},
			"X-Youtube-Client-Version": {result.ClientVersion},
		}
		if result.VisitorData != "" {
			headers.Set("X-Goog-Visitor-Id", result.VisitorData)
		}
		browseURL := youtubeOrigin + "/youtubei/v1/browse?key=" + url.QueryEscape(result.APIKey) + "&prettyPrint=false"
		body, err = c.postJSON(ctx, browseURL, requestBody, headers)
		if err != nil {
			return channelPage{}, fmt.Errorf("fetch YouTube channel continuation: %w", err)
		}
		page, err := parseContinuationResponse(body, seenTokens)
		if err != nil {
			return channelPage{}, err
		}
		for _, video := range page.Videos {
			if _, exists := seenVideos[video.VideoID]; exists {
				continue
			}
			seenVideos[video.VideoID] = struct{}{}
			result.Videos = append(result.Videos, video)
			if limit > 0 && len(result.Videos) == limit {
				return result, nil
			}
		}
		result.Continuation = page.Continuation
		result.ClickTrackingParams = page.ClickTrackingParams
		if page.VisitorData != "" {
			result.VisitorData = page.VisitorData
			client := objectAt(result.Context, "client")
			client["visitorData"] = page.VisitorData
		}
	}
	return result, nil
}

func (c *Client) get(ctx context.Context, rawURL string, caption bool) ([]byte, error) {
	if caption {
		if err := validateCaptionRequestURL(rawURL); err != nil {
			return nil, err
		}
		ctx = context.WithValue(ctx, captionRequestContextKey{}, true)
	}
	return c.do(ctx, http.MethodGet, rawURL, nil, http.Header{
		"Accept-Language": {"en-US,en;q=0.9"},
		"User-Agent":      {webUserAgent},
	})
}

func (c *Client) postJSON(ctx context.Context, rawURL string, value any, headers http.Header) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode YouTube request: %w", err)
	}
	return c.do(ctx, http.MethodPost, rawURL, body, headers)
}

func (c *Client) do(ctx context.Context, method, rawURL string, requestBody []byte, headers http.Header) ([]byte, error) {
	for attempt := 0; attempt < 3; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(requestBody))
		if err != nil {
			return nil, fmt.Errorf("build YouTube HTTP request: %w", err)
		}
		request.Header = headers.Clone()
		response, err := c.httpClient.Do(request)
		if err != nil {
			var consent consentRedirectError
			if errors.As(err, &consent) {
				return nil, consent
			}
			return nil, fmt.Errorf("YouTube HTTP timeout/failure: %w", err)
		}
		body, readErr := readBoundedBody(response)
		response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if blockErr := detectBlockingResponse(response, body); blockErr != nil {
			return nil, blockErr
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return body, nil
		}
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		if retryable && attempt < 2 {
			delay := retryDelay(response.Header.Get("Retry-After"), response.StatusCode, attempt)
			if err := c.sleep(ctx, delay); err != nil {
				return nil, fmt.Errorf("YouTube HTTP retry interrupted: %w", err)
			}
			continue
		}
		if response.StatusCode == http.StatusTooManyRequests {
			return nil, errors.New("YouTube HTTP 429 rate limit exhausted after retries")
		}
		return nil, fmt.Errorf("YouTube HTTP request failed with status %d", response.StatusCode)
	}
	return nil, errors.New("YouTube HTTP retries exhausted")
}

func readBoundedBody(response *http.Response) ([]byte, error) {
	if response.ContentLength > maxResponseBytes {
		return nil, errors.New("YouTube response exceeds the 10 MiB limit")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read YouTube HTTP response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("YouTube response exceeds the 10 MiB limit")
	}
	return body, nil
}

func retryDelay(retryAfter string, status, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, 30*time.Second)
	}
	if when, err := http.ParseTime(retryAfter); err == nil {
		return min(max(time.Until(when), 0), 30*time.Second)
	}
	if status == http.StatusTooManyRequests {
		return time.Duration(5*(attempt+1)) * time.Second
	}
	return time.Duration(attempt+1) * time.Second
}

func detectBlockingResponse(response *http.Response, body []byte) error {
	lower := strings.ToLower(string(body))
	location := response.Header.Get("Location")
	if isConsentHost(response.Request.URL.Hostname()) || strings.Contains(strings.ToLower(location), "consent.youtube.com") ||
		(strings.Contains(lower, "<form") && strings.Contains(lower, "consent.youtube.com")) {
		return errors.New("YouTube consent page blocked the request")
	}
	if isRecaptchaChallengeURL(response.Request.URL.String()) || isRecaptchaChallengeURL(location) ||
		strings.Contains(lower, "g-recaptcha") ||
		((strings.Contains(lower, "<form") || strings.Contains(lower, "<iframe")) &&
			(strings.Contains(lower, "recaptcha/api") || strings.Contains(lower, "recaptcha.net/recaptcha"))) {
		return errors.New("YouTube recaptcha blocked the request")
	}
	if strings.Contains(lower, "sign in to confirm") && strings.Contains(lower, "not a bot") {
		return errors.New("YouTube bot confirmation blocked the request")
	}
	return nil
}

func isRecaptchaChallengeURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	path := strings.ToLower(parsed.Path)
	googleHost := host == "google.com" || strings.HasSuffix(host, ".google.com")
	recaptchaHost := host == "recaptcha.net" || strings.HasSuffix(host, ".recaptcha.net")
	return (googleHost || recaptchaHost) && (strings.Contains(path, "/recaptcha/") || strings.HasPrefix(path, "/sorry/"))
}

func validateCaptionRequestURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return errors.New("invalid caption URL")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return errors.New("caption URL must use HTTPS")
	}
	if parsed.User != nil {
		return errors.New("caption URL must not contain credentials")
	}
	if !isYouTubeHost(parsed.Hostname()) {
		return fmt.Errorf("caption URL host %q is not allowed", parsed.Hostname())
	}
	return nil
}

func isYouTubeHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "youtube.com" || strings.HasSuffix(host, ".youtube.com")
}

func isConsentHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "consent.youtube.com" || strings.HasSuffix(host, ".consent.youtube.com")
}

func objectAt(root map[string]any, path ...string) map[string]any {
	current := root
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func arrayAt(root map[string]any, path ...string) ([]any, bool) {
	if len(path) == 0 {
		return nil, false
	}
	parent := objectAt(root, path[:len(path)-1]...)
	values, ok := parent[path[len(path)-1]].([]any)
	return values, ok
}

func asObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func stringAt(root map[string]any, key string) string {
	value, _ := root[key].(string)
	return value
}

func rendererText(value any) string {
	text := asObject(value)
	if simple := stringAt(text, "simpleText"); simple != "" {
		return simple
	}
	runs, _ := text["runs"].([]any)
	var result strings.Builder
	for _, run := range runs {
		result.WriteString(stringAt(asObject(run), "text"))
	}
	return result.String()
}

// Keep source labels and language lists deterministic for errors and storage.
func captionSource(kind string) string {
	if kind == "asr" {
		return "generated"
	}
	return "manual"
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
