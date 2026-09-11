package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// Opt-in development measurement through the same complete turn used by the
// acceptance tests. Configuration is frozen in the separately hashed test
// executable; this is not a held-out reader quality evaluation.
func TestMemoryGraphBoundsExperiment(t *testing.T) {
	directory := os.Getenv("EVIE_GRAPH_REPORT_DIR")
	if directory == "" {
		t.Skip("set EVIE_GRAPH_REPORT_DIR to run the local graph bounds experiment")
	}
	type sample struct {
		Milliseconds float64 `json:"whole_turn_ms"`
		RequestBytes int     `json:"complete_request_bytes"`
		MemoryBytes  int     `json:"memory_data_bytes"`
		Delivered    int     `json:"delivered_claims"`
		Truncated    bool    `json:"search_truncated"`
	}
	type profile struct {
		Fanout       int                             `json:"fanout"`
		Claims       []memory.RememberEntityProposal `json:"accepted_fixture"`
		Samples      []sample                        `json:"samples"`
		P50          float64                         `json:"p50_whole_turn_ms"`
		P95          float64                         `json:"p95_whole_turn_ms"`
		MaxRequest   int                             `json:"max_complete_request_bytes"`
		MaxMemory    int                             `json:"max_memory_data_bytes"`
		MaxDelivered int                             `json:"max_delivered_claims"`
	}
	var profiles []profile
	for _, width := range []int{1, 4, 12, 24} {
		f := newRetrievalFixture(t)
		source := f.global()
		bridge := f.rememberGraphEntity(source, "Maya's sister is Nora.", memory.RememberEntityRequest{
			Predicate: "sister", PredicateLabel: "sister",
			Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya"},
			Object:  memory.EntitySelector{Create: true, CanonicalName: "Nora", EntityType: "person", Alias: "Nora"},
		})
		p := profile{Fanout: width, Claims: []memory.RememberEntityProposal{bridge}}
		for i := 0; i < width; i++ {
			name := fmt.Sprintf("pastry %02d", i)
			p.Claims = append(p.Claims, f.rememberGraphEntity(source, "Nora prefers "+name+".", memory.RememberEntityRequest{
				Predicate: "prefers", PredicateLabel: "prefers", Subject: memory.EntitySelector{EntityID: bridge.Claim.ObjectEntityID},
				Object: memory.EntitySelector{Create: true, CanonicalName: name, EntityType: "food", Alias: name},
			}))
		}
		f.refresh()
		for n := -3; n < 30; n++ {
			reader := f.global()
			started := time.Now()
			client, _ := f.search(reader, "Maya")
			elapsed := time.Since(started)
			request := client.reqs[len(client.reqs)-1]
			evidence := expansionBoundaryEvidence(t, request)
			if len(evidence) < 2 || len(evidence) > 8 || evidence[0].ClaimID != bridge.Claim.ID {
				t.Fatalf("fanout %d did not deliver bounded source-supported relationship evidence: %+v", width, evidence)
			}
			for _, item := range evidence[1:] {
				if len(item.GraphPaths) != 1 || len(item.GraphPaths[0].ClaimIDs) != 2 || item.GraphPaths[0].ClaimIDs[0] != bridge.Claim.ID || len(item.Sources) != 1 || item.Sources[0].Evidence == "" {
					t.Fatalf("fanout %d delivered an unsupported path: %+v", width, item)
				}
			}
			var outcome struct {
				Truncated bool `json:"truncated"`
			}
			outcomeText := expansionBoundaryToolResult(t, request, "lookup")
			parts := strings.Split(outcomeText, "\n")
			if len(parts) != 3 {
				t.Fatalf("unexpected bounded retrieval outcome: %s", outcomeText)
			}
			if err := json.Unmarshal([]byte(parts[1]), &outcome); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			if n < 0 {
				continue
			}
			dataBytes := len(retrievalData(t, request))
			p.Samples = append(p.Samples, sample{Milliseconds: float64(elapsed.Nanoseconds()) / 1e6, RequestBytes: len(encoded), MemoryBytes: dataBytes, Delivered: len(evidence), Truncated: outcome.Truncated})
			p.MaxRequest = max(p.MaxRequest, len(encoded))
			p.MaxMemory = max(p.MaxMemory, dataBytes)
			p.MaxDelivered = max(p.MaxDelivered, len(evidence))
		}
		var times []float64
		for _, s := range p.Samples {
			times = append(times, s.Milliseconds)
		}
		slices.Sort(times)
		p.P50, p.P95 = times[14], times[28]
		profiles = append(profiles, p)
	}
	report := struct {
		RecordedAt string    `json:"recorded_at"`
		GoVersion  string    `json:"go_version"`
		Platform   string    `json:"platform"`
		Profiles   []profile `json:"profiles"`
	}{time.Now().UTC().Format(time.RFC3339Nano), runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH, profiles}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "report.json"), append(encoded, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	for _, p := range profiles {
		t.Logf("fanout=%d p50=%.3fms p95=%.3fms request=%dB memory=%dB claims=%d", p.Fanout, p.P50, p.P95, p.MaxRequest, p.MaxMemory, p.MaxDelivered)
	}
}
