package main

// Stage 4 CLI acceptance tests derived only from
// cmd/evie/docs/active/youtube-transcripts.spec.md. runYTScribe is the narrow
// injectable CLI seam expected here: main should pass os.Args[1:], stdout and
// stderr to it, while tests replace only domain operations—not the CLI under
// test and never the network.

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/youtube"
)

type fakeCLIService struct {
	fetchResult youtube.FetchResult
	fetchErr    error
	scrape      youtube.ScrapeResult
	scrapeErr   error
	fetchInput  string
	fetchLang   string
	refresh     bool
	scrapeInput string
	scrapeOpts  youtube.ScrapeOptions
}

func (f *fakeCLIService) Fetch(_ context.Context, input, language string, refresh bool) (youtube.FetchResult, error) {
	f.fetchInput, f.fetchLang, f.refresh = input, language, refresh
	return f.fetchResult, f.fetchErr
}

func (f *fakeCLIService) Scrape(_ context.Context, input string, opts youtube.ScrapeOptions) (youtube.ScrapeResult, error) {
	f.scrapeInput, f.scrapeOpts = input, opts
	if opts.Progress != nil {
		opts.Progress(youtube.ScrapeEvent{VideoID: "AAAAAAAAAAA", Title: "cached video", Index: 1, Total: 3, Status: "cached"})
		opts.Progress(youtube.ScrapeEvent{VideoID: "BBBBBBBBBBB", Title: "saved video", Index: 2, Total: 3, Status: "saved"})
		opts.Progress(youtube.ScrapeEvent{VideoID: "CCCCCCCCCCC", Title: "failed video", Index: 3, Total: 3, Status: "failed", Err: errors.New("HTTP 429")})
	}
	return f.scrape, f.scrapeErr
}

type fakeCLIImporter struct {
	result   youtube.ImportResult
	err      error
	root     string
	language string
}

func (f *fakeCLIImporter) Import(_ context.Context, _ *sql.DB, root, language string, progress func(youtube.ImportEvent)) (youtube.ImportResult, error) {
	f.root, f.language = root, language
	if progress != nil {
		progress(youtube.ImportEvent{Path: filepath.Join(root, "a", "one.txt"), Status: "inserted"})
		progress(youtube.ImportEvent{Path: filepath.Join(root, "b"), Status: "warning", Err: errors.New("malformed manifest")})
		progress(youtube.ImportEvent{Path: filepath.Join(root, "c", "bad.txt"), Status: "failed", Err: errors.New("invalid UTF-8")})
	}
	return f.result, f.err
}

func installCLISeams(t *testing.T, service *fakeCLIService, importer *fakeCLIImporter) {
	t.Helper()
	db, err := youtube.OpenDBAt(filepath.Join(t.TempDir(), "transcripts.db"))
	if err != nil {
		t.Fatalf("OpenDBAt: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	originalOpen := openYTScribeDB
	originalNew := newYTScribeService
	originalImport := importYTScribeLegacy
	openYTScribeDB = func() (*sql.DB, error) { return db, nil }
	newYTScribeService = func(*sql.DB) ytscribeService { return service }
	importYTScribeLegacy = importer.Import
	t.Cleanup(func() {
		openYTScribeDB = originalOpen
		newYTScribeService = originalNew
		importYTScribeLegacy = originalImport
	})
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runYTScribe(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestYTScribeUsageAndHelp(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no command", nil},
		{"unknown command", []string{"unknown"}},
		{"fetch missing video", []string{"fetch"}},
		{"fetch extra positional", []string{"fetch", "AAAAAAAAAAA", "extra"}},
		{"scrape missing channel", []string{"scrape"}},
		{"import missing root", []string{"import"}},
		{"fetch unknown flag", []string{"fetch", "--bogus", "AAAAAAAAAAA"}},
		{"scrape invalid limit", []string{"scrape", "--limit", "not-a-number", "@channel"}},
		{"scrape negative limit", []string{"scrape", "--limit", "-1", "@channel"}},
		{"scrape negative delay", []string{"scrape", "--delay", "-1ms", "@channel"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tc.args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2 (stdout %q stderr %q)", code, stdout, stderr)
			}
			if !strings.Contains(strings.ToLower(stderr), "usage:") {
				t.Errorf("stderr = %q, want usage", stderr)
			}
		})
	}

	code, stdout, stderr := runCLI(t, "help")
	if code != 0 {
		t.Errorf("help exit = %d, want 0", code)
	}
	help := stdout + stderr
	for _, want := range []string{"ytscribe fetch", "ytscribe scrape", "ytscribe import", "--refresh", "--delay"} {
		if !strings.Contains(help, want) {
			t.Errorf("help output does not contain %q:\n%s", want, help)
		}
	}
}

func TestYTScribeFetchFlagsRenderingAndExitStatuses(t *testing.T) {
	service := &fakeCLIService{fetchResult: youtube.FetchResult{
		Cached: false, VideoID: "AAAAAAAAAAA", Title: "A useful talk",
		VideoURL:  "https://www.youtube.com/watch?v=AAAAAAAAAAA",
		ChannelID: "UCssssssssssssssssssssss", ChannelName: "Service Channel",
		ChannelHandle: "@ServiceChannel", LanguageCode: "en-us",
		LanguageName: "English (United States)", TranscriptSource: "manual",
		Text: strings.Repeat("full transcript line\n", 7000),
	}}
	installCLISeams(t, service, &fakeCLIImporter{})

	code, stdout, stderr := runCLI(t, "fetch", "--language", "fr-CA", "--refresh", "AAAAAAAAAAA")
	if code != 0 || stderr != "" {
		t.Fatalf("fetch = exit %d stdout bytes %d stderr %q", code, len(stdout), stderr)
	}
	if service.fetchInput != "AAAAAAAAAAA" || (service.fetchLang != "fr-CA" && service.fetchLang != "fr-ca") || !service.refresh {
		t.Errorf("Fetch args = %q %q %t", service.fetchInput, service.fetchLang, service.refresh)
	}
	if !strings.Contains(stdout, "A useful talk") || !strings.Contains(stdout, "Service Channel") || !strings.Contains(stdout, "en-us") || !strings.Contains(stdout, "manual") {
		t.Errorf("fetch output is missing promised metadata")
	}
	if strings.Count(stdout, "full transcript line") != 7000 {
		t.Errorf("CLI capped full transcript: got %d lines", strings.Count(stdout, "full transcript line"))
	}
	if strings.Contains(strings.ToLower(stdout), "untrusted") || strings.Contains(strings.ToLower(stdout), "spill") {
		t.Errorf("CLI inherited agent-only framing/cap behavior")
	}

	service.fetchErr = &youtube.TerminalError{Kind: "no_captions", Detail: "captions disabled", Cached: true}
	code, stdout, stderr = runCLI(t, "fetch", "AAAAAAAAAAA")
	if code != 1 || stdout != "" {
		t.Errorf("terminal fetch = exit %d stdout %q, want 1/empty", code, stdout)
	}
	if !strings.Contains(stderr, "no_captions") || !strings.Contains(stderr, "captions disabled") {
		t.Errorf("terminal stderr lost kind/detail: %q", stderr)
	}

	service.fetchErr = errors.New("database is locked")
	code, _, stderr = runCLI(t, "fetch", "AAAAAAAAAAA")
	if code != 1 || !strings.Contains(stderr, "database is locked") {
		t.Errorf("fatal fetch = exit %d stderr %q", code, stderr)
	}
}

func TestYTScribeScrapeFlagsProgressSummaryAndPartialFailureExit(t *testing.T) {
	service := &fakeCLIService{scrape: youtube.ScrapeResult{
		ChannelID: "UCssssssssssssssssssssss", ChannelName: "Service Channel",
		Discovered: 3, Cached: 1, Saved: 1, Failed: 1,
		Failures: []youtube.ScrapeFailure{{VideoID: "CCCCCCCCCCC", Title: "failed video", Err: errors.New("HTTP 429")}},
	}}
	installCLISeams(t, service, &fakeCLIImporter{})

	code, stdout, stderr := runCLI(t, "scrape", "--language", "es", "--limit", "0", "--delay", "250ms", "@ServiceChannel")
	if code != 1 {
		t.Errorf("partial scrape = exit %d, want 1", code)
	}
	rendered := stdout + stderr
	if service.scrapeInput != "@ServiceChannel" || service.scrapeOpts.Language != "es" || service.scrapeOpts.Limit != 0 || service.scrapeOpts.Delay != 250*time.Millisecond {
		t.Errorf("Scrape opts = input %q language %q limit %d delay %s",
			service.scrapeInput, service.scrapeOpts.Language, service.scrapeOpts.Limit, service.scrapeOpts.Delay)
	}
	for _, want := range []string{
		"AAAAAAAAAAA", "cached video", "cached", "BBBBBBBBBBB", "saved video", "saved",
		"CCCCCCCCCCC", "failed video", "HTTP 429", "discovered", "3", "failed", "1",
	} {
		if !strings.Contains(strings.ToLower(rendered), strings.ToLower(want)) {
			t.Errorf("scrape output does not contain %q:\n%s", want, rendered)
		}
	}

	service.scrape.Failed = 0
	service.scrape.Failures = nil
	code, _, _ = runCLI(t, "scrape", "--limit", "2", "@ServiceChannel")
	if code != 0 || service.scrapeOpts.Limit != 2 || service.scrapeOpts.Language != "en" || service.scrapeOpts.Delay != 1500*time.Millisecond {
		t.Errorf("successful/default scrape = exit %d opts %+v", code, service.scrapeOpts)
	}

	service.scrapeErr = errors.New("channel listing format drift")
	code, _, stderr = runCLI(t, "scrape", "@ServiceChannel")
	if code != 1 || !strings.Contains(stderr, "format drift") {
		t.Errorf("fatal scrape = exit %d stderr %q", code, stderr)
	}
}

func TestYTScribeImportFlagsProgressWarningsSummaryAndExit(t *testing.T) {
	root := t.TempDir()
	importer := &fakeCLIImporter{result: youtube.ImportResult{
		Seen: 3, Inserted: 1, Skipped: 1, Matched: 1, Unmatched: 1, Failed: 1,
		Warnings: []youtube.ImportWarning{{Collection: filepath.Join(root, "b"), Err: errors.New("malformed manifest")}},
		Failures: []youtube.ImportFailure{{Path: filepath.Join(root, "c", "bad.txt"), Err: errors.New("invalid UTF-8")}},
	}}
	installCLISeams(t, &fakeCLIService{}, importer)

	code, stdout, stderr := runCLI(t, "import", "--language", "de", root)
	if code != 1 {
		t.Errorf("partial import = exit %d, want 1", code)
	}
	rendered := stdout + stderr
	if importer.root != root || importer.language != "de" {
		t.Errorf("Import args = root %q language %q", importer.root, importer.language)
	}
	for _, want := range []string{
		"one.txt", "inserted", "warning", "malformed manifest", "bad.txt", "invalid UTF-8",
		"seen", "3", "matched", "1", "unmatched", "1", "failed", "1",
	} {
		if !strings.Contains(strings.ToLower(rendered), strings.ToLower(want)) {
			t.Errorf("import output does not contain %q:\n%s", want, rendered)
		}
	}

	// Collection warnings alone are explicitly non-fatal.
	importer.result.Failed = 0
	importer.result.Failures = nil
	code, _, _ = runCLI(t, "import", root)
	if code != 0 || importer.language != "en" {
		t.Errorf("warning-only/default-language import = exit %d language %q, want 0/en", code, importer.language)
	}

	importer.err = os.ErrPermission
	code, _, stderr = runCLI(t, "import", root)
	if code != 1 || !strings.Contains(strings.ToLower(stderr), "permission") {
		t.Errorf("fatal import = exit %d stderr %q", code, stderr)
	}
}

func TestYTScribeOpenDatabaseFailureExitsOne(t *testing.T) {
	original := openYTScribeDB
	openYTScribeDB = func() (*sql.DB, error) { return nil, errors.New("cannot open transcript database") }
	t.Cleanup(func() { openYTScribeDB = original })

	for _, args := range [][]string{{"fetch", "AAAAAAAAAAA"}, {"scrape", "@ServiceChannel"}, {"import", t.TempDir()}} {
		code, _, stderr := runCLI(t, args...)
		if code != 1 || !strings.Contains(stderr, "cannot open transcript database") {
			t.Errorf("%v = exit %d stderr %q", args, code, stderr)
		}
	}
}
