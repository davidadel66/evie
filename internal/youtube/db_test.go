package youtube

// Acceptance tests derived from
// cmd/evie/docs/active/youtube-transcripts.spec.md before implementation.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var (
	_ func() (*sql.DB, error)       = OpenDB
	_ func() (*sql.DB, error)       = OpenDBReadOnly
	_ func(string) (*sql.DB, error) = OpenDBAt
)

const testTimestamp = "2026-08-12T15:04:05Z"

func newYouTubeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "transcripts.db"))
	if err != nil {
		t.Fatalf("OpenDBAt: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpenDBAtCreatesDeclaredSchema(t *testing.T) {
	db := newYouTubeTestDB(t)

	wantObjects := map[string]string{
		"channels":           "table",
		"videos":             "table",
		"transcripts":        "table",
		"transcript_states":  "table",
		"transcript_fts":     "table",
		"transcript_library": "view",
	}
	for name, wantType := range wantObjects {
		var gotType string
		err := db.QueryRow(
			`SELECT type FROM sqlite_master WHERE name = ?`, name,
		).Scan(&gotType)
		if err != nil {
			t.Errorf("schema object %q: %v", name, err)
			continue
		}
		if gotType != wantType {
			t.Errorf("schema object %q type = %q, want %q", name, gotType, wantType)
		}
	}

	for _, index := range []string{"videos_channel_id_idx", "transcripts_video_language_idx"} {
		var objectType string
		if err := db.QueryRow(`SELECT type FROM sqlite_master WHERE name = ?`, index).Scan(&objectType); err != nil {
			t.Errorf("required index %q: %v", index, err)
		} else if objectType != "index" {
			t.Errorf("schema object %q type = %q, want index", index, objectType)
		}
	}

	wantColumns := map[string][]string{
		"channels": {
			"id", "youtube_id", "name", "handle", "url", "legacy_name",
			"legacy_key", "created_at", "updated_at",
		},
		"videos": {
			"id", "youtube_id", "legacy_key", "channel_id", "title", "url",
			"published_at", "duration_seconds", "created_at", "updated_at",
		},
		"transcripts": {
			"id", "artifact_key", "video_id", "legacy_channel_id", "language_code",
			"language_name", "source", "text", "word_count", "retrieved_at",
			"source_path", "source_sha256",
		},
		"transcript_states": {
			"video_id", "language_code", "status", "detail", "checked_at",
		},
	}
	for table, want := range wantColumns {
		rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		var got []string
		for rows.Next() {
			var cid, notNull, pk int
			var name, typ string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				rows.Close()
				t.Fatalf("scan table_info(%s): %v", table, err)
			}
			got = append(got, name)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close table_info(%s): %v", table, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s columns = %v, want %v", table, got, want)
		}
	}

	rows, err := db.Query(`SELECT * FROM transcript_library LIMIT 0`)
	if err != nil {
		t.Fatalf("query transcript_library: %v", err)
	}
	gotViewColumns, err := rows.Columns()
	_ = rows.Close()
	if err != nil {
		t.Fatalf("transcript_library columns: %v", err)
	}
	wantViewColumns := []string{
		"transcript_id", "video_id", "channel_id", "channel_name",
		"legacy_channel_name", "channel_handle", "title", "url", "published_at",
		"duration_seconds", "language_code", "language_name", "source",
		"word_count", "retrieved_at", "text",
	}
	if !reflect.DeepEqual(gotViewColumns, wantViewColumns) {
		t.Errorf("transcript_library columns = %v, want %v", gotViewColumns, wantViewColumns)
	}
}

func TestOpenDBAtPragmasApplyToEveryPooledConnection(t *testing.T) {
	db := newYouTubeTestDB(t)
	db.SetMaxOpenConns(4)

	ctx := context.Background()
	var conns []*sql.Conn
	defer func() {
		for _, conn := range conns {
			_ = conn.Close()
		}
	}()

	// Retaining each connection forces database/sql to open distinct pooled
	// connections, proving these are DSN settings rather than one-off PRAGMAs.
	for i := 0; i < 4; i++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		conns = append(conns, conn)

		var foreignKeys, busyTimeout int
		if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatalf("connection %d foreign_keys: %v", i, err)
		}
		if err := conn.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
			t.Fatalf("connection %d busy_timeout: %v", i, err)
		}
		if foreignKeys != 1 {
			t.Errorf("connection %d foreign_keys = %d, want 1", i, foreignKeys)
		}
		if busyTimeout != 5000 {
			t.Errorf("connection %d busy_timeout = %d, want 5000", i, busyTimeout)
		}
	}
}

func TestSchemaEnforcesIdentityForeignKeysAndArtifactKinds(t *testing.T) {
	db := newYouTubeTestDB(t)

	if _, err := db.Exec(`INSERT INTO videos
		(youtube_id, channel_id, title, created_at, updated_at)
		VALUES ('abcdefghijk', 999, 'orphan', ?, ?)`, testTimestamp, testTimestamp); err == nil {
		t.Fatal("video with nonexistent channel inserted; foreign keys must be enforced")
	}

	channelID := insertTestChannel(t, db, "UCaaaaaaaaaaaaaaaaaaaaaa", "Channel", "")
	videoID := insertTestVideo(t, db, channelID, "abcdefghijk", "Video")

	if _, err := db.Exec(`INSERT INTO channels
		(youtube_id, name, created_at, updated_at) VALUES (?, 'duplicate', ?, ?)`,
		"UCaaaaaaaaaaaaaaaaaaaaaa", testTimestamp, testTimestamp); err == nil {
		t.Fatal("duplicate non-NULL channels.youtube_id inserted")
	}
	if _, err := db.Exec(`INSERT INTO videos
		(youtube_id, channel_id, title, created_at, updated_at) VALUES (?, ?, 'duplicate', ?, ?)`,
		"abcdefghijk", channelID, testTimestamp, testTimestamp); err == nil {
		t.Fatal("duplicate non-NULL videos.youtube_id inserted")
	}

	badArtifacts := []struct {
		name       string
		source     string
		legacyID   any
		sourcePath any
		hash       any
	}{
		{"unknown source", "machine", nil, nil, nil},
		{"legacy missing provenance", "legacy", nil, nil, nil},
		{"remote carrying file provenance", "manual", channelID, "/tmp/a.txt", "abc"},
	}
	for _, tc := range badArtifacts {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.Exec(`INSERT INTO transcripts
				(artifact_key, video_id, legacy_channel_id, language_code, language_name,
				 source, text, word_count, retrieved_at, source_path, source_sha256)
				VALUES (?, ?, ?, 'en', 'English', ?, 'text', 1, ?, ?, ?)`,
				"bad:"+tc.name, videoID, tc.legacyID, tc.source, testTimestamp, tc.sourcePath, tc.hash)
			if err == nil {
				t.Fatalf("invalid %s artifact inserted", tc.name)
			}
		})
	}

	for _, status := range []string{"ready", "no_captions", "language_unavailable", "unavailable"} {
		if _, err := db.Exec(`INSERT INTO transcript_states
			(video_id, language_code, status, checked_at) VALUES (?, ?, ?, ?)`,
			videoID, status, status, testTimestamp); err != nil {
			t.Errorf("valid state %q rejected: %v", status, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO transcript_states
		(video_id, language_code, status, checked_at) VALUES (?, 'xx', 'timeout', ?)`,
		videoID, testTimestamp); err == nil {
		t.Fatal("non-terminal state status inserted")
	}
}

func TestOpenDBAtIsIdempotentAndPreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcripts.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("first OpenDBAt: %v", err)
	}
	assertMode(t, path, 0o600)
	if _, err := db.Exec(`INSERT INTO channels
		(youtube_id, name, created_at, updated_at) VALUES ('UCaaaaaaaaaaaaaaaaaaaaaa', 'survivor', ?, ?)`,
		testTimestamp, testTimestamp); err != nil {
		t.Fatalf("insert before reopen: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatalf("second OpenDBAt: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var name string
	if err := db.QueryRow(`SELECT name FROM channels WHERE youtube_id = 'UCaaaaaaaaaaaaaaaaaaaaaa'`).Scan(&name); err != nil {
		t.Fatalf("read preserved row: %v", err)
	}
	if name != "survivor" {
		t.Errorf("preserved channel name = %q, want survivor", name)
	}
}

func TestOpenersProtectCanonicalDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db, err := OpenDB()
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO channels
		(name, created_at, updated_at) VALUES ('canonical', ?, ?)`, testTimestamp, testTimestamp); err != nil {
		_ = db.Close()
		t.Fatalf("write through OpenDB: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close OpenDB: %v", err)
	}

	dir := filepath.Join(home, ".evie", "transcripts")
	path := filepath.Join(dir, "transcripts.db")
	assertMode(t, dir, 0o700)
	assertMode(t, path, 0o600)

	ro, err := OpenDBReadOnly()
	if err != nil {
		t.Fatalf("OpenDBReadOnly: %v", err)
	}
	t.Cleanup(func() { _ = ro.Close() })
	var count int
	if err := ro.QueryRow(`SELECT COUNT(*) FROM channels WHERE name = 'canonical'`).Scan(&count); err != nil {
		t.Fatalf("read through OpenDBReadOnly: %v", err)
	}
	if count != 1 {
		t.Errorf("read-only channel count = %d, want 1", count)
	}
	var busyTimeout int
	if err := ro.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("read-only busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("read-only busy_timeout = %d, want 5000", busyTimeout)
	}
	if _, err := ro.Exec(`DELETE FROM channels`); err == nil {
		t.Fatal("write through OpenDBReadOnly succeeded")
	}
}

func TestOpenDBReadOnlyDoesNotCreateMissingDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db, err := OpenDBReadOnly()
	if err == nil && db != nil {
		err = db.Ping()
		_ = db.Close()
	}
	if err == nil {
		t.Fatal("OpenDBReadOnly succeeded for a missing database")
	}
	path := filepath.Join(home, ".evie", "transcripts", "transcripts.db")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("read-only open created %q (stat error %v)", path, statErr)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %04o, want %04o", path, got, want)
	}
}

func insertTestChannel(t *testing.T, db *sql.DB, youtubeID, name, legacyName string) int64 {
	t.Helper()
	var externalID any
	if youtubeID != "" {
		externalID = youtubeID
	}
	result, err := db.Exec(`INSERT INTO channels
		(youtube_id, name, legacy_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		externalID, name, legacyName, testTimestamp, testTimestamp)
	if err != nil {
		t.Fatalf("insert channel %q: %v", name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("channel LastInsertId: %v", err)
	}
	return id
}

func insertTestVideo(t *testing.T, db *sql.DB, channelID int64, youtubeID, title string) int64 {
	t.Helper()
	var externalID any
	if youtubeID != "" {
		externalID = youtubeID
	}
	result, err := db.Exec(`INSERT INTO videos
		(youtube_id, channel_id, title, url, published_at, duration_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, '2026-08-01', 123, ?, ?)`,
		externalID, channelID, title, canonicalTestURL(youtubeID), testTimestamp, testTimestamp)
	if err != nil {
		t.Fatalf("insert video %q: %v", title, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("video LastInsertId: %v", err)
	}
	return id
}

func canonicalTestURL(videoID string) string {
	if videoID == "" {
		return ""
	}
	return "https://www.youtube.com/watch?v=" + videoID
}
