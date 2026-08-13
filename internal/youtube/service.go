package youtube

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Service coordinates YouTube network access with the durable transcript cache.
type Service struct {
	db     *sql.DB
	client *Client
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
}

// FetchResult is one cached or newly downloaded transcript and its metadata.
type FetchResult struct {
	Cached           bool
	VideoID          string
	Title            string
	VideoURL         string
	PublishedAt      string
	DurationSeconds  int64
	ChannelID        string
	ChannelName      string
	ChannelHandle    string
	ChannelURL       string
	LanguageCode     string
	LanguageName     string
	TranscriptSource string
	Text             string
	WordCount        int
	RetrievedAt      string
}

// ScrapeOptions controls a serial channel scrape. A zero limit means all videos.
type ScrapeOptions struct {
	Language string
	Limit    int
	Delay    time.Duration
	Progress func(ScrapeEvent)
}

// ScrapeResult summarizes a channel scrape without producing output.
type ScrapeResult struct {
	ChannelID       string
	ChannelName     string
	ChannelHandle   string
	ChannelURL      string
	Discovered      int
	Cached          int
	TerminalSkipped int
	Saved           int
	Failed          int
	Failures        []ScrapeFailure
}

// ScrapeFailure describes one video that could not be downloaded or saved.
type ScrapeFailure struct {
	VideoID string
	Title   string
	Err     error
}

// ScrapeEvent reports the completed outcome for one discovered video.
type ScrapeEvent struct {
	VideoID string
	Title   string
	Index   int
	Total   int
	Status  string
	Err     error
}

type serviceDatabaseError struct{ err error }

func (e *serviceDatabaseError) Error() string { return e.err.Error() }
func (e *serviceDatabaseError) Unwrap() error { return e.err }

// NewService builds a silent transcript service over the supplied database and client.
func NewService(db *sql.DB, client *Client) *Service {
	if client == nil {
		client = NewClient(nil)
	}
	return &Service{
		db:     db,
		client: client,
		now:    time.Now,
		sleep:  sleepContext,
	}
}

// Fetch returns a preferred cached artifact unless refresh requests a network attempt.
func (s *Service) Fetch(ctx context.Context, input, language string, refresh bool) (FetchResult, error) {
	videoID, _, err := parseVideoInput(input)
	if err != nil {
		return FetchResult{}, fmt.Errorf("parse YouTube video input: %w", err)
	}
	language = normalizeLanguage(language)

	cached, found, err := s.lookupTranscript(ctx, videoID, language)
	if err != nil {
		return FetchResult{}, err
	}
	if found && !refresh {
		return cached, nil
	}
	if !refresh {
		terminal, found, err := s.lookupTerminalState(ctx, videoID, language)
		if err != nil {
			return FetchResult{}, err
		}
		if found {
			return FetchResult{}, terminal
		}
	}

	return s.fetchRemote(ctx, videoID, language, found, true)
}

// Scrape refreshes channel metadata, then downloads uncached transcripts serially.
func (s *Service) Scrape(ctx context.Context, input string, opts ScrapeOptions) (ScrapeResult, error) {
	var result ScrapeResult
	if opts.Limit < 0 {
		return result, errors.New("scrape limit must be non-negative")
	}
	if opts.Delay < 0 {
		return result, errors.New("scrape delay must be non-negative")
	}
	language := normalizeLanguage(opts.Language)

	page, err := s.client.listChannel(ctx, input, opts.Limit)
	if err != nil {
		return result, fmt.Errorf("list YouTube channel: %w", err)
	}
	result.ChannelID = page.ChannelID
	result.ChannelName = page.ChannelName
	result.ChannelHandle = page.ChannelHandle
	result.ChannelURL = page.ChannelURL
	result.Discovered = len(page.Videos)

	if err := s.saveListing(ctx, page); err != nil {
		return result, err
	}

	attempts := 0
	for index, video := range page.Videos {
		event := ScrapeEvent{VideoID: video.VideoID, Title: video.Title, Index: index + 1, Total: len(page.Videos)}
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("scrape YouTube channel: %w", err)
		}
		if _, found, err := s.lookupTranscript(ctx, video.VideoID, language); err != nil {
			return result, err
		} else if found {
			result.Cached++
			event.Status = "cached"
			emitScrapeEvent(opts.Progress, event)
			continue
		}
		if _, found, err := s.lookupTerminalState(ctx, video.VideoID, language); err != nil {
			return result, err
		} else if found {
			result.TerminalSkipped++
			event.Status = "terminal_skipped"
			emitScrapeEvent(opts.Progress, event)
			continue
		}

		if attempts > 0 && opts.Delay > 0 {
			if err := s.sleep(ctx, opts.Delay); err != nil {
				return result, fmt.Errorf("wait between YouTube transcript attempts: %w", err)
			}
		}
		attempts++
		_, err := s.fetchRemote(ctx, video.VideoID, language, false, false)
		if err != nil {
			var databaseErr *serviceDatabaseError
			if errors.As(err, &databaseErr) {
				return result, err
			}
			result.Failed++
			result.Failures = append(result.Failures, ScrapeFailure{VideoID: video.VideoID, Title: video.Title, Err: err})
			event.Status = "failed"
			event.Err = err
			emitScrapeEvent(opts.Progress, event)
			continue
		}
		result.Saved++
		event.Status = "saved"
		emitScrapeEvent(opts.Progress, event)
	}
	return result, nil
}

func (s *Service) fetchRemote(ctx context.Context, videoID, language string, preserveReady, saveTerminalMetadata bool) (FetchResult, error) {
	fetched, err := s.client.fetchVideo(ctx, videoID, language)
	if err != nil {
		var terminal *TerminalError
		if errors.As(err, &terminal) && !preserveReady {
			var fetchErr *videoFetchError
			if errors.As(err, &fetchErr) && saveTerminalMetadata {
				if persistErr := s.saveTerminalMetadata(ctx, fetchErr.metadata, language, terminal); persistErr != nil {
					return FetchResult{}, persistErr
				}
			} else if persistErr := s.persistTerminalState(ctx, videoID, language, terminal); persistErr != nil {
				return FetchResult{}, persistErr
			}
		}
		return FetchResult{}, fmt.Errorf("fetch YouTube transcript: %w", err)
	}

	now := s.now().UTC().Format(time.RFC3339)
	channel := channelRecord{
		YouTubeID: fetched.ChannelID,
		Name:      fetched.ChannelName,
		Handle:    fetched.ChannelHandle,
		URL:       youtubeOrigin + "/channel/" + fetched.ChannelID,
	}
	video := videoRecord{
		YouTubeID:      fetched.VideoID,
		Title:          fetched.Title,
		URL:            canonicalVideoURL(fetched.VideoID),
		PublishedAt:    sql.NullString{String: fetched.PublishedAt, Valid: fetched.PublishedAt != ""},
		DurationSecond: sql.NullInt64{Int64: fetched.DurationSeconds, Valid: true},
	}
	transcript := transcriptRecord{
		ArtifactKey:  "youtube:" + fetched.VideoID + ":" + fetched.LanguageCode + ":" + fetched.Source,
		LanguageCode: fetched.LanguageCode,
		LanguageName: fetched.LanguageName,
		Source:       fetched.Source,
		Text:         fetched.Text,
		WordCount:    len(strings.Fields(fetched.Text)),
		RetrievedAt:  now,
	}
	state := stateRecord{LanguageCode: language, Status: "ready", CheckedAt: now}
	if err := saveRemote(ctx, s.db, channel, video, transcript, state, now); err != nil {
		return FetchResult{}, databaseError("save fetched YouTube transcript", err)
	}
	return fetchResultFromRemote(fetched, transcript, channel.URL), nil
}

func (s *Service) saveTerminalMetadata(ctx context.Context, fetched videoFetch, language string, terminal *TerminalError) error {
	now := s.now().UTC().Format(time.RFC3339)
	channel := channelRecord{
		YouTubeID: fetched.ChannelID,
		Name:      fetched.ChannelName,
		Handle:    fetched.ChannelHandle,
		URL:       youtubeOrigin + "/channel/" + fetched.ChannelID,
	}
	video := videoRecord{
		YouTubeID:      fetched.VideoID,
		Title:          fetched.Title,
		URL:            canonicalVideoURL(fetched.VideoID),
		PublishedAt:    sql.NullString{String: fetched.PublishedAt, Valid: fetched.PublishedAt != ""},
		DurationSecond: sql.NullInt64{Int64: fetched.DurationSeconds, Valid: true},
	}
	state := stateRecord{
		LanguageCode: language,
		Status:       terminal.Kind,
		Detail:       terminal.Detail,
		CheckedAt:    now,
	}
	if err := saveRemoteState(ctx, s.db, channel, video, state, now); err != nil {
		return databaseError("save terminal YouTube transcript metadata", err)
	}
	return nil
}

func (s *Service) lookupTranscript(ctx context.Context, videoID, language string) (FetchResult, bool, error) {
	if s.db == nil {
		return FetchResult{}, false, databaseError("look up cached YouTube transcript", errors.New("nil database"))
	}
	allowVariants := !strings.Contains(language, "-")
	row := s.db.QueryRowContext(ctx, `SELECT
		v.youtube_id, v.title, v.url, v.published_at, v.duration_seconds,
		c.youtube_id, c.name, c.handle, c.url,
		t.language_code, t.language_name, t.source, t.text, t.word_count, t.retrieved_at
		FROM transcripts t
		JOIN videos v ON v.id = t.video_id
		JOIN channels c ON c.id = v.channel_id
		WHERE v.youtube_id = ?
		  AND (t.language_code = ? OR (? AND substr(t.language_code, 1, length(?) + 1) = ? || '-'))
		ORDER BY CASE WHEN t.language_code = ? THEN 0 ELSE 1 END,
		         t.language_code,
		         CASE t.source WHEN 'manual' THEN 0 WHEN 'generated' THEN 1 ELSE 2 END,
		         t.retrieved_at DESC, t.id DESC
		LIMIT 1`, videoID, language, allowVariants, language, language, language)
	var result FetchResult
	var channelID, publishedAt sql.NullString
	var durationSeconds sql.NullInt64
	if err := row.Scan(
		&result.VideoID, &result.Title, &result.VideoURL, &publishedAt, &durationSeconds,
		&channelID, &result.ChannelName,
		&result.ChannelHandle, &result.ChannelURL, &result.LanguageCode, &result.LanguageName,
		&result.TranscriptSource, &result.Text, &result.WordCount, &result.RetrievedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FetchResult{}, false, nil
		}
		return FetchResult{}, false, databaseError("look up cached YouTube transcript", err)
	}
	result.ChannelID = channelID.String
	result.PublishedAt = publishedAt.String
	result.DurationSeconds = durationSeconds.Int64
	result.Cached = true
	return result, true, nil
}

func (s *Service) lookupTerminalState(ctx context.Context, videoID, language string) (*TerminalError, bool, error) {
	if s.db == nil {
		return nil, false, databaseError("look up cached YouTube transcript state", errors.New("nil database"))
	}
	var status, detail string
	err := s.db.QueryRowContext(ctx, `SELECT s.status, s.detail
		FROM transcript_states s JOIN videos v ON v.id = s.video_id
		WHERE v.youtube_id = ? AND s.language_code = ?`, videoID, language).Scan(&status, &detail)
	if errors.Is(err, sql.ErrNoRows) || status == "ready" {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, databaseError("look up cached YouTube transcript state", err)
	}
	return &TerminalError{Kind: status, Detail: detail, Cached: true}, true, nil
}

func (s *Service) persistTerminalState(ctx context.Context, videoID, language string, terminal *TerminalError) error {
	if s.db == nil {
		return databaseError("save terminal YouTube transcript state", errors.New("nil database"))
	}
	now := s.now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `INSERT INTO transcript_states
		(video_id, language_code, status, detail, checked_at)
		SELECT id, ?, ?, ?, ? FROM videos WHERE youtube_id = ?
		ON CONFLICT(video_id, language_code) DO UPDATE SET
			status = excluded.status,
			detail = excluded.detail,
			checked_at = excluded.checked_at`, language, terminal.Kind, terminal.Detail, now, videoID)
	if err != nil {
		return databaseError("save terminal YouTube transcript state", err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return databaseError("verify terminal YouTube transcript state", err)
	}
	return nil
}

func (s *Service) saveListing(ctx context.Context, page channelPage) error {
	if s.db == nil {
		return databaseError("save YouTube channel listing", errors.New("nil database"))
	}
	now := s.now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return databaseError("begin YouTube channel listing save", err)
	}
	defer func() { _ = tx.Rollback() }()

	channelID, err := resolveListingChannel(ctx, tx, page, now)
	if err != nil {
		return databaseError("save YouTube channel metadata", err)
	}
	for _, listed := range page.Videos {
		if _, err := tx.ExecContext(ctx, `INSERT INTO videos
			(youtube_id, channel_id, title, url, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(youtube_id) DO UPDATE SET
				channel_id = excluded.channel_id,
				title = excluded.title,
				url = excluded.url,
				updated_at = excluded.updated_at`,
			listed.VideoID, channelID, listed.Title, canonicalVideoURL(listed.VideoID), now, now); err != nil {
			return databaseError("upsert listed YouTube video "+listed.VideoID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return databaseError("commit YouTube channel listing save", err)
	}
	return nil
}

func resolveListingChannel(ctx context.Context, tx *sql.Tx, page channelPage, now string) (int64, error) {
	var channelID int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM channels WHERE youtube_id = ?`, page.ChannelID).Scan(&channelID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("find listed YouTube channel: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		for _, video := range page.Videos {
			err = tx.QueryRowContext(ctx, `SELECT c.id
				FROM videos v JOIN channels c ON c.id = v.channel_id
				WHERE v.youtube_id = ? AND c.youtube_id IS NULL AND c.legacy_key IS NOT NULL`, video.VideoID).Scan(&channelID)
			if err == nil {
				break
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return 0, fmt.Errorf("find imported channel for listed video: %w", err)
			}
		}
		if errors.Is(err, sql.ErrNoRows) {
			result, insertErr := tx.ExecContext(ctx, `INSERT INTO channels
				(youtube_id, name, handle, url, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)`, page.ChannelID, page.ChannelName, page.ChannelHandle,
				page.ChannelURL, now, now)
			if insertErr != nil {
				return 0, fmt.Errorf("insert listed YouTube channel: %w", insertErr)
			}
			channelID, insertErr = result.LastInsertId()
			if insertErr != nil {
				return 0, fmt.Errorf("read listed YouTube channel ID: %w", insertErr)
			}
			return channelID, nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE channels
			SET youtube_id = ?, name = ?, handle = ?, url = ?, updated_at = ? WHERE id = ?`,
			page.ChannelID, page.ChannelName, page.ChannelHandle, page.ChannelURL, now, channelID); err != nil {
			return 0, fmt.Errorf("enrich imported channel from listing: %w", err)
		}
		return channelID, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE channels
		SET name = ?, handle = ?, url = ?, updated_at = ? WHERE id = ?`,
		page.ChannelName, page.ChannelHandle, page.ChannelURL, now, channelID); err != nil {
		return 0, fmt.Errorf("update listed YouTube channel: %w", err)
	}
	return channelID, nil
}

func fetchResultFromRemote(fetched videoFetch, transcript transcriptRecord, channelURL string) FetchResult {
	return FetchResult{
		Cached:           false,
		VideoID:          fetched.VideoID,
		Title:            fetched.Title,
		VideoURL:         canonicalVideoURL(fetched.VideoID),
		PublishedAt:      fetched.PublishedAt,
		DurationSeconds:  fetched.DurationSeconds,
		ChannelID:        fetched.ChannelID,
		ChannelName:      fetched.ChannelName,
		ChannelHandle:    fetched.ChannelHandle,
		ChannelURL:       channelURL,
		LanguageCode:     fetched.LanguageCode,
		LanguageName:     fetched.LanguageName,
		TranscriptSource: fetched.Source,
		Text:             fetched.Text,
		WordCount:        transcript.WordCount,
		RetrievedAt:      transcript.RetrievedAt,
	}
}

func databaseError(action string, err error) error {
	return &serviceDatabaseError{err: fmt.Errorf("%s: %w", action, err)}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func emitScrapeEvent(progress func(ScrapeEvent), event ScrapeEvent) {
	if progress != nil {
		progress(event)
	}
}
