package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

const (
	// toolResultAdmissionBytes bounds each durable tool result at write
	// time. The admitted value is what read_tool_result pages through.
	toolResultAdmissionBytes = 100 * 1024
	// toolResultGroupBytes bounds each complete group in compactor
	// transcripts, whose summary replaces the results it sees, and in
	// requests that do not offer read_tool_result.
	toolResultGroupBytes             = 128 * 1024
	toolResultGroupMinimumBytes      = 8 * 1024
	toolResultProjectionThreshold    = 4 * 1024
	toolResultProjectionExcerpt      = 1024
	retainedCompleteToolResultGroups = 3

	// modelToolResultBytes bounds what a request that offers read_tool_result
	// shows of one tool result (memory architecture Stage 9). A larger durable
	// result is shown as a labelled head and tail excerpt that names the
	// omitted byte range to page back. It sits above bash's 30,000-byte inline
	// cap with its spill note and equals read_tool_result's largest page.
	modelToolResultBytes = 32 * 1024
	// modelToolResultGroupBytes bounds what such a request shows of one
	// complete tool-result group: three full results, and exactly a
	// full delegate_research batch envelope (delegation.BatchEnvelopeBytes).
	modelToolResultGroupBytes = 96 * 1024
	// pressureProjectionClearAtLeastBytes is the fewest canonical request
	// bytes a pressure projection must free to be applied while the request
	// fits without it, like clear_at_least in Claude's context editing: a
	// projection rewrites history and invalidates the provider cache after
	// it, so a small saving is not worth it.
	pressureProjectionClearAtLeastBytes = 8 * 1024
)

// Excerpt reasons name the limit that shortened a model-visible result.
const (
	toolResultLimitReasonResult         = "result_limit"
	toolResultLimitReasonGroup          = "group_limit"
	toolResultLimitReasonProtectedGroup = "protected_group_limit"
)

// Protected tool results carry small, high-value state: memory, Task Tree,
// sub-agent results and reports, and pages already read back with
// read_tool_result. Conversation projection never shortens them, the
// equivalent of exclude_tools in Claude's tool-result clearing, unless they
// alone exceed a group's budget or the request cannot fit without them; then
// the normal rules apply and the projection says so.
var (
	protectedToolResultPrefixes = []string{"memory_", "todo_", "delegate_"}
	protectedToolResultNames    = map[string]bool{
		readToolResultToolName:      true,
		delegation.ToolName:         true,
		delegation.ExploreToolName:  true,
		delegation.ReportToolName:   true,
		delegation.ContinueToolName: true,
	}
)

func isProtectedToolResult(name string) bool {
	if protectedToolResultNames[name] {
		return true
	}
	for _, prefix := range protectedToolResultPrefixes {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
			return true
		}
	}
	return false
}

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

// readToolResultToolName is the tool that pages a stored tool result back.
const readToolResultToolName = "read_tool_result"

// offersToolResultPaging reports whether a request offers read_tool_result.
// Only then does it show excerpts under the Stage 9 limits, because only then
// can the model page back what an excerpt omits. Workers and sessions pinned
// to presets without it keep the 100 KiB admission and 128 KiB group bounds
// that their tools' own output envelopes are sized for.
func offersToolResultPaging(tools []openrouter.Tool) bool {
	for _, tool := range tools {
		if tool.Function.Name == readToolResultToolName {
			return true
		}
	}
	return false
}

// applyModelToolResultLimits bounds the tool results a request that offers
// read_tool_result shows. Each unprotected result shows at most modelToolResultBytes
// and each complete group at most modelToolResultGroupBytes, largest results
// reduced first. Protected results keep their full durable content while the
// group's other results can share what remains; when protected results alone
// leave no room, every result of the group follows the normal allocation and
// the excerpt names that reason. Every bound is a pure function of the
// durable group, so a result's bytes never change between iterations.
func applyModelToolResultLimits(events []memory.Event) ([]memory.Event, error) {
	groups, err := completeToolResultGroups(events)
	if err != nil {
		return nil, err
	}
	projected := append([]memory.Event(nil), events...)
	for _, group := range groups {
		count := len(group.resultIndexes)
		sizes := make([]int, count)
		protected := make([]bool, count)
		minimums := make([]int, count)
		for i, index := range group.resultIndexes {
			event := events[index]
			protected[i] = isProtectedToolResult(group.names[i])
			sizes[i] = len(event.Content)
			if !protected[i] {
				sizes[i] = min(sizes[i], modelToolResultBytes)
			}
			minimums[i] = min(sizes[i], toolResultExcerptOverhead(event, toolResultLimitReasonProtectedGroup))
		}
		allocations, err := modelGroupAllocations(sizes, protected, minimums)
		if err != nil {
			return nil, err
		}
		for i, index := range group.resultIndexes {
			event := events[index]
			if allocations[i] >= len(event.Content) {
				continue
			}
			reason := toolResultLimitReasonResult
			switch {
			case protected[i]:
				reason = toolResultLimitReasonProtectedGroup
			case allocations[i] < sizes[i]:
				reason = toolResultLimitReasonGroup
			}
			projected[index].Content = toolResultExcerpt(event, allocations[i], reason)
		}
	}
	return projected, nil
}

// modelGroupAllocations returns each result's visible byte budget within
// modelToolResultGroupBytes. Floors keep 8 KiB of each reduced result, or
// only its excerpt label when the group has too many results for that.
func modelGroupAllocations(sizes []int, protected []bool, minimums []int) ([]int, error) {
	budget := modelToolResultGroupBytes
	if sumInts(sizes) <= budget {
		return sizes, nil
	}
	floorsFor := func(sizes, minimums []int, budget int) ([]int, bool) {
		floors := make([]int, len(sizes))
		for i, size := range sizes {
			floors[i] = min(size, toolResultGroupMinimumBytes)
		}
		if sumInts(floors) <= budget {
			return floors, true
		}
		copy(floors, minimums)
		return floors, sumInts(floors) <= budget
	}
	protectedBytes := 0
	var others, otherMinimums []int
	for i, size := range sizes {
		if protected[i] {
			protectedBytes += size
		} else {
			others = append(others, size)
			otherMinimums = append(otherMinimums, minimums[i])
		}
	}
	if remaining := budget - protectedBytes; remaining >= 0 {
		if floors, ok := floorsFor(others, otherMinimums, remaining); ok {
			shared := largestFirstAllocationsWithFloors(others, remaining, floors)
			allocations := make([]int, len(sizes))
			next := 0
			for i, size := range sizes {
				if protected[i] {
					allocations[i] = size
				} else {
					allocations[i] = shared[next]
					next++
				}
			}
			return allocations, nil
		}
	}
	floors, ok := floorsFor(sizes, minimums, budget)
	if !ok {
		return nil, fmt.Errorf("%w: tool-result group metadata exceeds %d bytes", ErrContextOverflow, budget)
	}
	return largestFirstAllocationsWithFloors(sizes, budget, floors), nil
}

const (
	toolResultExcerptHeadMarker = "\n<head>\n"
	toolResultExcerptTailMarker = "\n<tail>\n"
)

func toolResultExcerptLabel(event memory.Event, reason string, head, tail int) string {
	return fmt.Sprintf(
		"[tool result excerpt: event_id=%s original_bytes=%d sha256=%x reason=%s shown=0-%d,%d-%d; "+
			"read the omitted bytes with read_tool_result event_id=%s offset=%d]",
		event.ID, len(event.Content), sha256.Sum256([]byte(event.Content)), reason,
		head, tail, len(event.Content), event.ID, head,
	)
}

// toolResultExcerptOverhead is the size of an excerpt that shows no content:
// its label with the widest offsets, and its markers.
func toolResultExcerptOverhead(event memory.Event, reason string) int {
	return len(toolResultExcerptLabel(event, reason, len(event.Content), len(event.Content))) +
		len(toolResultExcerptHeadMarker) + len(toolResultExcerptTailMarker)
}

// toolResultExcerpt shows a durable tool result in at most limit bytes: a
// label naming the event, its size and hash, why it was shortened, and the
// shown byte ranges, so the omitted middle can be paged with read_tool_result
// from the head's end; then the head and tail of the content. Cuts never
// split a UTF-8 sequence or an untrusted-data frame delimiter, and a frame
// that a cut leaves open is closed after the head and reopened before the
// tail with copies of its own delimiter lines, so each half is a complete
// data frame and harness text never lands inside one.
func toolResultExcerpt(event memory.Event, limit int, reason string) string {
	content := event.Content
	available := limit - toolResultExcerptOverhead(event, reason)
	if available < 0 {
		return validUTF8Prefix(toolResultExcerptLabel(event, reason, 0, len(content)), limit)
	}
	head, tail, headEnd, tailStart := frameSafeHeadTail(content, untrustedFrameSpans(content), available)
	return toolResultExcerptLabel(event, reason, headEnd, tailStart) +
		toolResultExcerptHeadMarker + head + toolResultExcerptTailMarker + tail
}

// textLine is one line of content: [start, end) excludes its newline.
type textLine struct{ start, end int }

// frameSpan is one untrusted-data frame of a tool result: a whole
// "[begin ...]" delimiter line and the later "[end ...]" line that closes it.
type frameSpan struct{ begin, end textLine }

// untrustedFrameSpans finds the data frames of content. Tools frame
// third-party text between a "[begin <kind>...]" line and an "[end <kind>]"
// line, both optionally numbered " #n" (internal/untrusted), and escape the
// payload so neither delimiter appears in it. An end line closes the
// innermost open begin line of its kind; lines that look like delimiters but
// never pair are payload.
func untrustedFrameSpans(content string) []frameSpan {
	var open []textLine
	var spans []frameSpan
	for start := 0; start < len(content); {
		end := strings.IndexByte(content[start:], '\n')
		if end < 0 {
			end = len(content)
		} else {
			end += start
		}
		line := textLine{start: start, end: end}
		text := content[start:end]
		switch {
		case strings.HasPrefix(text, "[begin ") && strings.HasSuffix(text, "]"):
			open = append(open, line)
		case strings.HasPrefix(text, "[end ") && strings.HasSuffix(text, "]"):
			for i := len(open) - 1; i >= 0; i-- {
				if frameDelimitersPair(content[open[i].start:open[i].end], text) {
					spans = append(spans, frameSpan{begin: open[i], end: line})
					open = open[:i]
					break
				}
			}
		}
		start = end + 1
	}
	return spans
}

var frameNumberSuffix = regexp.MustCompile(` #[0-9]+$`)

func frameDelimitersPair(begin, end string) bool {
	kind := strings.TrimSuffix(strings.TrimPrefix(end, "[end "), "]")
	number := frameNumberSuffix.FindString(kind)
	kind = strings.TrimSuffix(kind, number)
	body := strings.TrimSuffix(strings.TrimPrefix(begin, "[begin "), "]")
	if kind == "" || !strings.HasPrefix(body, kind) || (len(body) > len(kind) && body[len(kind)] != ' ') {
		return false
	}
	return number == "" || strings.HasSuffix(body, number)
}

// frameSafeHeadTail splits about available bytes between the head and tail
// of content, as head text, tail text, and the original offsets where the
// head ends and the tail starts. Re-emitted delimiter lines count toward
// available; when they do not fit, less content is shown.
func frameSafeHeadTail(content string, spans []frameSpan, available int) (string, string, int, int) {
	budget := available
	for {
		headBudget := (budget + 1) / 2
		headEnd := headCut(content, spans, headBudget)
		tailStart := max(headEnd, tailCut(content, spans, len(content)-(budget-headBudget)))
		closing := closeOpenFrames(content, spans, headEnd)
		opening := reopenFrames(content, spans, tailStart)
		size := headEnd + len(closing) + len(opening) + len(content) - tailStart
		if size <= available || budget == 0 {
			return content[:headEnd] + closing, opening + content[tailStart:], headEnd, tailStart
		}
		budget = max(0, budget-(size-available))
	}
}

// headCut is the head's end at or before limit: never inside a UTF-8
// sequence, and before any delimiter line it would split.
func headCut(content string, spans []frameSpan, limit int) int {
	cut := min(max(limit, 0), len(content))
	for i := 0; i < utf8.UTFMax-1 && cut > 0 && cut < len(content) && !utf8.RuneStart(content[cut]); i++ {
		cut--
	}
	for _, span := range spans {
		for _, line := range []textLine{span.begin, span.end} {
			if line.start < cut && cut < line.end {
				cut = line.start
			}
		}
	}
	return cut
}

// tailCut is the tail's start at or after from: never inside a UTF-8
// sequence, after any delimiter line it would split, and never at a "[" in
// mid-line, where a cut after an escaping backslash would make escaped
// payload text read as a delimiter.
func tailCut(content string, spans []frameSpan, from int) int {
	cut := min(max(from, 0), len(content))
	for i := 0; i < utf8.UTFMax-1 && cut < len(content) && !utf8.RuneStart(content[cut]); i++ {
		cut++
	}
	for _, span := range spans {
		for _, line := range []textLine{span.begin, span.end} {
			if line.start < cut && cut < line.end {
				cut = min(line.end+1, len(content))
			}
		}
	}
	for cut > 0 && cut < len(content) && content[cut-1] != '\n' && content[cut] == '[' {
		cut++
	}
	return cut
}

// closeOpenFrames copies, innermost first, the end line of every frame the
// head opens without closing.
func closeOpenFrames(content string, spans []frameSpan, headEnd int) string {
	var open []frameSpan
	for _, span := range spans {
		if span.begin.end <= headEnd && headEnd <= span.end.start {
			open = append(open, span)
		}
	}
	slices.SortFunc(open, func(a, b frameSpan) int { return b.begin.start - a.begin.start })
	var closing strings.Builder
	for _, span := range open {
		if closing.Len() > 0 || (headEnd > 0 && content[headEnd-1] != '\n') {
			closing.WriteByte('\n')
		}
		closing.WriteString(content[span.end.start:span.end.end])
	}
	return closing.String()
}

// reopenFrames copies, outermost first, the begin line of every frame the
// tail closes without opening.
func reopenFrames(content string, spans []frameSpan, tailStart int) string {
	var open []frameSpan
	for _, span := range spans {
		if span.begin.end <= tailStart && tailStart <= span.end.start {
			open = append(open, span)
		}
	}
	slices.SortFunc(open, func(a, b frameSpan) int { return a.begin.start - b.begin.start })
	var opening strings.Builder
	for _, span := range open {
		opening.WriteString(content[span.begin.start:span.begin.end])
		opening.WriteByte('\n')
	}
	return opening.String()
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
	return projectToolResultWithLabel(event, "older tool result projected")
}

// projectProtectedToolResult is the older-result projection of a protected
// result, which applies only when the request cannot fit without it.
func projectProtectedToolResult(event memory.Event) string {
	return projectToolResultWithLabel(event, "protected tool result projected to fit the context budget")
}

func projectToolResultWithLabel(event memory.Event, kind string) string {
	digest := sha256.Sum256([]byte(event.Content))
	label := fmt.Sprintf(
		"[%s: event_id=%s original_bytes=%d sha256=%x]",
		kind, event.ID, len(event.Content), digest,
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
