package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

func TestWebFetchExcerptReadsRelevantSectionAndChecksContinuation(t *testing.T) {
	text := strings.Repeat("irrelevant introduction\n", 6000) + "TARGET SECTION\n" + strings.Repeat("evidence 🐈\n", 3000)
	var body atomic.Value
	body.Store(text)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	defer server.Close()
	call := func(args map[string]any) (map[string]any, error) {
		b, _ := json.Marshal(args)
		raw, err := WebExcerptTool().Execute(context.Background(), string(b))
		if err != nil {
			return nil, err
		}
		var result map[string]any
		err = json.Unmarshal([]byte(raw), &result)
		return result, err
	}
	first, err := call(map[string]any{"url": server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if first["end"] != float64(16384) || first["total_bytes"] != float64(len(text)) || first["next_offset"] != float64(16384) || strings.Contains(first["content"].(string), "full text saved") {
		t.Fatalf("default excerpt metadata=%v", first)
	}
	section, err := call(map[string]any{"url": server.URL, "query": "TARGET SECTION", "max_bytes": 128})
	if err != nil {
		t.Fatal(err)
	}
	if section["start"] != float64(strings.Index(text, "TARGET SECTION")) || !strings.Contains(section["content"].(string), "TARGET SECTION") || !strings.Contains(section["content"].(string), "untrusted web content") {
		t.Fatalf("section=%v", section)
	}
	_, err = call(map[string]any{"url": server.URL, "offset": section["next_offset"], "expected_sha256": section["sha256"], "max_bytes": 128})
	if err != nil {
		t.Fatal(err)
	}
	body.Store(text + "changed")
	if _, err := call(map[string]any{"url": server.URL, "offset": section["next_offset"], "expected_sha256": section["sha256"]}); err == nil {
		t.Fatal("changed document continuation succeeded")
	}
}

func TestWebFetchExcerptPreservesUTF8AndBoundsEscapedEnvelope(t *testing.T) {
	for _, source := range []string{strings.Repeat("🐈é", 30), strings.Repeat("\x00<\"", 32_768)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(source))
		}))
		func() {
			defer server.Close()
			offset, hash := 0, ""
			var joined strings.Builder
			for offset < len(source) {
				limit := 32_768
				if len(source) < 1000 {
					limit = 5
				}
				args, _ := json.Marshal(map[string]any{"url": server.URL, "offset": offset, "expected_sha256": hash, "max_bytes": limit})
				raw, err := WebExcerptTool().Execute(context.Background(), string(args))
				if err != nil {
					t.Fatal(err)
				}
				var result webExcerptResult
				if err := json.Unmarshal([]byte(raw), &result); err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(raw)
				if len(encoded) > 64*1024 {
					t.Fatalf("escaped excerpt exceeds safe tool envelope: %d", len(encoded))
				}
				if result.Start != offset || result.End <= offset || !utf8.ValidString(source[result.Start:result.End]) {
					t.Fatal("excerpt did not make contiguous UTF-8 progress")
				}
				joined.WriteString(source[result.Start:result.End])
				offset, hash = result.End, result.SHA256
				if offset < len(source) && (result.NextOffset == nil || *result.NextOffset != offset) {
					t.Fatal("missing continuation")
				}
			}
			if joined.String() != source {
				t.Fatal("excerpt continuation lost content")
			}
		}()
	}
}

func TestWebFetchExcerptValidatesBeforeNetworkAndBoundsMissingQuery(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("known document"))
	}))
	defer server.Close()
	for _, patch := range []map[string]any{{"offset": -1}, {"offset": 1}, {"max_bytes": 32769}, {"max_bytes": 3}, {"query": strings.Repeat("x", 257)}, {"unknown": true}, {"expected_sha256": "bad"}} {
		patch["url"] = server.URL
		args, _ := json.Marshal(patch)
		if _, err := WebExcerptTool().Execute(context.Background(), string(args)); err == nil {
			t.Fatalf("invalid args accepted: %v", patch)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid arguments contacted source")
	}
	args, _ := json.Marshal(map[string]any{"url": server.URL, "query": "missing"})
	raw, err := WebExcerptTool().Execute(context.Background(), string(args))
	if err != nil {
		t.Fatal(err)
	}
	var result webExcerptResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.QueryFound == nil || *result.QueryFound || result.NextOffset != nil || strings.Contains(result.Content, "known document") {
		t.Fatalf("missing query dumped content: %+v", result)
	}
	args, _ = json.Marshal(map[string]any{"url": server.URL + "/" + strings.Repeat("x", 70*1024), "query": "missing"})
	if raw, err := WebExcerptTool().Execute(context.Background(), string(args)); err == nil {
		t.Fatalf("oversized missing-query envelope accepted: %d bytes", len(raw))
	}
}

func TestWebFetchExcerptBoundsCrossHostRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://elsewhere.example/"+strings.Repeat("x", 70*1024))
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	args, _ := json.Marshal(map[string]any{"url": server.URL})
	if result, err := WebExcerptTool().Execute(context.Background(), string(args)); err == nil {
		t.Fatalf("oversized redirect was admitted: %d bytes", len(result))
	}
}
