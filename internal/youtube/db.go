// Package youtube owns Evie's durable YouTube transcript library.
package youtube

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS channels (
    id                  INTEGER PRIMARY KEY,
    youtube_id          TEXT UNIQUE,
    name                TEXT NOT NULL,
    handle              TEXT NOT NULL DEFAULT '',
    url                 TEXT NOT NULL DEFAULT '',
    legacy_name         TEXT NOT NULL DEFAULT '',
    legacy_key          TEXT UNIQUE,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS videos (
    id                  INTEGER PRIMARY KEY,
    youtube_id          TEXT UNIQUE,
    legacy_key          TEXT UNIQUE,
    channel_id          INTEGER NOT NULL REFERENCES channels(id),
    title               TEXT NOT NULL,
    url                 TEXT NOT NULL DEFAULT '',
    published_at        TEXT,
    duration_seconds    INTEGER,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS videos_channel_id_idx ON videos(channel_id);

CREATE TABLE IF NOT EXISTS transcripts (
    id                  INTEGER PRIMARY KEY,
    artifact_key        TEXT NOT NULL UNIQUE,
    video_id            INTEGER NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    legacy_channel_id   INTEGER REFERENCES channels(id),
    language_code       TEXT NOT NULL,
    language_name       TEXT NOT NULL,
    source              TEXT NOT NULL CHECK (source IN ('manual', 'generated', 'legacy')),
    text                TEXT NOT NULL,
    word_count          INTEGER NOT NULL,
    retrieved_at        TEXT NOT NULL,
    source_path         TEXT UNIQUE,
    source_sha256       TEXT,
    CHECK (
        (source = 'legacy' AND source_path IS NOT NULL
         AND source_sha256 IS NOT NULL AND legacy_channel_id IS NOT NULL)
        OR
        (source <> 'legacy' AND source_path IS NULL
         AND source_sha256 IS NULL AND legacy_channel_id IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS transcripts_video_language_idx
    ON transcripts(video_id, language_code);

CREATE TABLE IF NOT EXISTS transcript_states (
    video_id            INTEGER NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    language_code       TEXT NOT NULL,
    status              TEXT NOT NULL CHECK (status IN (
                            'ready', 'no_captions',
                            'language_unavailable', 'unavailable'
                        )),
    detail              TEXT NOT NULL DEFAULT '',
    checked_at          TEXT NOT NULL,
    PRIMARY KEY (video_id, language_code)
);

CREATE VIEW IF NOT EXISTS transcript_library AS
SELECT
    t.id AS transcript_id,
    v.youtube_id AS video_id,
    c.youtube_id AS channel_id,
    c.name AS channel_name,
    COALESCE(lc.legacy_name, '') AS legacy_channel_name,
    c.handle AS channel_handle,
    v.title AS title,
    v.url AS url,
    v.published_at AS published_at,
    v.duration_seconds AS duration_seconds,
    t.language_code AS language_code,
    t.language_name AS language_name,
    t.source AS source,
    t.word_count AS word_count,
    t.retrieved_at AS retrieved_at,
    t.text AS text
FROM transcripts t
JOIN videos v ON v.id = t.video_id
JOIN channels c ON c.id = v.channel_id
LEFT JOIN channels lc ON lc.id = t.legacy_channel_id
WHERE
    (t.source IN ('manual', 'generated') AND NOT EXISTS (
        SELECT 1
        FROM transcripts preferred
        WHERE preferred.video_id = t.video_id
          AND preferred.language_code = t.language_code
          AND preferred.source IN ('manual', 'generated')
          AND (
              CASE preferred.source WHEN 'manual' THEN 0 ELSE 1 END
                  < CASE t.source WHEN 'manual' THEN 0 ELSE 1 END
              OR (
                  preferred.source = t.source
                  AND (preferred.retrieved_at > t.retrieved_at
                       OR (preferred.retrieved_at = t.retrieved_at AND preferred.id > t.id))
              )
          )
    ))
    OR
    (t.source = 'legacy' AND NOT EXISTS (
        SELECT 1
        FROM transcripts remote
        WHERE remote.video_id = t.video_id
          AND remote.language_code = t.language_code
          AND remote.source IN ('manual', 'generated')
    ));

CREATE VIRTUAL TABLE IF NOT EXISTS transcript_fts USING fts5(
    transcript_id UNINDEXED,
    channel,
    title,
    text,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS transcripts_fts_insert
AFTER INSERT ON transcripts
BEGIN
    INSERT INTO transcript_fts(rowid, transcript_id, channel, title, text)
    SELECT NEW.id, NEW.id, c.name, v.title, NEW.text
    FROM videos v
    JOIN channels c ON c.id = v.channel_id
    WHERE v.id = NEW.video_id;
END;

CREATE TRIGGER IF NOT EXISTS transcripts_fts_update
AFTER UPDATE OF text, video_id ON transcripts
BEGIN
    DELETE FROM transcript_fts WHERE rowid = OLD.id;
    INSERT INTO transcript_fts(rowid, transcript_id, channel, title, text)
    SELECT NEW.id, NEW.id, c.name, v.title, NEW.text
    FROM videos v
    JOIN channels c ON c.id = v.channel_id
    WHERE v.id = NEW.video_id;
END;

CREATE TRIGGER IF NOT EXISTS transcripts_fts_delete
AFTER DELETE ON transcripts
BEGIN
    DELETE FROM transcript_fts WHERE rowid = OLD.id;
END;

CREATE TRIGGER IF NOT EXISTS videos_fts_update
AFTER UPDATE OF title, channel_id ON videos
BEGIN
    UPDATE transcript_fts
    SET title = NEW.title,
        channel = (SELECT name FROM channels WHERE id = NEW.channel_id)
    WHERE rowid IN (SELECT id FROM transcripts WHERE video_id = NEW.id);
END;

CREATE TRIGGER IF NOT EXISTS channels_fts_update
AFTER UPDATE OF name ON channels
BEGIN
    UPDATE transcript_fts
    SET channel = NEW.name
    WHERE rowid IN (
        SELECT t.id
        FROM transcripts t
        JOIN videos v ON v.id = t.video_id
        WHERE v.channel_id = NEW.id
    );
END;
`

const writePragmas = "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"

// OpenDB opens the canonical writable transcript database.
func OpenDB() (*sql.DB, error) {
	return OpenDBContext(context.Background())
}

func OpenDBContext(ctx context.Context) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := databasePath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create transcript database directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("secure transcript database directory: %w", err)
	}
	return OpenDBAtContext(ctx, path)
}

// OpenDBReadOnly opens the canonical database without creating it or applying schema.
func OpenDBReadOnly() (*sql.DB, error) {
	return OpenDBReadOnlyContext(context.Background())
}

func OpenDBReadOnlyContext(ctx context.Context) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := databasePath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open transcript database read-only: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open transcript database read-only: %w", err)
	}
	return db, nil
}

// OpenDBAt opens a writable database at path and applies the idempotent schema.
func OpenDBAt(path string) (*sql.DB, error) {
	return OpenDBAtContext(context.Background(), path)
}

func OpenDBAtContext(ctx context.Context, path string) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+writePragmas)
	if err != nil {
		return nil, fmt.Errorf("open transcript database: %w", err)
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create transcript schema: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("secure transcript database: %w", err)
	}
	return db, nil
}

func databasePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".evie", "transcripts", "transcripts.db"), nil
}
