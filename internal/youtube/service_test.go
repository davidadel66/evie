package youtube

// Stage 3 acceptance tests derived only from
// cmd/evie/docs/active/youtube-transcripts.spec.md.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	_ func(*sql.DB, *Client) *Service                                              = NewService
	_ func(*Service, context.Context, string, string, bool) (FetchResult, error)   = (*Service).Fetch
	_ func(*Service, context.Context, string, ScrapeOptions) (ScrapeResult, error) = (*Service).Scrape
)

const (
	serviceChannelID = "UCssssssssssssssssssssss"
	serviceVideoA    = "AAAAAAAAAAA"
	serviceVideoB    = "BBBBBBBBBBB"
	serviceVideoC    = "CCCCCCCCCCC"
	serviceVideoD    = "DDDDDDDDDDD"
	serviceVideoE    = "EEEEEEEEEEE"
)

func TestServiceFetchUsesPreferredCacheWithoutHTTP(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*testing.T, *sql.DB, int64)
		wantText   string
		wantLang   string
		wantSource string
	}{
		{
			name: "exact language precedes a regional variant",
			setup: func(t *testing.T, db *sql.DB, videoID int64) {
				insertRemoteTranscript(t, db, videoID, "youtube:"+serviceVideoA+":en:generated", "en", "generated", "exact generated")
				insertRemoteTranscript(t, db, videoID, "youtube:"+serviceVideoA+":en-us:manual", "en-us", "manual", "variant manual")
			},
			wantText: "exact generated", wantLang: "en", wantSource: "generated",
		},
		{
			name: "lexically first variant precedes source preference in a later variant",
			setup: func(t *testing.T, db *sql.DB, videoID int64) {
				insertRemoteTranscript(t, db, videoID, "youtube:"+serviceVideoA+":en-gb:generated", "en-gb", "generated", "British generated")
				insertRemoteTranscript(t, db, videoID, "youtube:"+serviceVideoA+":en-us:manual", "en-us", "manual", "American manual")
			},
			wantText: "British generated", wantLang: "en-gb", wantSource: "generated",
		},
		{
			name: "manual precedes generated for one selected language",
			setup: func(t *testing.T, db *sql.DB, videoID int64) {
				legacyID := insertTestChannel(t, db, "", "Imported", "Imported")
				insertLegacyTranscript(t, db, videoID, legacyID, "file:/manual-preference.txt", "en", "legacy copy", "/manual-preference.txt")
				insertRemoteTranscript(t, db, videoID, "youtube:"+serviceVideoA+":en:generated", "en", "generated", "generated copy")
				insertRemoteTranscript(t, db, videoID, "youtube:"+serviceVideoA+":en:manual", "en", "manual", "manual copy")
			},
			wantText: "manual copy", wantLang: "en", wantSource: "manual",
		},
		{
			name: "generated precedes legacy for one selected language",
			setup: func(t *testing.T, db *sql.DB, videoID int64) {
				legacyID := insertTestChannel(t, db, "", "Imported", "Imported")
				insertLegacyTranscript(t, db, videoID, legacyID, "file:/generated-preference.txt", "en", "legacy copy", "/generated-preference.txt")
				insertRemoteTranscript(t, db, videoID, "youtube:"+serviceVideoA+":en:generated", "en", "generated", "generated copy")
			},
			wantText: "generated copy", wantLang: "en", wantSource: "generated",
		},
		{
			name: "newest legacy is used when no remote artifact exists",
			setup: func(t *testing.T, db *sql.DB, videoID int64) {
				legacyID := insertTestChannel(t, db, "", "Imported", "Imported")
				oldID := insertLegacyTranscript(t, db, videoID, legacyID, "file:/old-service.txt", "en", "old legacy", "/old-service.txt")
				newID := insertLegacyTranscript(t, db, videoID, legacyID, "file:/new-service.txt", "en", "new legacy", "/new-service.txt")
				if _, err := db.Exec(`UPDATE transcripts SET retrieved_at = '2026-01-01T00:00:00Z' WHERE id = ?`, oldID); err != nil {
					t.Fatalf("age old legacy artifact: %v", err)
				}
				if _, err := db.Exec(`UPDATE transcripts SET retrieved_at = '2026-02-01T00:00:00Z' WHERE id = ?`, newID); err != nil {
					t.Fatalf("date new legacy artifact: %v", err)
				}
			},
			wantText: "new legacy", wantLang: "en", wantSource: "legacy",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newYouTubeTestDB(t)
			channelID := seedServiceChannel(t, db)
			videoID := insertTestVideo(t, db, channelID, serviceVideoA, "Cached title")
			tc.setup(t, db, videoID)
			// A satisfying artifact wins before the exact requested-language
			// terminal state is consulted, even if the database contains both.
			seedServiceState(t, db, videoID, "en", "language_unavailable", "stale terminal")
			h := newServiceHTTPHarness(t, nil, nil)

			result, err := NewService(db, h.client).Fetch(context.Background(), serviceVideoA, " EN ", false)
			if err != nil {
				t.Fatalf("Fetch cache hit: %v", err)
			}
			if h.totalHits.Load() != 0 {
				t.Fatalf("cache hit made %d HTTP requests", h.totalHits.Load())
			}
			assertFetchResult(t, result, fetchExpectation{
				cached: true, videoID: serviceVideoA, title: "Cached title",
				channelID: serviceChannelID, channelName: "Service Channel",
				channelHandle: "@ServiceChannel", language: tc.wantLang,
				source: tc.wantSource, text: tc.wantText,
			})
		})
	}
}

func TestServiceFetchReturnsCachedTerminalTypedErrorWithoutHTTP(t *testing.T) {
	db := newYouTubeTestDB(t)
	channelID := seedServiceChannel(t, db)
	videoID := insertTestVideo(t, db, channelID, serviceVideoA, "Terminal title")
	seedServiceState(t, db, videoID, "en", "language_unavailable", "available: fr (manual)")
	h := newServiceHTTPHarness(t, nil, nil)

	_, err := NewService(db, h.client).Fetch(context.Background(), serviceVideoA, " EN ", false)
	if err == nil {
		t.Fatal("Fetch returned nil error for cached terminal state")
	}
	kind, detail, cached := terminalErrorParts(t, err)
	if kind != "language_unavailable" || detail != "available: fr (manual)" || !cached {
		t.Errorf("cached terminal = kind %q detail %q cached %t", kind, detail, cached)
	}
	if h.totalHits.Load() != 0 {
		t.Fatalf("cached terminal made %d HTTP requests", h.totalHits.Load())
	}
}

func TestServiceFetchRefreshPreservesOldArtifactAndReadyStateOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		outcome     serviceOutcome
		terminal    bool
		wantErrPart string
	}{
		{"terminal failure", outcomeUnavailable, true, "private fixture"},
		{"transient format failure", outcomeTransient, false, "format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newYouTubeTestDB(t)
			channelID := seedServiceChannel(t, db)
			videoID := insertTestVideo(t, db, channelID, serviceVideoA, "Old title")
			insertRemoteTranscript(t, db, videoID, "youtube:"+serviceVideoA+":en:manual", "en", "manual", "old durable text")
			seedServiceState(t, db, videoID, "en", "ready", "old ready detail")
			h := newServiceHTTPHarness(t, nil, map[string]serviceOutcome{serviceVideoA: tc.outcome})

			_, err := NewService(db, h.client).Fetch(context.Background(), serviceVideoA, "en", true)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantErrPart) {
				t.Fatalf("refresh error = %v, want %q failure", err, tc.wantErrPart)
			}
			if h.watchCount(serviceVideoA) != 1 {
				t.Errorf("refresh watch requests = %d, want 1 fresh attempt", h.watchCount(serviceVideoA))
			}
			var text, status, detail, checked string
			if err := db.QueryRow(`SELECT t.text, s.status, s.detail, s.checked_at
				FROM transcripts t JOIN transcript_states s ON s.video_id = t.video_id
				WHERE t.video_id = ? AND t.language_code = 'en' AND s.language_code = 'en'`, videoID).
				Scan(&text, &status, &detail, &checked); err != nil {
				t.Fatalf("read preserved cache: %v", err)
			}
			if text != "old durable text" || status != "ready" || detail != "old ready detail" || checked != testTimestamp {
				t.Errorf("refresh changed durable cache: text=%q status=%q detail=%q checked=%q", text, status, detail, checked)
			}
			if tc.terminal {
				kind, _, cached := terminalErrorParts(t, err)
				if kind != "unavailable" || cached {
					t.Errorf("fresh terminal = kind %q cached %t", kind, cached)
				}
			} else {
				var terminal *TerminalError
				if errors.As(err, &terminal) {
					t.Errorf("transient refresh was returned as terminal: %#v", terminal)
				}
			}
		})
	}
}

func TestServiceFetchRefreshBypassesCachedTerminalAndReplacesItWithReady(t *testing.T) {
	db := newYouTubeTestDB(t)
	channelID := seedServiceChannel(t, db)
	videoID := insertTestVideo(t, db, channelID, serviceVideoA, "Listed title")
	seedServiceState(t, db, videoID, "en", "no_captions", "old terminal")
	h := newServiceHTTPHarness(t, nil, map[string]serviceOutcome{serviceVideoA: outcomeSuccess})
	h.languages[serviceVideoA] = "en-us"

	result, err := NewService(db, h.client).Fetch(context.Background(), serviceVideoA, "en", true)
	if err != nil {
		t.Fatalf("refresh cached terminal: %v", err)
	}
	if h.watchCount(serviceVideoA) != 1 {
		t.Fatalf("refresh did not bypass terminal cache: watch requests=%d", h.watchCount(serviceVideoA))
	}
	assertFetchResult(t, result, fetchExpectation{
		cached: false, videoID: serviceVideoA, title: "Fetched " + serviceVideoA,
		channelID: serviceChannelID, channelName: "Service Channel",
		channelHandle: "@ServiceChannel", language: "en-us", source: "manual",
		text: "transcript " + serviceVideoA,
	})
	var status string
	if err := db.QueryRow(`SELECT status FROM transcript_states WHERE video_id = ? AND language_code = 'en'`, videoID).Scan(&status); err != nil {
		t.Fatalf("read refreshed state: %v", err)
	}
	if status != "ready" {
		t.Errorf("refreshed state = %q, want ready", status)
	}
}

func TestServiceFetchSuccessfulRemoteSaveIsAtomicAndEnrichesImportedChannel(t *testing.T) {
	db := newYouTubeTestDB(t)
	placeholderID := insertTestChannel(t, db, "", "old-folder", "old-folder")
	if _, err := db.Exec(`UPDATE channels SET legacy_key = '/corpus/old-folder' WHERE id = ?`, placeholderID); err != nil {
		t.Fatalf("set placeholder key: %v", err)
	}
	videoID := insertTestVideo(t, db, placeholderID, serviceVideoA, "Imported title")
	insertLegacyTranscript(t, db, videoID, placeholderID, "file:/corpus/old-folder/a.txt", "en", "legacy text", "/corpus/old-folder/a.txt")
	h := newServiceHTTPHarness(t, nil, map[string]serviceOutcome{serviceVideoA: outcomeSuccess})
	h.sources[serviceVideoA] = "generated"

	result, err := NewService(db, h.client).Fetch(context.Background(), serviceVideoA, "en", true)
	if err != nil {
		t.Fatalf("Fetch remote success: %v", err)
	}
	assertFetchResult(t, result, fetchExpectation{
		cached: false, videoID: serviceVideoA, title: "Fetched " + serviceVideoA,
		channelID: serviceChannelID, channelName: "Service Channel",
		channelHandle: "@ServiceChannel", language: "en", source: "generated",
		text: "transcript " + serviceVideoA,
	})

	var (
		gotChannelID                    int64
		channelCount                    int
		channelName, handle, channelURL string
		title, videoURL, published      string
		duration                        int64
	)
	if err := db.QueryRow(`SELECT id, name, handle, url FROM channels WHERE youtube_id = ?`, serviceChannelID).
		Scan(&gotChannelID, &channelName, &handle, &channelURL); err != nil {
		t.Fatalf("read enriched channel: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM channels`).Scan(&channelCount); err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if gotChannelID != placeholderID || channelCount != 1 || channelName != "Service Channel" || handle != "@ServiceChannel" ||
		channelURL != "https://www.youtube.com/channel/"+serviceChannelID {
		t.Errorf("channel enrichment = id %d count %d name %q handle %q URL %q", gotChannelID, channelCount, channelName, handle, channelURL)
	}
	if err := db.QueryRow(`SELECT title, url, published_at, duration_seconds FROM videos WHERE id = ?`, videoID).
		Scan(&title, &videoURL, &published, &duration); err != nil {
		t.Fatalf("read enriched video: %v", err)
	}
	if title != "Fetched "+serviceVideoA || videoURL != canonicalVideoURL(serviceVideoA) || published != "2026-08-01" || duration != 123 {
		t.Errorf("video metadata = title %q URL %q published %q duration %d", title, videoURL, published, duration)
	}

	var artifactKey, language, source, text, state string
	var words int
	if err := db.QueryRow(`SELECT t.artifact_key, t.language_code, t.source, t.text, t.word_count, s.status
		FROM transcripts t JOIN transcript_states s ON s.video_id = t.video_id AND s.language_code = 'en'
		WHERE t.video_id = ? AND t.source <> 'legacy'`, videoID).
		Scan(&artifactKey, &language, &source, &text, &words, &state); err != nil {
		t.Fatalf("read committed remote artifact/state: %v", err)
	}
	if artifactKey != "youtube:"+serviceVideoA+":en:generated" || language != "en" || source != "generated" ||
		text != "transcript "+serviceVideoA || words != 2 || state != "ready" {
		t.Errorf("remote save = key %q language %q source %q text %q words %d state %q", artifactKey, language, source, text, words, state)
	}
	var provenance int64
	if err := db.QueryRow(`SELECT legacy_channel_id FROM transcripts WHERE source = 'legacy'`).Scan(&provenance); err != nil {
		t.Fatalf("read legacy provenance: %v", err)
	}
	if provenance != placeholderID {
		t.Errorf("legacy provenance = %d, want %d", provenance, placeholderID)
	}
}

func TestServiceFetchReassignsImportedVideoToExistingRealChannel(t *testing.T) {
	db := newYouTubeTestDB(t)
	placeholderID := insertTestChannel(t, db, "", "legacy-folder", "legacy-folder")
	if _, err := db.Exec(`UPDATE channels SET legacy_key = '/corpus/legacy-folder' WHERE id = ?`, placeholderID); err != nil {
		t.Fatalf("set placeholder key: %v", err)
	}
	videoID := insertTestVideo(t, db, placeholderID, serviceVideoA, "Imported title")
	legacyID := insertLegacyTranscript(t, db, videoID, placeholderID, "file:/corpus/legacy-folder/a.txt", "en", "legacy text", "/corpus/legacy-folder/a.txt")
	realID := seedServiceChannel(t, db)
	h := newServiceHTTPHarness(t, nil, map[string]serviceOutcome{serviceVideoA: outcomeSuccess})

	if _, err := NewService(db, h.client).Fetch(context.Background(), serviceVideoA, "en", true); err != nil {
		t.Fatalf("Fetch existing real channel: %v", err)
	}
	var gotChannelID, provenanceID int64
	if err := db.QueryRow(`SELECT channel_id FROM videos WHERE id = ?`, videoID).Scan(&gotChannelID); err != nil {
		t.Fatalf("read reassigned video: %v", err)
	}
	if err := db.QueryRow(`SELECT legacy_channel_id FROM transcripts WHERE id = ?`, legacyID).Scan(&provenanceID); err != nil {
		t.Fatalf("read reassigned provenance: %v", err)
	}
	if gotChannelID != realID || provenanceID != placeholderID {
		t.Errorf("reassignment = channel %d provenance %d, want real %d placeholder %d", gotChannelID, provenanceID, realID, placeholderID)
	}
	assertFTSRow(t, db, legacyID, "Service Channel", "Fetched "+serviceVideoA, "legacy text")
}

func TestServiceFetchRepeatedRefreshUpsertsMutableMetadata(t *testing.T) {
	db := newYouTubeTestDB(t)
	h := newServiceHTTPHarness(t, nil, map[string]serviceOutcome{serviceVideoA: outcomeSuccess})
	service := NewService(db, h.client)
	if _, err := service.Fetch(context.Background(), serviceVideoA, "en", true); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	h.channelName = "Renamed Service Channel"
	h.channelHandle = "@RenamedService"
	h.titlePrefix = "Updated "
	h.publishDate = "2026-08-02"
	h.duration = "456"
	if _, err := service.Fetch(context.Background(), serviceVideoA, "en", true); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	for table, want := range map[string]int{"channels": 1, "videos": 1, "transcripts": 1, "transcript_states": 1} {
		var got int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Errorf("%s rows = %d, want %d after repeated upsert", table, got, want)
		}
	}
	var channelName, handle, title, published string
	var duration int64
	if err := db.QueryRow(`SELECT c.name, c.handle, v.title, v.published_at, v.duration_seconds
		FROM videos v JOIN channels c ON c.id = v.channel_id WHERE v.youtube_id = ?`, serviceVideoA).
		Scan(&channelName, &handle, &title, &published, &duration); err != nil {
		t.Fatalf("read repeated metadata upsert: %v", err)
	}
	if channelName != h.channelName || handle != h.channelHandle || title != h.titlePrefix+serviceVideoA || published != h.publishDate || duration != 456 {
		t.Errorf("upserted metadata = %q %q %q %q %d", channelName, handle, title, published, duration)
	}
	assertFTSMatchCount(t, db, "renamed", 1)
	assertFTSMatchCount(t, db, "updated", 1)
}

func TestServiceFetchDoesNotReturnSuccessBeforeAtomicSaveCommits(t *testing.T) {
	db := newYouTubeTestDB(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_service_ready BEFORE INSERT ON transcript_states
		WHEN NEW.status = 'ready' BEGIN SELECT RAISE(ABORT, 'forced ready-state failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	h := newServiceHTTPHarness(t, nil, map[string]serviceOutcome{serviceVideoA: outcomeSuccess})

	if _, err := NewService(db, h.client).Fetch(context.Background(), serviceVideoA, "en", false); err == nil {
		t.Fatal("Fetch returned success despite a failed atomic database save")
	}
	for _, table := range []string{"channels", "videos", "transcripts", "transcript_states", "transcript_fts"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s has %d rows after failed atomic save, want 0", table, count)
		}
	}
}

func TestServiceFetchDoesNotPersistTransientErrors(t *testing.T) {
	db := newYouTubeTestDB(t)
	channelID := seedServiceChannel(t, db)
	videoID := insertTestVideo(t, db, channelID, serviceVideoA, "Known video")
	h := newServiceHTTPHarness(t, nil, map[string]serviceOutcome{serviceVideoA: outcomeTransient})

	_, err := NewService(db, h.client).Fetch(context.Background(), serviceVideoA, "en", false)
	if err == nil {
		t.Fatal("transient malformed response succeeded")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcript_states WHERE video_id = ?`, videoID).Scan(&count); err != nil {
		t.Fatalf("count states: %v", err)
	}
	if count != 0 {
		t.Errorf("transient error persisted %d terminal states", count)
	}
}

func TestServiceScrapeHonorsPositiveAndAllLimits(t *testing.T) {
	videos := []string{serviceVideoA, serviceVideoB, serviceVideoC}
	for _, tc := range []struct {
		name  string
		limit int
		want  int
	}{
		{"positive newest window", 2, 2},
		{"zero means all", 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newYouTubeTestDB(t)
			outcomes := map[string]serviceOutcome{}
			for _, id := range videos {
				outcomes[id] = outcomeSuccess
			}
			h := newServiceHTTPHarness(t, videos, outcomes)
			result, err := NewService(db, h.client).Scrape(context.Background(), "@ServiceChannel", ScrapeOptions{
				Language: "en", Limit: tc.limit, Delay: 0,
			})
			if err != nil {
				t.Fatalf("Scrape: %v", err)
			}
			assertScrapeCounts(t, result, tc.want, 0, 0, tc.want, 0)
			if got := len(h.watchOrder()); got != tc.want {
				t.Errorf("network attempts = %d, want %d", got, tc.want)
			}
			var videoCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM videos`).Scan(&videoCount); err != nil {
				t.Fatalf("count videos: %v", err)
			}
			if videoCount != tc.want {
				t.Errorf("stored videos = %d, want limited discovered count %d", videoCount, tc.want)
			}
		})
	}
}

func TestServiceScrapeSkipsCacheAndTerminalContinuesSeriallyAndReportsProgress(t *testing.T) {
	db := newYouTubeTestDB(t)
	channelID := seedServiceChannel(t, db)
	cachedVideoID := insertTestVideo(t, db, channelID, serviceVideoA, "stale cached title")
	insertRemoteTranscript(t, db, cachedVideoID, "youtube:"+serviceVideoA+":en-gb:manual", "en-gb", "manual", "cached variant")
	terminalVideoID := insertTestVideo(t, db, channelID, serviceVideoB, "stale terminal title")
	seedServiceState(t, db, terminalVideoID, "en", "no_captions", "predated scrape")

	videos := []string{serviceVideoA, serviceVideoB, serviceVideoC, serviceVideoD, serviceVideoE}
	h := newServiceHTTPHarness(t, videos, map[string]serviceOutcome{
		serviceVideoC: outcomeNoCaptions,
		serviceVideoD: outcomeTransient,
		serviceVideoE: outcomeSuccess,
	})
	const delay = 200 * time.Millisecond
	var events []ScrapeEvent
	started := time.Now()
	result, err := NewService(db, h.client).Scrape(context.Background(), "@ServiceChannel", ScrapeOptions{
		Language: " EN ", Limit: 0, Delay: delay,
		Progress: func(event ScrapeEvent) { events = append(events, event) },
	})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("per-video failures became top-level error: %v", err)
	}
	assertScrapeCounts(t, result, 5, 1, 1, 1, 2)
	assertScrapeFailureIDs(t, result, []string{serviceVideoC, serviceVideoD})

	watches, watchTimes := h.watchAttempts()
	wantWatches := []string{serviceVideoC, serviceVideoD, serviceVideoE}
	if !reflect.DeepEqual(watches, wantWatches) {
		t.Errorf("transcript attempt order = %v, want %v (cache/state must not be attempted)", watches, wantWatches)
	}
	if h.maxActiveFetches() != 1 {
		t.Errorf("maximum simultaneous transcript attempts = %d, want serial 1", h.maxActiveFetches())
	}
	for i := 1; i < len(watchTimes); i++ {
		if gap := watchTimes[i].Sub(watchTimes[i-1]); gap < 150*time.Millisecond {
			t.Errorf("network attempt gap %d = %s, want the configured delay between attempts", i, gap)
		}
	}
	if len(watchTimes) == 3 && time.Since(watchTimes[2]) >= 150*time.Millisecond {
		t.Errorf("Scrape delayed after its last actual attempt: returned %s after final attempt", time.Since(watchTimes[2]))
	}
	if elapsed < 350*time.Millisecond {
		t.Errorf("Scrape elapsed %s, want two delays between three attempts", elapsed)
	}

	wantStatuses := []string{"cached", "terminal_skipped", "failed", "failed", "saved"}
	if len(events) != len(videos) {
		t.Fatalf("progress events = %d, want one for each of %d videos", len(events), len(videos))
	}
	for i, event := range events {
		if got := serviceString(t, event, "video ID", []string{"VideoID", "YouTubeID", "ID"}); got != videos[i] {
			t.Errorf("progress event %d video = %q, want %q", i, got, videos[i])
		}
		status := normalizeServiceStatus(serviceString(t, event, "status", []string{"Status", "Outcome", "Kind"}))
		if status != wantStatuses[i] {
			t.Errorf("progress event %d status = %q, want %q", i, status, wantStatuses[i])
		}
	}

	// Listing metadata is durable even for videos skipped or failed later.
	rows, err := db.Query(`SELECT youtube_id, title FROM videos ORDER BY youtube_id`)
	if err != nil {
		t.Fatalf("query listed metadata: %v", err)
	}
	defer rows.Close()
	var gotIDs, gotTitles []string
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			t.Fatalf("scan listed metadata: %v", err)
		}
		gotIDs = append(gotIDs, id)
		gotTitles = append(gotTitles, title)
	}
	if !reflect.DeepEqual(gotIDs, videos) {
		t.Errorf("upserted video IDs = %v, want %v", gotIDs, videos)
	}
	for i, id := range gotIDs {
		want := "Listed " + id
		if id == serviceVideoE {
			want = "Fetched " + id
		}
		if gotTitles[i] != want {
			t.Errorf("video %s title = %q, want %q", id, gotTitles[i], want)
		}
	}
	var channelName, handle, channelURL string
	if err := db.QueryRow(`SELECT name, handle, url FROM channels WHERE youtube_id = ?`, serviceChannelID).
		Scan(&channelName, &handle, &channelURL); err != nil {
		t.Fatalf("read channel metadata: %v", err)
	}
	if channelName != "Service Channel" || handle != "@ServiceChannel" || channelURL != "https://www.youtube.com/channel/"+serviceChannelID {
		t.Errorf("channel metadata = %q %q %q", channelName, handle, channelURL)
	}

	assertStoredState(t, db, serviceVideoB, "en", "no_captions", "predated scrape")
	assertStoredState(t, db, serviceVideoC, "en", "no_captions", "captions are disabled or absent")
	var transientStates int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcript_states s JOIN videos v ON v.id = s.video_id WHERE v.youtube_id = ?`, serviceVideoD).
		Scan(&transientStates); err != nil {
		t.Fatalf("count transient states: %v", err)
	}
	if transientStates != 0 {
		t.Errorf("transient scrape failure persisted %d states", transientStates)
	}
}

func TestServiceScrapeReturnsFatalListAndDatabaseErrors(t *testing.T) {
	t.Run("channel listing failure", func(t *testing.T) {
		db := newYouTubeTestDB(t)
		h := newServiceHTTPHarness(t, nil, nil)
		h.channelFailure = true
		_, err := NewService(db, h.client).Scrape(context.Background(), "@ServiceChannel", ScrapeOptions{Language: "en"})
		if err == nil {
			t.Fatal("channel listing failure was returned as a normal scrape result")
		}
		if len(h.watchOrder()) != 0 {
			t.Errorf("listing failure still attempted videos: %v", h.watchOrder())
		}
	})

	t.Run("database write failure", func(t *testing.T) {
		db := newYouTubeTestDB(t)
		if err := db.Close(); err != nil {
			t.Fatalf("close database: %v", err)
		}
		h := newServiceHTTPHarness(t, []string{serviceVideoA}, map[string]serviceOutcome{serviceVideoA: outcomeSuccess})
		_, err := NewService(db, h.client).Scrape(context.Background(), "@ServiceChannel", ScrapeOptions{Language: "en"})
		if err == nil {
			t.Fatal("closed database was reduced to a per-video failure")
		}
		if len(h.watchOrder()) != 0 {
			t.Errorf("database metadata failure should be fatal before transcript attempts; got %v", h.watchOrder())
		}
	})
}

func TestServiceScrapeParentCancellationStopsDelayAndLaterVideos(t *testing.T) {
	db := newYouTubeTestDB(t)
	h := newServiceHTTPHarness(t, []string{serviceVideoA, serviceVideoB}, map[string]serviceOutcome{
		serviceVideoA: outcomeSuccess, serviceVideoB: outcomeSuccess,
	})
	service := NewService(db, h.client)
	ctx, cancel := context.WithCancel(context.Background())
	service.sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		<-ctx.Done()
		return ctx.Err()
	}
	_, err := service.Scrape(ctx, "@ServiceChannel", ScrapeOptions{Language: "en", Delay: time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Scrape error = %v, want context.Canceled", err)
	}
	if got := h.watchOrder(); !reflect.DeepEqual(got, []string{serviceVideoA}) {
		t.Fatalf("video attempts after cancellation = %v, want only first video", got)
	}
}

type fetchExpectation struct {
	cached        bool
	videoID       string
	title         string
	channelID     string
	channelName   string
	channelHandle string
	language      string
	source        string
	text          string
}

func assertFetchResult(t *testing.T, result any, want fetchExpectation) {
	t.Helper()
	assertServiceString(t, result, "video ID", want.videoID,
		[]string{"VideoID"}, []string{"YouTubeID"}, []string{"Video", "VideoID"}, []string{"Video", "YouTubeID"}, []string{"Video", "ID"})
	assertServiceString(t, result, "video title", want.title,
		[]string{"Title"}, []string{"VideoTitle"}, []string{"Video", "Title"})
	assertServiceString(t, result, "channel ID", want.channelID,
		[]string{"ChannelID"}, []string{"ChannelYouTubeID"}, []string{"Channel", "YouTubeID"}, []string{"Channel", "ID"})
	assertServiceString(t, result, "channel name", want.channelName,
		[]string{"ChannelName"}, []string{"Channel", "Name"}, []string{"Channel", "Title"})
	assertServiceString(t, result, "channel handle", want.channelHandle,
		[]string{"ChannelHandle"}, []string{"Channel", "Handle"})
	assertServiceString(t, result, "selected language", want.language,
		[]string{"LanguageCode"}, []string{"Language"}, []string{"Transcript", "LanguageCode"}, []string{"Artifact", "LanguageCode"})
	assertServiceString(t, result, "transcript source", want.source,
		[]string{"TranscriptSource"}, []string{"CaptionSource"}, []string{"Transcript", "Source"}, []string{"Artifact", "Source"}, []string{"Source"})
	assertServiceString(t, result, "transcript text", want.text,
		[]string{"Text"}, []string{"TranscriptText"}, []string{"Transcript", "Text"}, []string{"Artifact", "Text"})
	assertServiceCacheSource(t, result, want.cached)
}

func assertServiceCacheSource(t *testing.T, result any, wantCached bool) {
	t.Helper()
	for _, path := range [][]string{{"Cached"}, {"FromCache"}, {"CacheHit"}} {
		if value, ok := servicePathValue(result, path...); ok && value.Kind() == reflect.Bool {
			if value.Bool() != wantCached {
				t.Errorf("fetch cache source = %t, want %t", value.Bool(), wantCached)
			}
			return
		}
	}
	want := "network"
	if wantCached {
		want = "cache"
	}
	for _, path := range [][]string{{"ResultSource"}, {"FetchSource"}, {"Origin"}, {"CacheSource"}} {
		if value, ok := servicePathValue(result, path...); ok && value.Kind() == reflect.String {
			if strings.EqualFold(value.String(), want) {
				return
			}
		}
	}
	t.Fatalf("%T does not identify whether Fetch used cache or network", result)
}

func assertServiceString(t *testing.T, value any, label, want string, paths ...[]string) {
	t.Helper()
	var found []string
	for _, path := range paths {
		candidate, ok := servicePathValue(value, path...)
		if ok && candidate.Kind() == reflect.String {
			found = append(found, candidate.String())
			if strings.EqualFold(candidate.String(), want) {
				return
			}
		}
	}
	if len(found) == 0 {
		t.Fatalf("%T does not carry promised %s", value, label)
	}
	t.Errorf("%s candidates = %q, want %q", label, found, want)
}

func servicePathValue(value any, path ...string) (reflect.Value, bool) {
	current := reflect.ValueOf(value)
	for _, name := range path {
		for current.IsValid() && (current.Kind() == reflect.Pointer || current.Kind() == reflect.Interface) {
			if current.IsNil() {
				return reflect.Value{}, false
			}
			current = current.Elem()
		}
		if !current.IsValid() {
			return reflect.Value{}, false
		}
		if current.Kind() == reflect.Struct {
			field := current.FieldByName(name)
			if field.IsValid() && field.CanInterface() {
				current = field
				continue
			}
		}
		method := current.MethodByName(name)
		if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
			current = method.Call(nil)[0]
			continue
		}
		return reflect.Value{}, false
	}
	for current.IsValid() && (current.Kind() == reflect.Pointer || current.Kind() == reflect.Interface) {
		if current.IsNil() {
			return reflect.Value{}, false
		}
		current = current.Elem()
	}
	return current, current.IsValid() && current.CanInterface()
}

func assertScrapeCounts(t *testing.T, result any, discovered, cached, terminalSkipped, saved, failed int) {
	t.Helper()
	want := map[string]int{
		"discovered":       discovered,
		"cached":           cached,
		"terminal-skipped": terminalSkipped,
		"saved":            saved,
		"failed":           failed,
	}
	for name, expected := range want {
		if got := serviceMetric(t, result, name); got != expected {
			t.Errorf("scrape %s = %d, want %d", name, got, expected)
		}
	}
}

func serviceMetric(t *testing.T, result any, name string) int {
	t.Helper()
	names := []string{strings.ReplaceAll(name, "-", ""), strings.ReplaceAll(name, "-", "_")}
	v := reflect.ValueOf(result)
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			t.Fatalf("nil scrape result while reading %s", name)
		}
		v = v.Elem()
	}
	if v.IsValid() && v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField(); i++ {
			fieldName := v.Type().Field(i).Name
			for _, candidate := range names {
				if strings.EqualFold(strings.ReplaceAll(fieldName, "_", ""), strings.ReplaceAll(candidate, "_", "")) {
					return serviceInteger(t, v.Field(i), name)
				}
			}
		}
	}
	t.Fatalf("%T does not carry promised scrape %s count", result, name)
	return 0
}

func serviceInteger(t *testing.T, value reflect.Value, name string) int {
	t.Helper()
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return 0
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(value.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(value.Uint())
	case reflect.Array, reflect.Slice, reflect.Map:
		return value.Len()
	default:
		t.Fatalf("scrape %s count has unsupported type %s", name, value.Type())
		return 0
	}
}

func assertScrapeFailureIDs(t *testing.T, result any, want []string) {
	t.Helper()
	value, ok := servicePathValue(result, "Failures")
	if !ok {
		value, ok = servicePathValue(result, "Errors")
	}
	if !ok || (value.Kind() != reflect.Slice && value.Kind() != reflect.Array) {
		t.Fatalf("%T does not carry promised concise per-video failures", result)
	}
	var got []string
	for i := 0; i < value.Len(); i++ {
		failure := value.Index(i).Interface()
		got = append(got, serviceString(t, failure, "failure video ID", []string{"VideoID", "YouTubeID", "ID"}))
		if _, ok := servicePathValue(failure, "Err"); !ok {
			if _, ok := servicePathValue(failure, "Error"); !ok {
				t.Errorf("failure for %s has no error/detail", got[len(got)-1])
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("failure IDs = %v, want %v", got, want)
	}
}

func serviceString(t *testing.T, value any, label string, names []string) string {
	t.Helper()
	for _, name := range names {
		if field, ok := servicePathValue(value, name); ok && field.Kind() == reflect.String {
			return field.String()
		}
	}
	t.Fatalf("%T has no %s", value, label)
	return ""
}

func normalizeServiceStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	status = strings.ReplaceAll(status, "-", "_")
	status = strings.ReplaceAll(status, " ", "_")
	switch status {
	case "cache", "cache_hit", "skipped_cached":
		return "cached"
	case "terminal", "terminal_skip", "skipped_terminal":
		return "terminal_skipped"
	case "success":
		return "saved"
	case "error":
		return "failed"
	default:
		return status
	}
}

func seedServiceChannel(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	id := insertTestChannel(t, db, serviceChannelID, "Service Channel", "")
	if _, err := db.Exec(`UPDATE channels SET handle = '@ServiceChannel', url = ? WHERE id = ?`,
		"https://www.youtube.com/channel/"+serviceChannelID, id); err != nil {
		t.Fatalf("complete service channel fixture: %v", err)
	}
	return id
}

func seedServiceState(t *testing.T, db *sql.DB, videoID int64, language, status, detail string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO transcript_states
		(video_id, language_code, status, detail, checked_at) VALUES (?, ?, ?, ?, ?)`,
		videoID, language, status, detail, testTimestamp); err != nil {
		t.Fatalf("insert service state: %v", err)
	}
}

func assertStoredState(t *testing.T, db *sql.DB, videoYouTubeID, language, wantStatus, wantDetail string) {
	t.Helper()
	var status, detail string
	if err := db.QueryRow(`SELECT s.status, s.detail FROM transcript_states s
		JOIN videos v ON v.id = s.video_id WHERE v.youtube_id = ? AND s.language_code = ?`, videoYouTubeID, language).
		Scan(&status, &detail); err != nil {
		t.Fatalf("read state for %s: %v", videoYouTubeID, err)
	}
	if status != wantStatus || !strings.Contains(strings.ToLower(detail), strings.ToLower(wantDetail)) {
		t.Errorf("state for %s = %q %q, want %q containing %q", videoYouTubeID, status, detail, wantStatus, wantDetail)
	}
}

type serviceOutcome string

const (
	outcomeSuccess     serviceOutcome = "success"
	outcomeNoCaptions  serviceOutcome = "no_captions"
	outcomeUnavailable serviceOutcome = "unavailable"
	outcomeTransient   serviceOutcome = "transient"
)

type serviceHTTPHarness struct {
	t              *testing.T
	server         *httptest.Server
	client         *Client
	videos         []string
	outcomes       map[string]serviceOutcome
	languages      map[string]string
	sources        map[string]string
	channelFailure bool
	channelName    string
	channelHandle  string
	titlePrefix    string
	publishDate    string
	duration       string
	totalHits      atomic.Int64
	channelHits    atomic.Int64

	mu             sync.Mutex
	watches        []string
	watchTimes     []time.Time
	activeFetches  int
	maximumFetches int
}

func newServiceHTTPHarness(t *testing.T, videos []string, outcomes map[string]serviceOutcome) *serviceHTTPHarness {
	t.Helper()
	h := &serviceHTTPHarness{
		t: t, videos: append([]string(nil), videos...), outcomes: outcomes,
		languages: make(map[string]string), sources: make(map[string]string),
		channelName: "Service Channel", channelHandle: "@ServiceChannel",
		titlePrefix: "Fetched ", publishDate: "2026-08-01", duration: "123",
	}
	h.server = httptest.NewServer(http.HandlerFunc(h.serveHTTP))
	t.Cleanup(h.server.Close)
	httpClient := h.server.Client()
	httpClient.Transport = rewriteTransport{target: mustURL(t, h.server.URL), base: httpClient.Transport}
	h.client = NewClient(httpClient)
	h.client.sleep = func(context.Context, time.Duration) error { return nil }
	return h
}

func (h *serviceHTTPHarness) serveHTTP(w http.ResponseWriter, r *http.Request) {
	h.totalHits.Add(1)
	switch {
	case strings.HasSuffix(r.URL.Path, "/videos"):
		h.channelHits.Add(1)
		if h.channelFailure {
			fmt.Fprint(w, "<html>upstream shape changed</html>")
			return
		}
		_, _ = w.Write(h.channelPage())
	case r.URL.Path == "/watch":
		id := r.URL.Query().Get("v")
		h.beginFetch(id)
		time.Sleep(20 * time.Millisecond)
		fmt.Fprint(w, `ytcfg.set({"INNERTUBE_API_KEY":"service-key"});`)
	case r.URL.Path == "/youtubei/v1/player":
		var request struct {
			VideoID string `json:"videoId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			h.t.Errorf("decode player request: %v", err)
			h.endFetch()
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		h.writePlayer(w, request.VideoID)
	case r.URL.Path == "/api/timedtext":
		id := r.URL.Query().Get("v")
		_ = json.NewEncoder(w).Encode(map[string]any{"events": []any{
			map[string]any{"segs": []any{map[string]any{"utf8": "transcript " + id}}},
		}})
		h.endFetch()
	default:
		http.NotFound(w, r)
	}
}

func (h *serviceHTTPHarness) writePlayer(w http.ResponseWriter, id string) {
	outcome := h.outcomes[id]
	if outcome == "" {
		outcome = outcomeSuccess
	}
	if outcome == outcomeTransient {
		fmt.Fprint(w, `{"playabilityStatus":`)
		h.endFetch()
		return
	}
	if outcome == outcomeUnavailable {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"playabilityStatus": map[string]any{"status": "ERROR", "reason": "Private fixture reason"},
		})
		h.endFetch()
		return
	}
	language := h.languages[id]
	if language == "" {
		language = "en"
	}
	response := map[string]any{
		"playabilityStatus": map[string]any{"status": "OK"},
		"videoDetails": map[string]any{
			"videoId": id, "title": h.titlePrefix + id, "channelId": serviceChannelID,
			"author": h.channelName, "lengthSeconds": h.duration,
		},
		"microformat": map[string]any{"playerMicroformatRenderer": map[string]any{
			"publishDate": h.publishDate, "ownerProfileUrl": "https://www.youtube.com/" + h.channelHandle,
		}},
	}
	if outcome != outcomeNoCaptions {
		track := map[string]any{
			"baseUrl": "https://www.youtube.com/api/timedtext?v=" + id + "&sig=fixture",
			"name":    map[string]any{"simpleText": "English"}, "languageCode": language,
		}
		if h.sources[id] == "generated" {
			track["kind"] = "asr"
		}
		response["captions"] = map[string]any{"playerCaptionsTracklistRenderer": map[string]any{
			"captionTracks": []any{track},
		}}
	}
	_ = json.NewEncoder(w).Encode(response)
	if outcome == outcomeNoCaptions {
		h.endFetch()
	}
}

func (h *serviceHTTPHarness) channelPage() []byte {
	contents := make([]any, 0, len(h.videos))
	for _, id := range h.videos {
		contents = append(contents, map[string]any{"richItemRenderer": map[string]any{"content": map[string]any{
			"videoRenderer": map[string]any{"videoId": id, "title": map[string]any{"simpleText": "Listed " + id}},
		}}})
	}
	initial := map[string]any{
		"metadata": map[string]any{"channelMetadataRenderer": map[string]any{
			"externalId": serviceChannelID, "title": "Service Channel",
			"vanityChannelUrl": "https://www.youtube.com/@ServiceChannel",
		}},
		"contents": map[string]any{"twoColumnBrowseResultsRenderer": map[string]any{"tabs": []any{
			map[string]any{"tabRenderer": map[string]any{
				"selected": true, "title": "Videos",
				"content": map[string]any{"richGridRenderer": map[string]any{"contents": contents}},
			}},
		}}},
	}
	encoded, err := json.Marshal(initial)
	if err != nil {
		h.t.Fatalf("encode channel fixture: %v", err)
	}
	return []byte(`ytcfg.set({"INNERTUBE_API_KEY":"browse-key","INNERTUBE_CONTEXT":{"client":{"clientName":"WEB","clientVersion":"1.0","visitorData":"visitor"}}}); var ytInitialData = ` + string(encoded) + `;`)
}

func (h *serviceHTTPHarness) beginFetch(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.watches = append(h.watches, id)
	h.watchTimes = append(h.watchTimes, time.Now())
	h.activeFetches++
	if h.activeFetches > h.maximumFetches {
		h.maximumFetches = h.activeFetches
	}
}

func (h *serviceHTTPHarness) endFetch() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.activeFetches > 0 {
		h.activeFetches--
	}
}

func (h *serviceHTTPHarness) watchCount(id string) int {
	count := 0
	for _, got := range h.watchOrder() {
		if got == id {
			count++
		}
	}
	return count
}

func (h *serviceHTTPHarness) watchOrder() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.watches...)
}

func (h *serviceHTTPHarness) watchAttempts() ([]string, []time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.watches...), append([]time.Time(nil), h.watchTimes...)
}

func (h *serviceHTTPHarness) maxActiveFetches() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.maximumFetches
}
