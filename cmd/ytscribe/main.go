// Command ytscribe is the CLI frontend for Evie's YouTube transcript library.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/davidadel66/evie/internal/youtube"
)

type ytscribeService interface {
	Fetch(context.Context, string, string, bool) (youtube.FetchResult, error)
	Scrape(context.Context, string, youtube.ScrapeOptions) (youtube.ScrapeResult, error)
}

var (
	openYTScribeDB     = youtube.OpenDB
	newYTScribeService = func(db *sql.DB) ytscribeService {
		return youtube.NewService(db, youtube.NewClient(nil))
	}
	importYTScribeLegacy = youtube.ImportLegacy
)

func main() {
	os.Exit(runYTScribe(os.Args[1:], os.Stdout, os.Stderr))
}

func runYTScribe(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "help":
		if len(args) != 1 {
			printUsage(stderr)
			return 2
		}
		printUsage(stdout)
		return 0
	case "fetch":
		return runFetch(args[1:], stdout, stderr)
	case "scrape":
		return runScrape(args[1:], stdout, stderr)
	case "import":
		return runImport(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ytscribe: unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runFetch(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("fetch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	language := flags.String("language", "en", "caption language")
	refresh := flags.Bool("refresh", false, "bypass cached results")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		printUsage(stderr)
		return 2
	}

	db, err := openYTScribeDB()
	if err != nil {
		fmt.Fprintf(stderr, "open transcript database: %v\n", err)
		return 1
	}
	defer db.Close()

	result, err := newYTScribeService(db).Fetch(context.Background(), flags.Arg(0), *language, *refresh)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	renderFetch(stdout, result)
	return 0
}

func runScrape(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("scrape", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	language := flags.String("language", "en", "caption language")
	limit := flags.Int("limit", 0, "newest videos to examine (0 = all)")
	delay := flags.Duration("delay", 1500*time.Millisecond, "delay between network attempts")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 || *limit < 0 || *delay < 0 {
		printUsage(stderr)
		return 2
	}

	db, err := openYTScribeDB()
	if err != nil {
		fmt.Fprintf(stderr, "open transcript database: %v\n", err)
		return 1
	}
	defer db.Close()

	progress := func(event youtube.ScrapeEvent) {
		fmt.Fprintf(stdout, "[%d/%d] %s | %s | %s", event.Index, event.Total, event.VideoID, event.Title, event.Status)
		if event.Err != nil {
			fmt.Fprintf(stdout, ": %v", event.Err)
		}
		fmt.Fprintln(stdout)
	}
	result, err := newYTScribeService(db).Scrape(context.Background(), flags.Arg(0), youtube.ScrapeOptions{
		Language: *language,
		Limit:    *limit,
		Delay:    *delay,
		Progress: progress,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	renderScrapeSummary(stdout, result)
	if result.Failed > 0 {
		return 1
	}
	return 0
}

func runImport(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	language := flags.String("language", "en", "transcript language")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		printUsage(stderr)
		return 2
	}

	db, err := openYTScribeDB()
	if err != nil {
		fmt.Fprintf(stderr, "open transcript database: %v\n", err)
		return 1
	}
	defer db.Close()

	progress := func(event youtube.ImportEvent) {
		fmt.Fprintf(stdout, "%s | %s", event.Path, event.Status)
		if event.Err != nil {
			fmt.Fprintf(stdout, ": %v", event.Err)
		}
		fmt.Fprintln(stdout)
	}
	result, err := importYTScribeLegacy(context.Background(), db, flags.Arg(0), *language, progress)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "Summary: seen %d, inserted %d, updated %d, skipped %d, matched %d, unmatched %d, failed %d, warnings %d\n",
		result.Seen, result.Inserted, result.Updated, result.Skipped, result.Matched, result.Unmatched, result.Failed, len(result.Warnings))
	if result.Failed > 0 {
		return 1
	}
	return 0
}

func renderFetch(w io.Writer, result youtube.FetchResult) {
	source := "network"
	if result.Cached {
		source = "cache"
	}
	fmt.Fprintf(w, "Source: %s\nVideo: %s | %s\nURL: %s\nChannel: %s | %s | %s\nChannel URL: %s\nPublished: %s\nDuration seconds: %d\nLanguage: %s | %s\nTranscript source: %s\nWords: %d\nRetrieved: %s\n\n%s",
		source, result.VideoID, result.Title, result.VideoURL, result.ChannelID, result.ChannelName,
		result.ChannelHandle, result.ChannelURL, result.PublishedAt, result.DurationSeconds,
		result.LanguageCode, result.LanguageName, result.TranscriptSource, result.WordCount,
		result.RetrievedAt, result.Text)
	if len(result.Text) == 0 || result.Text[len(result.Text)-1] != '\n' {
		fmt.Fprintln(w)
	}
}

func renderScrapeSummary(w io.Writer, result youtube.ScrapeResult) {
	fmt.Fprintf(w, "Channel: %s | %s | %s\nURL: %s\nSummary: discovered %d, cached %d, terminal-skipped %d, saved %d, failed %d\n",
		result.ChannelID, result.ChannelName, result.ChannelHandle, result.ChannelURL,
		result.Discovered, result.Cached, result.TerminalSkipped, result.Saved, result.Failed)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: ytscribe <command>

commands:
  ytscribe fetch [--language en] [--refresh] <video>
  ytscribe scrape [--language en] [--limit N] [--delay 1.5s] <channel>
  ytscribe import [--language en] <root>
  ytscribe help`)
}
