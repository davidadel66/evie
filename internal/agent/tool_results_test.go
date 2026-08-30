package agent

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

func TestAdmitToolResultExactThresholdAndUTF8Boundary(t *testing.T) {
	for _, test := range []struct {
		name        string
		content     string
		wantChanged bool
	}{
		{name: "exact threshold", content: strings.Repeat("x", toolResultAdmissionBytes)},
		{name: "one byte above", content: strings.Repeat("x", toolResultAdmissionBytes+1), wantChanged: true},
		{name: "multi-byte boundary", content: strings.Repeat("🙂", toolResultAdmissionBytes/4+1), wantChanged: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := admitToolResult(test.content)
			if len(got) > toolResultAdmissionBytes || !utf8.ValidString(got) {
				t.Fatalf("bytes=%d valid_utf8=%v", len(got), utf8.ValidString(got))
			}
			if (got != test.content) != test.wantChanged {
				t.Fatalf("changed=%v, want %v", got != test.content, test.wantChanged)
			}
		})
	}
}

func TestLargestFirstAllocationsPreserveMinimumUntilImpossible(t *testing.T) {
	for _, test := range []struct {
		name  string
		sizes []int
		want  []int
	}{
		{name: "exact group threshold", sizes: []int{100 * 1024, 28 * 1024}, want: []int{100 * 1024, 28 * 1024}},
		{name: "largest compete while small stays full", sizes: []int{90 * 1024, 70 * 1024, 10 * 1024}, want: []int{59 * 1024, 59 * 1024, 10 * 1024}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := largestFirstAllocations(test.sizes, toolResultGroupBytes, toolResultGroupMinimumBytes)
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("allocations=%v want=%v", got, test.want)
			}
		})
	}

	possible := largestFirstAllocations([]int{120 * 1024, 16 * 1024}, toolResultGroupBytes, toolResultGroupMinimumBytes)
	for i, allocation := range possible {
		if allocation < toolResultGroupMinimumBytes {
			t.Fatalf("possible allocation %d=%d below minimum", i, allocation)
		}
	}
	impossible := largestFirstAllocations(make([]int, 17), toolResultGroupBytes, toolResultGroupMinimumBytes)
	for i := range impossible {
		if impossible[i] != 0 {
			t.Fatalf("zero-sized input allocation %d=%d", i, impossible[i])
		}
	}
	sizes := make([]int, 17)
	for i := range sizes {
		sizes[i] = 10 * 1024
	}
	impossible = largestFirstAllocations(sizes, toolResultGroupBytes, toolResultGroupMinimumBytes)
	if sumInts(impossible) != toolResultGroupBytes || impossible[0] >= toolResultGroupMinimumBytes {
		t.Fatalf("impossible minimum allocations=%v total=%d", impossible, sumInts(impossible))
	}
}

func TestOldToolResultProjectionHasSafeExcerptsIdentityAndHash(t *testing.T) {
	content := strings.Repeat("h", 2048) + strings.Repeat("m", 2048) + strings.Repeat("t", 2048)
	event := memory.Event{ID: "result-9", Type: memory.EventToolSucceeded, Content: content}
	projected := projectOldToolResult(event)
	digest := sha256.Sum256([]byte(content))
	for _, want := range []string{
		"event_id=result-9",
		"original_bytes=6144",
		fmt.Sprintf("sha256=%x", digest),
		"<head>\n" + strings.Repeat("h", 512),
		"<tail>\n" + strings.Repeat("t", 512),
	} {
		if !strings.Contains(projected, want) {
			t.Fatalf("projection omitted %q", want)
		}
	}
	if !utf8.ValidString(projected) {
		t.Fatal("projection is not valid UTF-8")
	}
	if isPressureProjectableToolResult(memory.Event{Content: strings.Repeat("x", 4*1024)}) ||
		!isPressureProjectableToolResult(memory.Event{Content: strings.Repeat("x", 4*1024+1)}) {
		t.Fatal("pressure projection threshold is not strict")
	}
}

func TestToolResultGroupRejectsWhenRequiredMetadataCannotFit(t *testing.T) {
	const resultCount = 1000
	results := make([]string, resultCount)
	for i := range results {
		results[i] = strings.Repeat("x", 10*1024)
	}
	_, err := applyToolResultGroupLimits(toolGroupEvents(t, "root", results))
	if !IsContextOverflow(err) {
		t.Fatalf("group limit error=%v, want context overflow", err)
	}
}

func TestPercentageFloorDoesNotOverflow(t *testing.T) {
	if got := percentageFloor(1<<63-1, 60); got != 5534023222112865484 {
		t.Fatalf("60%% of MaxInt64=%d", got)
	}
}
