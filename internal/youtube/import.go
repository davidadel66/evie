package youtube

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxLegacyTranscriptBytes = 10 << 20

var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// ImportResult summarizes a legacy corpus import without producing output.
type ImportResult struct {
	Seen      int
	Inserted  int
	Updated   int
	Skipped   int
	Matched   int
	Unmatched int
	Failed    int
	Warnings  []ImportWarning
	Failures  []ImportFailure
}

type ImportWarning struct {
	Collection string
	Err        error
}

type ImportFailure struct {
	Path string
	Err  error
}

type ImportEvent struct {
	Path   string
	Status string
	Err    error
}

type legacyCollection struct {
	path  string
	files []string
}

type legacyArtifact struct {
	path         string
	title        string
	text         string
	hash         string
	language     string
	youtubeID    string
	canonicalURL string
	matched      bool
}

// ImportLegacy copies immediate transcript files from legacy collection directories.
func ImportLegacy(ctx context.Context, db *sql.DB, root, language string, progress func(ImportEvent)) (ImportResult, error) {
	var result ImportResult
	collections, err := collectLegacyCollections(root)
	if err != nil {
		return result, err
	}
	language = normalizeLanguage(language)
	now := time.Now().UTC().Format(time.RFC3339)

	for _, collection := range collections {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("import legacy transcripts: %w", err)
		}
		channelID, err := upsertLegacyChannel(ctx, db, collection.path, now)
		if err != nil {
			return result, err
		}
		manifest, warning := readLegacyManifest(collection.path)
		if warning != nil {
			result.Warnings = append(result.Warnings, ImportWarning{Collection: collection.path, Err: warning})
			emitImportEvent(progress, ImportEvent{Path: collection.path, Status: "warning", Err: warning})
		}

		for _, path := range collection.files {
			result.Seen++
			artifact, err := readLegacyArtifact(path, language, manifest)
			if err != nil {
				recordImportFailure(&result, progress, path, err)
				continue
			}
			status, err := saveLegacyArtifact(ctx, db, channelID, artifact, now)
			if err != nil {
				recordImportFailure(&result, progress, path, err)
				continue
			}
			switch status {
			case "inserted":
				result.Inserted++
			case "updated":
				result.Updated++
			case "skipped":
				result.Skipped++
			}
			if artifact.matched {
				result.Matched++
			} else {
				result.Unmatched++
			}
			emitImportEvent(progress, ImportEvent{Path: path, Status: status})
		}
	}
	return result, nil
}

func collectLegacyCollections(root string) ([]legacyCollection, error) {
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, fmt.Errorf("resolve legacy root: %w", err)
	}
	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return nil, fmt.Errorf("read legacy root %q: %w", absRoot, err)
	}

	var collections []legacyCollection
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(absRoot, entry.Name())
		children, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("read legacy collection %q: %w", path, err)
		}
		var files []string
		for _, child := range children {
			if child.IsDir() || filepath.Ext(child.Name()) != ".txt" {
				continue
			}
			files = append(files, filepath.Join(path, child.Name()))
		}
		if len(files) > 0 {
			sort.Strings(files)
			collections = append(collections, legacyCollection{path: filepath.Clean(path), files: files})
		}
	}
	sort.Slice(collections, func(i, j int) bool { return collections[i].path < collections[j].path })
	return collections, nil
}

func readLegacyManifest(collection string) (map[string]string, error) {
	path := filepath.Join(collection, "channel_videos.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest map[string]string
	if err := json.Unmarshal(data, &manifest); err != nil || manifest == nil {
		if err == nil {
			err = errors.New("expected a JSON object")
		}
		return nil, fmt.Errorf("parse manifest %q: %w", path, err)
	}
	return manifest, nil
}

func readLegacyArtifact(path, language string, manifest map[string]string) (legacyArtifact, error) {
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return legacyArtifact{}, fmt.Errorf("resolve source path: %w", err)
	}
	file, err := os.Open(absPath)
	if err != nil {
		return legacyArtifact{}, fmt.Errorf("open transcript: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxLegacyTranscriptBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return legacyArtifact{}, fmt.Errorf("read transcript: %w", readErr)
	}
	if closeErr != nil {
		return legacyArtifact{}, fmt.Errorf("close transcript: %w", closeErr)
	}
	if len(data) > maxLegacyTranscriptBytes {
		return legacyArtifact{}, fmt.Errorf("transcript exceeds %d bytes", maxLegacyTranscriptBytes)
	}
	if len(data) == 0 {
		return legacyArtifact{}, errors.New("transcript is empty")
	}
	if !utf8.Valid(data) {
		return legacyArtifact{}, errors.New("transcript is not valid UTF-8")
	}

	title := strings.TrimSuffix(filepath.Base(absPath), ".txt")
	youtubeID, matched := parseManifestVideoID(manifest[title])
	digest := sha256.Sum256(data)
	artifact := legacyArtifact{
		path:      absPath,
		title:     title,
		text:      string(data),
		hash:      hex.EncodeToString(digest[:]),
		language:  language,
		youtubeID: youtubeID,
		matched:   matched,
	}
	if matched {
		artifact.canonicalURL = "https://www.youtube.com/watch?v=" + youtubeID
	}
	return artifact, nil
}

func parseManifestVideoID(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	id, _, err := parseVideoInput(raw)
	if err != nil {
		return "", false
	}
	return id, true
}

func upsertLegacyChannel(ctx context.Context, db *sql.DB, path, now string) (int64, error) {
	name := filepath.Base(path)
	if _, err := db.ExecContext(ctx, `INSERT INTO channels
		(name, legacy_name, legacy_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(legacy_key) DO UPDATE SET
			legacy_name = excluded.legacy_name,
			name = CASE WHEN channels.youtube_id IS NULL THEN excluded.name ELSE channels.name END,
			updated_at = excluded.updated_at`, name, name, path, now, now); err != nil {
		return 0, fmt.Errorf("upsert legacy collection %q: %w", path, err)
	}
	var id int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM channels WHERE legacy_key = ?`, path).Scan(&id); err != nil {
		return 0, fmt.Errorf("read legacy collection %q: %w", path, err)
	}
	return id, nil
}

func saveLegacyArtifact(ctx context.Context, db *sql.DB, channelID int64, artifact legacyArtifact, now string) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin import of %q: %w", artifact.path, err)
	}
	defer func() { _ = tx.Rollback() }()

	targetVideoID, err := resolveLegacyVideo(ctx, tx, channelID, artifact, now)
	if err != nil {
		return "", err
	}

	var transcriptID, oldVideoID int64
	var oldHash string
	err = tx.QueryRowContext(ctx, `SELECT id, video_id, source_sha256
		FROM transcripts WHERE source_path = ?`, artifact.path).Scan(&transcriptID, &oldVideoID, &oldHash)
	status := "inserted"
	switch {
	case errors.Is(err, sql.ErrNoRows):
		result, err := tx.ExecContext(ctx, `INSERT INTO transcripts
			(artifact_key, video_id, legacy_channel_id, language_code, language_name,
			 source, text, word_count, retrieved_at, source_path, source_sha256)
			VALUES (?, ?, ?, ?, ?, 'legacy', ?, ?, ?, ?, ?)`,
			"file:"+artifact.path, targetVideoID, channelID, artifact.language, artifact.language,
			artifact.text, len(strings.Fields(artifact.text)), now, artifact.path, artifact.hash)
		if err != nil {
			return "", fmt.Errorf("insert legacy transcript %q: %w", artifact.path, err)
		}
		if _, err := result.LastInsertId(); err != nil {
			return "", fmt.Errorf("read inserted transcript ID for %q: %w", artifact.path, err)
		}
	case err != nil:
		return "", fmt.Errorf("find legacy transcript %q: %w", artifact.path, err)
	case oldHash == artifact.hash:
		status = "skipped"
		if _, err := tx.ExecContext(ctx, `UPDATE transcripts
			SET video_id = ?, legacy_channel_id = ?, language_code = ?, language_name = ?
			WHERE id = ?`, targetVideoID, channelID, artifact.language, artifact.language, transcriptID); err != nil {
			return "", fmt.Errorf("reconcile legacy transcript %q: %w", artifact.path, err)
		}
	default:
		status = "updated"
		if _, err := tx.ExecContext(ctx, `UPDATE transcripts
			SET video_id = ?, legacy_channel_id = ?, language_code = ?, language_name = ?,
				text = ?, word_count = ?, retrieved_at = ?, source_sha256 = ?
			WHERE id = ?`, targetVideoID, channelID, artifact.language, artifact.language,
			artifact.text, len(strings.Fields(artifact.text)), now, artifact.hash, transcriptID); err != nil {
			return "", fmt.Errorf("update legacy transcript %q: %w", artifact.path, err)
		}
	}

	if err == nil && oldVideoID != targetVideoID {
		if _, err := tx.ExecContext(ctx, `DELETE FROM videos
			WHERE id = ? AND youtube_id IS NULL AND legacy_key LIKE 'file:%'
			  AND NOT EXISTS (SELECT 1 FROM transcripts WHERE video_id = videos.id)`, oldVideoID); err != nil {
			return "", fmt.Errorf("remove orphaned legacy video for %q: %w", artifact.path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit import of %q: %w", artifact.path, err)
	}
	return status, nil
}

func resolveLegacyVideo(ctx context.Context, tx *sql.Tx, channelID int64, artifact legacyArtifact, now string) (int64, error) {
	if artifact.matched {
		if _, err := tx.ExecContext(ctx, `INSERT INTO videos
			(youtube_id, channel_id, title, url, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(youtube_id) DO NOTHING`, artifact.youtubeID, channelID, artifact.title,
			artifact.canonicalURL, now, now); err != nil {
			return 0, fmt.Errorf("resolve matched video for %q: %w", artifact.path, err)
		}
		var id int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM videos WHERE youtube_id = ?`, artifact.youtubeID).Scan(&id); err != nil {
			return 0, fmt.Errorf("read matched video for %q: %w", artifact.path, err)
		}
		return id, nil
	}

	legacyKey := "file:" + artifact.path
	if _, err := tx.ExecContext(ctx, `INSERT INTO videos
		(legacy_key, channel_id, title, url, created_at, updated_at)
		VALUES (?, ?, ?, '', ?, ?)
		ON CONFLICT(legacy_key) DO UPDATE SET
			channel_id = excluded.channel_id,
			title = excluded.title,
			updated_at = excluded.updated_at`, legacyKey, channelID, artifact.title, now, now); err != nil {
		return 0, fmt.Errorf("resolve unmatched video for %q: %w", artifact.path, err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM videos WHERE legacy_key = ?`, legacyKey).Scan(&id); err != nil {
		return 0, fmt.Errorf("read unmatched video for %q: %w", artifact.path, err)
	}
	return id, nil
}

func normalizeLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	language = strings.ReplaceAll(language, "_", "-")
	if language == "" {
		return "en"
	}
	return language
}

func recordImportFailure(result *ImportResult, progress func(ImportEvent), path string, err error) {
	result.Failed++
	result.Failures = append(result.Failures, ImportFailure{Path: path, Err: err})
	emitImportEvent(progress, ImportEvent{Path: path, Status: "failed", Err: err})
}

func emitImportEvent(progress func(ImportEvent), event ImportEvent) {
	if progress != nil {
		progress(event)
	}
}
