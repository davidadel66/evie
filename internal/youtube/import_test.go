package youtube

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var _ func(context.Context, *sql.DB, string, string, func(ImportEvent)) (ImportResult, error) = ImportLegacy

func writeCorpusFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func makeCollection(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir collection %q: %v", name, err)
	}
	return path
}

// ImportResult's concrete representation is deliberately executor-owned. This
// helper accepts either an integer field/method or a collection carrying the
// promised summary item, without imposing event/failure struct shapes.
func importMetric(t *testing.T, result ImportResult, name string) int {
	t.Helper()
	v := reflect.ValueOf(result)
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			t.Fatalf("ImportResult is nil while reading %q", name)
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		t.Fatalf("ImportResult has no %q summary", name)
	}

	field := v.FieldByNameFunc(func(candidate string) bool {
		return strings.EqualFold(candidate, name)
	})
	if field.IsValid() {
		return metricValue(t, field, name)
	}

	resultValue := reflect.ValueOf(result)
	resultType := resultValue.Type()
	for i := 0; i < resultType.NumMethod(); i++ {
		candidate := resultType.Method(i)
		if !strings.EqualFold(candidate.Name, name) {
			continue
		}
		method := resultValue.Method(i)
		if method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
			return metricValue(t, method.Call(nil)[0], name)
		}
	}
	t.Fatalf("ImportResult does not carry promised %q summary", name)
	return 0
}

func metricValue(t *testing.T, value reflect.Value, name string) int {
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
		t.Fatalf("ImportResult %q summary has unsupported type %s", name, value.Type())
		return 0
	}
}

func assertImportSummary(t *testing.T, result ImportResult, seen, inserted, updated, skipped, matched, unmatched, failed, warnings int) {
	t.Helper()
	want := map[string]int{
		"seen": seen, "inserted": inserted, "updated": updated,
		"skipped": skipped, "matched": matched, "unmatched": unmatched,
		"failed": failed, "warnings": warnings,
	}
	for name, expected := range want {
		if got := importMetric(t, result, name); got != expected {
			t.Errorf("import summary %s = %d, want %d", name, got, expected)
		}
	}
	if seen != inserted+updated+skipped+failed {
		t.Fatalf("invalid test expectation: seen equation does not balance")
	}
	if matched+unmatched != inserted+updated+skipped {
		t.Fatalf("invalid test expectation: match equation does not balance")
	}
}

func TestImportLegacyExactManifestMatchAndBytePreservation(t *testing.T) {
	db := newYouTubeTestDB(t)
	root := t.TempDir()
	collection := makeCollection(t, root, "My Collection")
	body := []byte("  Héllo, 世界\n\nkeep [Music]\tand trailing spaces  \n")
	sourcePath := filepath.Join(collection, "Exact Title.txt")
	writeCorpusFile(t, sourcePath, body)
	writeCorpusFile(t, filepath.Join(collection, "Case Mismatch.txt"), []byte("case-sensitive mismatch"))
	manifest := `{
		"Exact Title": "https://youtu.be/abcdefghijk",
		"case mismatch": "https://www.youtube.com/watch?v=lmnopqrstuv"
	}`
	writeCorpusFile(t, filepath.Join(collection, "channel_videos.json"), []byte(manifest))

	result, err := ImportLegacy(context.Background(), db, root, " EN_us ", nil)
	if err != nil {
		t.Fatalf("ImportLegacy: %v", err)
	}
	assertImportSummary(t, result, 2, 2, 0, 0, 1, 1, 0, 0)

	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		t.Fatalf("absolute source path: %v", err)
	}
	absSource = filepath.Clean(absSource)
	wantHash := sha256.Sum256(body)

	var (
		artifactKey, languageCode, languageName, text, storedPath, hash string
		youtubeID, url                                                  sql.NullString
		wordCount                                                       int
		retrievedAt                                                     string
	)
	err = db.QueryRow(`SELECT t.artifact_key, t.language_code, t.language_name, t.text,
		t.word_count, t.retrieved_at, t.source_path, t.source_sha256,
		v.youtube_id, v.url
		FROM transcripts t JOIN videos v ON v.id = t.video_id
		WHERE t.source_path = ?`, absSource).Scan(
		&artifactKey, &languageCode, &languageName, &text, &wordCount,
		&retrievedAt, &storedPath, &hash, &youtubeID, &url,
	)
	if err != nil {
		t.Fatalf("select imported transcript: %v", err)
	}
	if artifactKey != "file:"+absSource {
		t.Errorf("artifact_key = %q, want %q", artifactKey, "file:"+absSource)
	}
	if text != string(body) {
		t.Errorf("stored text changed bytes: got %q, want %q", text, string(body))
	}
	if !utf8.ValidString(text) {
		t.Fatal("stored valid UTF-8 became invalid")
	}
	if languageCode != "en-us" || languageName != "en-us" {
		t.Errorf("imported languages = (%q, %q), want normalized en-us for both", languageCode, languageName)
	}
	if wordCount != len(strings.Fields(string(body))) {
		t.Errorf("word_count = %d, want %d", wordCount, len(strings.Fields(string(body))))
	}
	decodedHash, decodeErr := hex.DecodeString(hash)
	if storedPath != absSource || decodeErr != nil || !reflect.DeepEqual(decodedHash, wantHash[:]) {
		t.Errorf("source provenance = (%q, %q), want path %q and SHA-256 %x (decode error %v)", storedPath, hash, absSource, wantHash, decodeErr)
	}
	parsedTime, err := time.Parse(time.RFC3339, retrievedAt)
	_, offset := parsedTime.Zone()
	if err != nil || offset != 0 {
		t.Errorf("retrieved_at = %q, want UTC RFC3339 (parse error %v)", retrievedAt, err)
	}
	if !youtubeID.Valid || youtubeID.String != "abcdefghijk" {
		t.Errorf("matched youtube_id = %#v, want abcdefghijk", youtubeID)
	}
	if !url.Valid || url.String != "https://www.youtube.com/watch?v=abcdefghijk" {
		t.Errorf("matched URL = %#v, want canonical watch URL", url)
	}

	unmatchedPath, err := filepath.Abs(filepath.Join(collection, "Case Mismatch.txt"))
	if err != nil {
		t.Fatalf("absolute unmatched path: %v", err)
	}
	var unmatchedID, unmatchedURL sql.NullString
	var legacyKey string
	if err := db.QueryRow(`SELECT v.youtube_id, v.url, v.legacy_key
		FROM transcripts t JOIN videos v ON v.id = t.video_id WHERE t.source_path = ?`, filepath.Clean(unmatchedPath)).
		Scan(&unmatchedID, &unmatchedURL, &legacyKey); err != nil {
		t.Fatalf("select case-mismatched video: %v", err)
	}
	if unmatchedID.Valid || !unmatchedURL.Valid || unmatchedURL.String != "" {
		t.Errorf("unmatched identity = youtube %#v URL %#v, want NULL ID and empty URL", unmatchedID, unmatchedURL)
	}
	if !strings.HasPrefix(legacyKey, "file:") {
		t.Errorf("unmatched legacy_key = %q, want file:<absolute path>", legacyKey)
	}

	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source after import: %v", err)
	}
	if !reflect.DeepEqual(after, body) {
		t.Fatal("ImportLegacy modified its source file")
	}
}

func TestImportLegacyManifestVariantsAndNoRecursion(t *testing.T) {
	db := newYouTubeTestDB(t)
	root := t.TempDir()

	missing := makeCollection(t, root, "a-missing")
	writeCorpusFile(t, filepath.Join(missing, "one.txt"), []byte("one"))

	empty := makeCollection(t, root, "b-empty")
	writeCorpusFile(t, filepath.Join(empty, "two.txt"), []byte("two"))
	writeCorpusFile(t, filepath.Join(empty, "channel_videos.json"), []byte(`{}`))

	malformed := makeCollection(t, root, "c-malformed")
	writeCorpusFile(t, filepath.Join(malformed, "three.txt"), []byte("three"))
	writeCorpusFile(t, filepath.Join(malformed, "channel_videos.json"), []byte(`[]`))

	nested := filepath.Join(missing, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	writeCorpusFile(t, filepath.Join(nested, "ignored.txt"), []byte("must not import"))
	writeCorpusFile(t, filepath.Join(root, "root-level.txt"), []byte("must not import"))
	writeCorpusFile(t, filepath.Join(missing, "ignored.TXT"), []byte("must not import"))

	result, err := ImportLegacy(context.Background(), db, root, "", nil)
	if err != nil {
		t.Fatalf("ImportLegacy: %v", err)
	}
	assertImportSummary(t, result, 3, 3, 0, 0, 0, 3, 0, 1)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcripts`).Scan(&count); err != nil {
		t.Fatalf("count transcripts: %v", err)
	}
	if count != 3 {
		t.Errorf("transcript count = %d, want only 3 immediate lowercase-.txt children", count)
	}
	var nonDefaultLanguages int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcripts WHERE language_code <> 'en' OR language_name <> 'en'`).Scan(&nonDefaultLanguages); err != nil {
		t.Fatalf("count non-default languages: %v", err)
	}
	if nonDefaultLanguages != 0 {
		t.Errorf("%d imports did not default empty language to en", nonDefaultLanguages)
	}
}

func TestImportLegacyManifestUsesStrictVideoInputParsing(t *testing.T) {
	db := newYouTubeTestDB(t)
	root := t.TempDir()
	collection := makeCollection(t, root, "strict-manifest")
	for _, title := range []string{"Duplicate Query", "Shorts Query", "Fragment", "Suffix Host", "Valid"} {
		writeCorpusFile(t, filepath.Join(collection, title+".txt"), []byte(title))
	}
	writeCorpusFile(t, filepath.Join(collection, "channel_videos.json"), []byte(`{
		"Duplicate Query":"https://www.youtube.com/watch?v=abcdefghijk&v=lmnopqrstuv",
		"Shorts Query":"https://www.youtube.com/shorts/abcdefghijk?feature=share",
		"Fragment":"https://youtu.be/abcdefghijk#fragment",
		"Suffix Host":"https://youtube.com.evil.test/watch?v=abcdefghijk",
		"Valid":"https://www.youtube.com/watch?v=abcdefghijk&list=ignored"
	}`))

	result, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("ImportLegacy strict manifest: %v", err)
	}
	assertImportSummary(t, result, 5, 5, 0, 0, 1, 4, 0, 0)
	var matched int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcripts t JOIN videos v ON v.id = t.video_id WHERE v.youtube_id IS NOT NULL`).Scan(&matched); err != nil {
		t.Fatalf("count strict manifest matches: %v", err)
	}
	if matched != 1 {
		t.Errorf("strict manifest matched %d entries, want only the valid watch URL", matched)
	}
}

func TestImportLegacySortsCollectionsAndFilesDeterministically(t *testing.T) {
	db := newYouTubeTestDB(t)
	root := t.TempDir()
	z := makeCollection(t, root, "z-collection")
	a := makeCollection(t, root, "a-collection")
	writeCorpusFile(t, filepath.Join(z, "b.txt"), []byte("z b"))
	writeCorpusFile(t, filepath.Join(z, "a.txt"), []byte("z a"))
	writeCorpusFile(t, filepath.Join(a, "z.txt"), []byte("a z"))
	writeCorpusFile(t, filepath.Join(a, "a.txt"), []byte("a a"))

	result, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("ImportLegacy: %v", err)
	}
	assertImportSummary(t, result, 4, 4, 0, 0, 0, 4, 0, 0)

	rows, err := db.Query(`SELECT source_path FROM transcripts ORDER BY id`)
	if err != nil {
		t.Fatalf("query insertion order: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			t.Fatalf("scan insertion order: %v", err)
		}
		got = append(got, path)
	}
	want := []string{
		filepath.Join(a, "a.txt"), filepath.Join(a, "z.txt"),
		filepath.Join(z, "a.txt"), filepath.Join(z, "b.txt"),
	}
	for i := range want {
		want[i], _ = filepath.Abs(filepath.Clean(want[i]))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("artifact insertion order = %v, want sorted %v", got, want)
	}
}

func TestImportLegacyRejectsBadFilesButContinues(t *testing.T) {
	db := newYouTubeTestDB(t)
	root := t.TempDir()
	collection := makeCollection(t, root, "failures")
	writeCorpusFile(t, filepath.Join(collection, "empty.txt"), nil)
	writeCorpusFile(t, filepath.Join(collection, "invalid.txt"), []byte{0xff, 0xfe, 0xfd})
	writeCorpusFile(t, filepath.Join(collection, "good.txt"), []byte("small valid text"))
	writeCorpusFile(t, filepath.Join(collection, "at-limit.txt"), []byte(strings.Repeat("a", 10<<20)))
	writeCorpusFile(t, filepath.Join(collection, "over-limit.txt"), []byte(strings.Repeat("b", (10<<20)+1)))

	result, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("per-file failures became top-level error: %v", err)
	}
	assertImportSummary(t, result, 5, 2, 0, 0, 0, 2, 3, 0)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcripts`).Scan(&count); err != nil {
		t.Fatalf("count successful imports: %v", err)
	}
	if count != 2 {
		t.Errorf("successful transcript count = %d, want good and exact-10-MiB files", count)
	}
	var exactSize int
	if err := db.QueryRow(`SELECT length(text) FROM transcripts WHERE source_path LIKE '%/at-limit.txt'`).Scan(&exactSize); err != nil {
		t.Fatalf("select exact-limit artifact: %v", err)
	}
	if exactSize != 10<<20 {
		t.Errorf("exact-limit text bytes = %d, want %d", exactSize, 10<<20)
	}
}

func TestImportLegacyRerunReconcilesIdentityBeforeSkippingText(t *testing.T) {
	db := newYouTubeTestDB(t)
	root := t.TempDir()
	collection := makeCollection(t, root, "reconcile")
	source := filepath.Join(collection, "Talk.txt")
	manifestPath := filepath.Join(collection, "channel_videos.json")
	writeCorpusFile(t, source, []byte("original comet words"))

	first, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("first unmatched import: %v", err)
	}
	assertImportSummary(t, first, 1, 1, 0, 0, 0, 1, 0, 0)
	transcriptID := transcriptIDForPath(t, db, source)

	second, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("same-hash rerun: %v", err)
	}
	assertImportSummary(t, second, 1, 0, 0, 1, 0, 1, 0, 0)

	writeCorpusFile(t, manifestPath, []byte(`{"Talk":"https://www.youtube.com/watch?v=abcdefghijk&list=ignored"}`))
	matched, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("reconcile to canonical video: %v", err)
	}
	assertImportSummary(t, matched, 1, 0, 0, 1, 1, 0, 0, 0)
	assertArtifactIdentity(t, db, transcriptID, "abcdefghijk", "https://www.youtube.com/watch?v=abcdefghijk")
	assertPathVideoCount(t, db, source, 0)

	writeCorpusFile(t, manifestPath, []byte(`{"Talk":"https://youtu.be/lmnopqrstuv"}`))
	changedMatch, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("reconcile changed canonical ID: %v", err)
	}
	assertImportSummary(t, changedMatch, 1, 0, 0, 1, 1, 0, 0, 0)
	assertArtifactIdentity(t, db, transcriptID, "lmnopqrstuv", "https://www.youtube.com/watch?v=lmnopqrstuv")

	writeCorpusFile(t, manifestPath, []byte(`{}`))
	unmatched, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("reconcile back to local video: %v", err)
	}
	assertImportSummary(t, unmatched, 1, 0, 0, 1, 0, 1, 0, 0)
	assertArtifactIdentity(t, db, transcriptID, "", "")
	assertPathVideoCount(t, db, source, 1)

	writeCorpusFile(t, source, []byte("updated galaxy words"))
	updated, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("changed-hash update: %v", err)
	}
	assertImportSummary(t, updated, 1, 0, 1, 0, 0, 1, 0, 0)
	if gotID := transcriptIDForPath(t, db, source); gotID != transcriptID {
		t.Errorf("changed hash created transcript id %d, want in-place id %d", gotID, transcriptID)
	}
	assertFTSMatchCount(t, db, "comet", 0)
	assertFTSMatchCount(t, db, "galaxy", 1)
}

func TestImportLegacyArtifactSaveIsAtomicAndContinues(t *testing.T) {
	db := newYouTubeTestDB(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_explode BEFORE INSERT ON transcripts
		WHEN NEW.text LIKE '%explode%'
		BEGIN SELECT RAISE(ABORT, 'forced artifact failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	root := t.TempDir()
	collection := makeCollection(t, root, "atomic")
	writeCorpusFile(t, filepath.Join(collection, "a-bad.txt"), []byte("explode this save"))
	writeCorpusFile(t, filepath.Join(collection, "b-good.txt"), []byte("independent success"))

	result, err := ImportLegacy(context.Background(), db, root, "en", nil)
	if err != nil {
		t.Fatalf("one artifact failure prevented useful progress: %v", err)
	}
	assertImportSummary(t, result, 2, 1, 0, 0, 0, 1, 1, 0)

	badPath, err := filepath.Abs(filepath.Join(collection, "a-bad.txt"))
	if err != nil {
		t.Fatalf("absolute failed artifact path: %v", err)
	}
	var badVideos, goodTranscripts, badFTS int
	if err := db.QueryRow(`SELECT COUNT(*) FROM videos WHERE legacy_key = ?`, "file:"+filepath.Clean(badPath)).Scan(&badVideos); err != nil {
		t.Fatalf("count rolled-back video: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcripts WHERE text = 'independent success'`).Scan(&goodTranscripts); err != nil {
		t.Fatalf("count independent success: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcript_fts WHERE transcript_fts MATCH 'explode'`).Scan(&badFTS); err != nil {
		t.Fatalf("count failed FTS artifact: %v", err)
	}
	if badVideos != 0 || goodTranscripts != 1 || badFTS != 0 {
		t.Errorf("atomic/continue state = bad videos %d, good transcripts %d, bad FTS %d; want 0,1,0",
			badVideos, goodTranscripts, badFTS)
	}
}

func TestImportLegacyUnreadableRootIsFatal(t *testing.T) {
	db := newYouTubeTestDB(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := ImportLegacy(context.Background(), db, missing, "en", nil)
	if err == nil {
		t.Fatal("ImportLegacy returned nil error for an unreadable/missing root")
	}
}

func transcriptIDForPath(t *testing.T, db *sql.DB, path string) int64 {
	t.Helper()
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM transcripts WHERE source_path = ?`, abs).Scan(&id); err != nil {
		t.Fatalf("transcript for %q: %v", abs, err)
	}
	return id
}

func assertArtifactIdentity(t *testing.T, db *sql.DB, transcriptID int64, wantYouTubeID, wantURL string) {
	t.Helper()
	var youtubeID sql.NullString
	var url string
	if err := db.QueryRow(`SELECT v.youtube_id, v.url FROM transcripts t
		JOIN videos v ON v.id = t.video_id WHERE t.id = ?`, transcriptID).Scan(&youtubeID, &url); err != nil {
		t.Fatalf("artifact video identity: %v", err)
	}
	if wantYouTubeID == "" {
		if youtubeID.Valid {
			t.Errorf("artifact youtube_id = %q, want NULL", youtubeID.String)
		}
	} else if !youtubeID.Valid || youtubeID.String != wantYouTubeID {
		t.Errorf("artifact youtube_id = %#v, want %q", youtubeID, wantYouTubeID)
	}
	if url != wantURL {
		t.Errorf("artifact URL = %q, want %q", url, wantURL)
	}
}

func assertPathVideoCount(t *testing.T, db *sql.DB, path string, want int) {
	t.Helper()
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM videos WHERE legacy_key = ?`, "file:"+abs).Scan(&got); err != nil {
		t.Fatalf("count path-keyed videos: %v", err)
	}
	if got != want {
		t.Errorf("path-keyed video count = %d, want %d", got, want)
	}
}
