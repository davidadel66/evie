package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

const (
	toolResultAdmissionBytes         = 100 * 1024
	toolResultGroupBytes             = 128 * 1024
	toolResultGroupMinimumBytes      = 8 * 1024
	toolResultProjectionThreshold    = 4 * 1024
	toolResultProjectionExcerpt      = 1024
	retainedCompleteToolResultGroups = 3
)

func admitToolResult(content string) string {
	if len(content) <= toolResultAdmissionBytes {
		return content
	}
	digest := sha256.Sum256([]byte(content))
	label := fmt.Sprintf(
		"[tool result bounded: original_bytes=%d sha256=%x]",
		len(content),
		digest,
	)
	return headTailPreview(content, toolResultAdmissionBytes, label)
}

func headTailPreview(content string, limit int, label string) string {
	if len(content) <= limit {
		return content
	}
	header := label + "\n<head>\n"
	middle := "\n<tail>\n"
	available := limit - len(header) - len(middle)
	if available <= 0 {
		return validUTF8Prefix(label, limit)
	}
	headBudget := (available + 1) / 2
	tailBudget := available - headBudget
	head := validUTF8Prefix(content, headBudget)
	tail := validUTF8Suffix(content, tailBudget)
	padding := strings.Repeat(".", available-len(head)-len(tail))
	return header + head + padding + middle + tail
}

func validUTF8Prefix(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}

func validUTF8Suffix(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	start := len(value) - limit
	for start < len(value) && !utf8.ValidString(value[start:]) {
		start++
	}
	return value[start:]
}

func applyToolResultGroupLimits(events []memory.Event) ([]memory.Event, error) {
	groups, err := completeToolResultGroups(events)
	if err != nil {
		return nil, err
	}
	projected := append([]memory.Event(nil), events...)
	for _, group := range groups {
		sizes := make([]int, len(group.resultIndexes))
		ordinaryFloors := make([]int, len(group.resultIndexes))
		metadataFloors := make([]int, len(group.resultIndexes))
		for i, index := range group.resultIndexes {
			sizes[i] = min(len(events[index].Content), toolResultAdmissionBytes)
			ordinaryFloors[i] = min(sizes[i], toolResultGroupMinimumBytes)
			metadataFloors[i] = min(sizes[i], len(toolResultGroupPreviewLabel(events[index]))+len("\n<head>\n\n<tail>\n"))
		}
		floors := ordinaryFloors
		if sumInts(floors) > toolResultGroupBytes {
			floors = metadataFloors
		}
		if sumInts(floors) > toolResultGroupBytes {
			return nil, fmt.Errorf("%w: tool-result group metadata exceeds %d bytes", ErrContextOverflow, toolResultGroupBytes)
		}
		allocations := largestFirstAllocationsWithFloors(sizes, toolResultGroupBytes, floors)
		for i, index := range group.resultIndexes {
			if allocations[i] >= len(events[index].Content) {
				continue
			}
			label := toolResultGroupPreviewLabel(events[index])
			projected[index].Content = headTailPreview(events[index].Content, allocations[i], label)
		}
	}
	return projected, nil
}

func largestFirstAllocations(sizes []int, budget, minimum int) []int {
	floors := make([]int, len(sizes))
	for i, size := range sizes {
		floors[i] = min(size, minimum)
	}
	if sumInts(floors) > budget {
		clear(floors)
	}
	return largestFirstAllocationsWithFloors(sizes, budget, floors)
}

func largestFirstAllocationsWithFloors(sizes []int, budget int, floors []int) []int {
	allocations := append([]int(nil), sizes...)
	if sumInts(allocations) <= budget {
		return allocations
	}
	maxSize := 0
	for _, size := range sizes {
		maxSize = max(maxSize, size)
	}
	low, high := 0, maxSize
	for low < high {
		candidate := low + (high-low+1)/2
		if cappedAllocationTotal(sizes, floors, candidate) <= budget {
			low = candidate
		} else {
			high = candidate - 1
		}
	}
	for i, size := range sizes {
		allocations[i] = min(size, max(floors[i], low))
	}
	remaining := budget - sumInts(allocations)
	for i := range allocations {
		if remaining == 0 {
			break
		}
		if allocations[i] < sizes[i] {
			allocations[i]++
			remaining--
		}
	}
	return allocations
}

func toolResultGroupPreviewLabel(event memory.Event) string {
	digest := sha256.Sum256([]byte(event.Content))
	return fmt.Sprintf(
		"[tool result group-bounded: event_id=%s original_bytes=%d sha256=%x]",
		event.ID, len(event.Content), digest,
	)
}

func cappedAllocationTotal(sizes, floors []int, cap int) int {
	total := 0
	for i, size := range sizes {
		total += min(size, max(floors[i], cap))
	}
	return total
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func projectOldToolResult(event memory.Event) string {
	digest := sha256.Sum256([]byte(event.Content))
	label := fmt.Sprintf(
		"[older tool result projected: event_id=%s original_bytes=%d sha256=%x]",
		event.ID, len(event.Content), digest,
	)
	header := label + "\n<head>\n"
	middle := "\n<tail>\n"
	return header + validUTF8Prefix(event.Content, toolResultProjectionExcerpt/2) + middle +
		validUTF8Suffix(event.Content, toolResultProjectionExcerpt/2)
}

func isPressureProjectableToolResult(event memory.Event) bool {
	return len(event.Content) > toolResultProjectionThreshold
}

func toolResultPlaceholderManifests(
	original, projected []memory.Event,
) []memory.ContextPlaceholderManifest {
	var manifests []memory.ContextPlaceholderManifest
	for i := range original {
		if original[i].Content == projected[i].Content ||
			(original[i].Type != memory.EventToolSucceeded && original[i].Type != memory.EventToolFailed &&
				original[i].Type != memory.EventToolCancelled) {
			continue
		}
		digest := sha256.Sum256([]byte(original[i].Content))
		manifests = append(manifests, memory.ContextPlaceholderManifest{
			EventID: original[i].ID, OriginalBytes: int64(len(original[i].Content)),
			ProjectedBytes: int64(len(projected[i].Content)), SHA256: hex.EncodeToString(digest[:]),
		})
	}
	return manifests
}
