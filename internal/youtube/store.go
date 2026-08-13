package youtube

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type channelRecord struct {
	YouTubeID string
	Name      string
	Handle    string
	URL       string
}

type videoRecord struct {
	YouTubeID      string
	Title          string
	URL            string
	PublishedAt    sql.NullString
	DurationSecond sql.NullInt64
}

type transcriptRecord struct {
	ArtifactKey  string
	LanguageCode string
	LanguageName string
	Source       string
	Text         string
	WordCount    int
	RetrievedAt  string
}

type stateRecord struct {
	LanguageCode string
	Status       string
	Detail       string
	CheckedAt    string
}

// saveRemote stores one successful online fetch as a single atomic unit.
func saveRemote(ctx context.Context, db *sql.DB, channel channelRecord, video videoRecord, transcript transcriptRecord, state stateRecord, now string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remote transcript save: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	channelID, err := resolveRemoteChannel(ctx, tx, channel, video.YouTubeID, now)
	if err != nil {
		return err
	}
	videoID, err := upsertRemoteVideo(ctx, tx, channelID, video, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO transcripts
		(artifact_key, video_id, language_code, language_name, source, text,
		 word_count, retrieved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(artifact_key) DO UPDATE SET
			video_id = excluded.video_id,
			language_code = excluded.language_code,
			language_name = excluded.language_name,
			source = excluded.source,
			text = excluded.text,
			word_count = excluded.word_count,
			retrieved_at = excluded.retrieved_at`,
		transcript.ArtifactKey, videoID, transcript.LanguageCode, transcript.LanguageName,
		transcript.Source, transcript.Text, transcript.WordCount, transcript.RetrievedAt); err != nil {
		return fmt.Errorf("upsert remote transcript: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO transcript_states
		(video_id, language_code, status, detail, checked_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(video_id, language_code) DO UPDATE SET
			status = excluded.status,
			detail = excluded.detail,
			checked_at = excluded.checked_at`,
		videoID, state.LanguageCode, state.Status, state.Detail, state.CheckedAt); err != nil {
		return fmt.Errorf("upsert transcript state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remote transcript save: %w", err)
	}
	return nil
}

// saveRemoteState atomically stores metadata and a terminal outcome when the
// player response contained enough identity to create a valid relationship.
func saveRemoteState(ctx context.Context, db *sql.DB, channel channelRecord, video videoRecord, state stateRecord, now string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remote transcript state save: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	channelID, err := resolveRemoteChannel(ctx, tx, channel, video.YouTubeID, now)
	if err != nil {
		return err
	}
	videoID, err := upsertRemoteVideo(ctx, tx, channelID, video, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO transcript_states
		(video_id, language_code, status, detail, checked_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(video_id, language_code) DO UPDATE SET
			status = excluded.status,
			detail = excluded.detail,
			checked_at = excluded.checked_at`,
		videoID, state.LanguageCode, state.Status, state.Detail, state.CheckedAt); err != nil {
		return fmt.Errorf("upsert transcript state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remote transcript state save: %w", err)
	}
	return nil
}

func resolveRemoteChannel(ctx context.Context, tx *sql.Tx, channel channelRecord, youtubeVideoID, now string) (int64, error) {
	var currentChannelID int64
	var currentYouTubeID, currentLegacyKey sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT c.id, c.youtube_id, c.legacy_key
		FROM videos v JOIN channels c ON c.id = v.channel_id
		WHERE v.youtube_id = ?`, youtubeVideoID).
		Scan(&currentChannelID, &currentYouTubeID, &currentLegacyKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("find video's current channel: %w", err)
	}

	var realChannelID int64
	realErr := tx.QueryRowContext(ctx, `SELECT id FROM channels WHERE youtube_id = ?`, channel.YouTubeID).Scan(&realChannelID)
	if realErr != nil && !errors.Is(realErr, sql.ErrNoRows) {
		return 0, fmt.Errorf("find YouTube channel: %w", realErr)
	}

	if errors.Is(realErr, sql.ErrNoRows) && err == nil && !currentYouTubeID.Valid && currentLegacyKey.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE channels
			SET youtube_id = ?, name = ?, handle = ?, url = ?, updated_at = ?
			WHERE id = ?`, channel.YouTubeID, channel.Name, channel.Handle, channel.URL, now, currentChannelID); err != nil {
			return 0, fmt.Errorf("enrich imported channel: %w", err)
		}
		return currentChannelID, nil
	}

	if errors.Is(realErr, sql.ErrNoRows) {
		result, err := tx.ExecContext(ctx, `INSERT INTO channels
			(youtube_id, name, handle, url, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, channel.YouTubeID, channel.Name, channel.Handle, channel.URL, now, now)
		if err != nil {
			return 0, fmt.Errorf("insert YouTube channel: %w", err)
		}
		realChannelID, err = result.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("read inserted YouTube channel ID: %w", err)
		}
		return realChannelID, nil
	}

	if _, err := tx.ExecContext(ctx, `UPDATE channels
		SET name = ?, handle = ?, url = ?, updated_at = ? WHERE id = ?`,
		channel.Name, channel.Handle, channel.URL, now, realChannelID); err != nil {
		return 0, fmt.Errorf("update YouTube channel: %w", err)
	}
	return realChannelID, nil
}

func upsertRemoteVideo(ctx context.Context, tx *sql.Tx, channelID int64, video videoRecord, now string) (int64, error) {
	if _, err := tx.ExecContext(ctx, `INSERT INTO videos
		(youtube_id, channel_id, title, url, published_at, duration_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(youtube_id) DO UPDATE SET
			channel_id = excluded.channel_id,
			title = excluded.title,
			url = excluded.url,
			published_at = excluded.published_at,
			duration_seconds = excluded.duration_seconds,
			updated_at = excluded.updated_at`,
		video.YouTubeID, channelID, video.Title, video.URL, video.PublishedAt,
		video.DurationSecond, now, now); err != nil {
		return 0, fmt.Errorf("upsert YouTube video: %w", err)
	}
	var videoID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM videos WHERE youtube_id = ?`, video.YouTubeID).Scan(&videoID); err != nil {
		return 0, fmt.Errorf("read YouTube video ID: %w", err)
	}
	return videoID, nil
}
