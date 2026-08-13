package youtube

import (
	"database/sql"
	"strings"
	"testing"
)

func insertRemoteTranscript(t *testing.T, db *sql.DB, videoID int64, key, language, source, text string) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO transcripts
		(artifact_key, video_id, language_code, language_name, source, text,
		 word_count, retrieved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		key, videoID, language, language, source, text, len(strings.Fields(text)), testTimestamp)
	if err != nil {
		t.Fatalf("insert remote transcript %q: %v", key, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("transcript LastInsertId: %v", err)
	}
	return id
}

func insertLegacyTranscript(t *testing.T, db *sql.DB, videoID, legacyChannelID int64, key, language, text, path string) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO transcripts
		(artifact_key, video_id, legacy_channel_id, language_code, language_name,
		 source, text, word_count, retrieved_at, source_path, source_sha256)
		VALUES (?, ?, ?, ?, ?, 'legacy', ?, ?, ?, ?, ?)`,
		key, videoID, legacyChannelID, language, language, text,
		len(strings.Fields(text)), testTimestamp, path, strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("insert legacy transcript %q: %v", key, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("legacy transcript LastInsertId: %v", err)
	}
	return id
}

func TestTranscriptLibrarySelectsPreferredArtifacts(t *testing.T) {
	db := newYouTubeTestDB(t)
	channelID := insertTestChannel(t, db, "UCaaaaaaaaaaaaaaaaaaaaaa", "Current Channel", "")
	legacyID := insertTestChannel(t, db, "", "Imported Collection", "Imported Collection")
	videoID := insertTestVideo(t, db, channelID, "abcdefghijk", "Preference Video")

	legacyEN := insertLegacyTranscript(t, db, videoID, legacyID, "file:/en.txt", "en", "legacy en", "/en.txt")
	generatedEN := insertRemoteTranscript(t, db, videoID, "youtube:abcdefghijk:en:generated", "en", "generated", "generated en")
	manualEN := insertRemoteTranscript(t, db, videoID, "youtube:abcdefghijk:en:manual", "en", "manual", "manual en")
	_ = legacyEN
	_ = generatedEN

	insertLegacyTranscript(t, db, videoID, legacyID, "file:/fr.txt", "fr", "legacy fr", "/fr.txt")
	generatedFR := insertRemoteTranscript(t, db, videoID, "youtube:abcdefghijk:fr:generated", "fr", "generated", "generated fr")
	legacyDE1 := insertLegacyTranscript(t, db, videoID, legacyID, "file:/de-one.txt", "de", "legacy de one", "/de-one.txt")
	legacyDE2 := insertLegacyTranscript(t, db, videoID, legacyID, "file:/de-two.txt", "de", "legacy de two", "/de-two.txt")

	rows, err := db.Query(`SELECT transcript_id, language_code, source
		FROM transcript_library WHERE video_id = 'abcdefghijk'
		ORDER BY language_code, transcript_id`)
	if err != nil {
		t.Fatalf("query transcript_library: %v", err)
	}
	defer rows.Close()
	type row struct {
		id       int64
		language string
		source   string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.language, &r.source); err != nil {
			t.Fatalf("scan transcript_library: %v", err)
		}
		got = append(got, r)
	}
	want := []row{
		{legacyDE1, "de", "legacy"},
		{legacyDE2, "de", "legacy"},
		{manualEN, "en", "manual"},
		{generatedFR, "fr", "generated"},
	}
	if len(got) != len(want) {
		t.Fatalf("preferred view rows = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("preferred row %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestTranscriptLibraryPreservesLegacyProvenanceThroughEnrichmentAndReassignment(t *testing.T) {
	db := newYouTubeTestDB(t)
	placeholderID := insertTestChannel(t, db, "", "old-folder", "old-folder")
	if _, err := db.Exec(`UPDATE channels SET legacy_key = '/corpus/old-folder' WHERE id = ?`, placeholderID); err != nil {
		t.Fatalf("set placeholder legacy key: %v", err)
	}
	videoID := insertTestVideo(t, db, placeholderID, "abcdefghijk", "Imported exact match")
	insertLegacyTranscript(t, db, videoID, placeholderID, "file:/corpus/old-folder/video.txt", "en", "legacy body", "/corpus/old-folder/video.txt")

	// The first online encounter may enrich the placeholder itself. The mutable
	// channel name changes, but the imported collection name remains visible.
	if _, err := db.Exec(`UPDATE channels SET youtube_id = ?, name = ?, handle = ?, url = ?, updated_at = ? WHERE id = ?`,
		"UCaaaaaaaaaaaaaaaaaaaaaa", "Online Name", "@online", "https://www.youtube.com/channel/UCaaaaaaaaaaaaaaaaaaaaaa", testTimestamp, placeholderID); err != nil {
		t.Fatalf("enrich placeholder: %v", err)
	}
	assertLibraryChannelNames(t, db, "abcdefghijk", "Online Name", "old-folder")

	// If the real channel already exists, membership moves instead. Provenance
	// follows transcripts.legacy_channel_id, not videos.channel_id.
	realID := insertTestChannel(t, db, "UCbbbbbbbbbbbbbbbbbbbbbb", "Other Real Name", "")
	if _, err := db.Exec(`UPDATE videos SET channel_id = ?, updated_at = ? WHERE id = ?`, realID, testTimestamp, videoID); err != nil {
		t.Fatalf("reassign video: %v", err)
	}
	assertLibraryChannelNames(t, db, "abcdefghijk", "Other Real Name", "old-folder")

	var provenanceID int64
	if err := db.QueryRow(`SELECT legacy_channel_id FROM transcripts WHERE video_id = ?`, videoID).Scan(&provenanceID); err != nil {
		t.Fatalf("select legacy provenance: %v", err)
	}
	if provenanceID != placeholderID {
		t.Errorf("legacy_channel_id = %d, want original placeholder %d", provenanceID, placeholderID)
	}
}

func TestTranscriptLibrarySupportsRequestedLanguageVariantOrdering(t *testing.T) {
	db := newYouTubeTestDB(t)
	channelID := insertTestChannel(t, db, "UCaaaaaaaaaaaaaaaaaaaaaa", "Language Channel", "")

	exactVideo := insertTestVideo(t, db, channelID, "abcdefghijk", "Exact wins")
	exactID := insertRemoteTranscript(t, db, exactVideo, "youtube:abcdefghijk:en:generated", "en", "generated", "exact English")
	insertRemoteTranscript(t, db, exactVideo, "youtube:abcdefghijk:en-us:manual", "en-us", "manual", "regional manual")

	variantVideo := insertTestVideo(t, db, channelID, "lmnopqrstuv", "Lexical variant wins")
	lexicalID := insertRemoteTranscript(t, db, variantVideo, "youtube:lmnopqrstuv:en-gb:generated", "en-gb", "generated", "British generated")
	insertRemoteTranscript(t, db, variantVideo, "youtube:lmnopqrstuv:en-us:manual", "en-us", "manual", "American manual")

	// This is the spec's cache-selection ordering expressed entirely through
	// public SQL: exact requested code first, then <code>-* lexically. Artifact
	// source preference has already been resolved by transcript_library within
	// each video/language.
	selection := `SELECT transcript_id FROM transcript_library
		WHERE video_id = ?
		AND (language_code = ? OR language_code LIKE ? || '-%')
		ORDER BY CASE WHEN language_code = ? THEN 0 ELSE 1 END, language_code
		LIMIT 1`
	for _, tc := range []struct {
		videoID string
		wantID  int64
	}{
		{"abcdefghijk", exactID},
		{"lmnopqrstuv", lexicalID},
	} {
		var gotID int64
		if err := db.QueryRow(selection, tc.videoID, "en", "en", "en").Scan(&gotID); err != nil {
			t.Fatalf("select requested language for %s: %v", tc.videoID, err)
		}
		if gotID != tc.wantID {
			t.Errorf("selected transcript for %s = %d, want %d", tc.videoID, gotID, tc.wantID)
		}
	}
}

func assertLibraryChannelNames(t *testing.T, db *sql.DB, videoID, wantCurrent, wantLegacy string) {
	t.Helper()
	var current, legacy string
	if err := db.QueryRow(`SELECT channel_name, legacy_channel_name
		FROM transcript_library WHERE video_id = ?`, videoID).Scan(&current, &legacy); err != nil {
		t.Fatalf("query library channel names: %v", err)
	}
	if current != wantCurrent || legacy != wantLegacy {
		t.Errorf("library names = current %q, legacy %q; want current %q, legacy %q",
			current, legacy, wantCurrent, wantLegacy)
	}
}

func TestFTSTriggersTrackTranscriptAndMetadataChanges(t *testing.T) {
	db := newYouTubeTestDB(t)
	firstChannel := insertTestChannel(t, db, "UCaaaaaaaaaaaaaaaaaaaaaa", "First Channel", "")
	secondChannel := insertTestChannel(t, db, "UCbbbbbbbbbbbbbbbbbbbbbb", "Second Channel", "")
	videoID := insertTestVideo(t, db, firstChannel, "abcdefghijk", "Original Title")
	transcriptID := insertRemoteTranscript(t, db, videoID, "youtube:abcdefghijk:en:manual", "en", "manual", "alpha idolatry omega")

	assertFTSRow(t, db, transcriptID, "First Channel", "Original Title", "alpha idolatry omega")
	assertFTSMatchCount(t, db, "idolatry", 1)

	if _, err := db.Exec(`UPDATE transcripts SET text = 'replacement telescope text', word_count = 3 WHERE id = ?`, transcriptID); err != nil {
		t.Fatalf("update transcript text: %v", err)
	}
	assertFTSMatchCount(t, db, "idolatry", 0)
	assertFTSMatchCount(t, db, "telescope", 1)

	if _, err := db.Exec(`UPDATE videos SET title = 'Nebula Video' WHERE id = ?`, videoID); err != nil {
		t.Fatalf("rename video: %v", err)
	}
	assertFTSMatchCount(t, db, "nebula", 1)

	if _, err := db.Exec(`UPDATE channels SET name = 'Quasar First Channel' WHERE id = ?`, firstChannel); err != nil {
		t.Fatalf("rename channel: %v", err)
	}
	assertFTSMatchCount(t, db, "quasar", 1)

	if _, err := db.Exec(`UPDATE videos SET channel_id = ? WHERE id = ?`, secondChannel, videoID); err != nil {
		t.Fatalf("reassign channel: %v", err)
	}
	assertFTSRow(t, db, transcriptID, "Second Channel", "Nebula Video", "replacement telescope text")
	assertFTSMatchCount(t, db, `channel:"Second Channel"`, 1)
	assertFTSMatchCount(t, db, `channel:"Quasar First Channel"`, 0)

	if _, err := db.Exec(`UPDATE channels SET name = 'Final Cosmos Channel' WHERE id = ?`, secondChannel); err != nil {
		t.Fatalf("rename reassigned channel: %v", err)
	}
	assertFTSMatchCount(t, db, "cosmos", 1)

	if _, err := db.Exec(`DELETE FROM videos WHERE id = ?`, videoID); err != nil {
		t.Fatalf("delete video: %v", err)
	}
	var transcriptCount, ftsCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcripts WHERE id = ?`, transcriptID).Scan(&transcriptCount); err != nil {
		t.Fatalf("count cascaded transcript: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcript_fts WHERE rowid = ?`, transcriptID).Scan(&ftsCount); err != nil {
		t.Fatalf("count cascaded FTS row: %v", err)
	}
	if transcriptCount != 0 || ftsCount != 0 {
		t.Errorf("after video delete: transcripts=%d FTS=%d, want both 0", transcriptCount, ftsCount)
	}
}

func TestFTSSearchJoinReturnsOnlyPreferredArtifactWithSnippet(t *testing.T) {
	db := newYouTubeTestDB(t)
	channelID := insertTestChannel(t, db, "UCaaaaaaaaaaaaaaaaaaaaaa", "Search Channel", "")
	legacyID := insertTestChannel(t, db, "", "Search Import", "Search Import")
	videoID := insertTestVideo(t, db, channelID, "abcdefghijk", "Searchable Lecture")
	insertLegacyTranscript(t, db, videoID, legacyID, "file:/search.txt", "en", "legacy idolatry result", "/search.txt")
	insertRemoteTranscript(t, db, videoID, "youtube:abcdefghijk:en:generated", "en", "generated", "generated idolatry result")
	manualID := insertRemoteTranscript(t, db, videoID, "youtube:abcdefghijk:en:manual", "en", "manual", "manual context around idolatry winner")

	rows, err := db.Query(`SELECT l.transcript_id, l.channel_name, l.title,
		snippet(transcript_fts, 3, '[', ']', '...', 24)
		FROM transcript_fts
		JOIN transcript_library l ON l.transcript_id = transcript_fts.rowid
		WHERE transcript_fts MATCH 'idolatry'
		ORDER BY bm25(transcript_fts)`)
	if err != nil {
		t.Fatalf("FTS search query: %v", err)
	}
	defer rows.Close()
	var count int
	for rows.Next() {
		count++
		var id int64
		var channel, title, excerpt string
		if err := rows.Scan(&id, &channel, &title, &excerpt); err != nil {
			t.Fatalf("scan FTS search: %v", err)
		}
		if id != manualID || channel != "Search Channel" || title != "Searchable Lecture" {
			t.Errorf("search row = id %d, channel %q, title %q; want preferred manual row", id, channel, title)
		}
		if !strings.Contains(excerpt, "[idolatry]") {
			t.Errorf("snippet = %q, want highlighted useful match", excerpt)
		}
	}
	if count != 1 {
		t.Errorf("joined preferred search returned %d rows, want 1", count)
	}
}

func assertFTSRow(t *testing.T, db *sql.DB, id int64, wantChannel, wantTitle, wantText string) {
	t.Helper()
	var transcriptID int64
	var channel, title, text string
	if err := db.QueryRow(`SELECT transcript_id, channel, title, text FROM transcript_fts WHERE rowid = ?`, id).
		Scan(&transcriptID, &channel, &title, &text); err != nil {
		t.Fatalf("select FTS row %d: %v", id, err)
	}
	if transcriptID != id || channel != wantChannel || title != wantTitle || text != wantText {
		t.Errorf("FTS row = (%d, %q, %q, %q), want (%d, %q, %q, %q)",
			transcriptID, channel, title, text, id, wantChannel, wantTitle, wantText)
	}
}

func assertFTSMatchCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transcript_fts WHERE transcript_fts MATCH ?`, query).Scan(&got); err != nil {
		t.Fatalf("FTS MATCH %q: %v", query, err)
	}
	if got != want {
		t.Errorf("FTS MATCH %q count = %d, want %d", query, got, want)
	}
}
