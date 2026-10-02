package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// Inline result bounds. The full report stays readable through
// ReadSubagentReport, so only the summary and the lists are cut here.
const (
	maxResultSources       = 24
	maxResultUnverified    = 8
	maxResultURLBytes      = 512
	maxReportLimitations   = 8
	maxReportLimitationLen = 400

	defaultReportPageBytes = 16 * 1024
	minReportPageBytes     = 256
	maxReportPageBytes     = 32 * 1024
	maxReportPageEnvelope  = 64 * 1024
)

// webURL is one URL the child's own Web tool events returned, in the order
// the events first returned it. recent marks a page fetched during the
// attempt being settled; earlier turns of a continued child only verify the
// report's citations.
type webURL struct {
	url      string
	fetched  bool
	returned bool
	recent   bool
}

// subagentWebEvidence reads the URLs the child actually fetched (successful
// web_fetch results) or was shown as web_search results, in every turn of
// the child session; those after sequence after belong to the attempt being
// settled. Snippet text and other free text in tool results is not evidence
// of a source.
func subagentWebEvidence(ctx context.Context, conn *sql.Conn, child memory.SessionID, after int64) ([]webURL, error) {
	rows, err := conn.QueryContext(ctx, `SELECT i.payload_json,o.content,o.sequence>? FROM events o
 JOIN events i ON i.session_id=o.session_id AND i.execution_id=o.execution_id AND i.event_type='tool_intent'
 WHERE o.session_id=? AND o.event_type='tool_succeeded' ORDER BY o.sequence`, after, child)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []webURL
	index := map[string]int{}
	add := func(raw string, fetched, recent bool) {
		key, ok := sourceKey(raw)
		if !ok {
			return
		}
		i, seen := index[key]
		if !seen {
			i = len(found)
			index[key] = i
			found = append(found, webURL{url: raw})
		}
		if fetched {
			found[i].fetched = true
			found[i].recent = found[i].recent || recent
		} else {
			found[i].returned = true
		}
	}
	for rows.Next() {
		var payload []byte
		var content string
		var recent bool
		if err = rows.Scan(&payload, &content, &recent); err != nil {
			return nil, err
		}
		var intent memory.ToolIntentPayload
		if err = json.Unmarshal(payload, &intent); err != nil {
			return nil, err
		}
		switch intent.Call.Name {
		case "web_fetch":
			if fetched := fetchedURL(intent.Call.Arguments, content); fetched != "" {
				add(fetched, true, recent)
			}
		case "web_search":
			for _, line := range strings.Split(content, "\n") {
				if result := searchResultURL(line); result != "" {
					add(result, false, recent)
				}
			}
		}
	}
	return found, rows.Err()
}

// fetchedURL is the page a successful web_fetch read: the excerpt contract's
// reported source URL, or the requested URL for the legacy text contract. A
// cross-host redirect notice read nothing.
func fetchedURL(arguments, content string) string {
	var excerpt struct {
		URL string `json:"url"`
	}
	if json.Unmarshal([]byte(content), &excerpt) == nil && excerpt.URL != "" {
		return excerpt.URL
	}
	if strings.HasPrefix(content, "This URL redirects to a different host") {
		return ""
	}
	var request struct {
		URL string `json:"url"`
	}
	if json.Unmarshal([]byte(arguments), &request) != nil {
		return ""
	}
	return request.URL
}

var numberedResult = regexp.MustCompile(`^\d+\.\s+`)

// searchResultURL recognizes web_search's result URL lines: a line that is
// exactly one URL, optionally numbered when it is also the result's title.
func searchResultURL(line string) string {
	line = numberedResult.ReplaceAllString(strings.TrimSpace(line), "")
	if strings.ContainsAny(line, " \t") || !strings.HasPrefix(line, "http") {
		return ""
	}
	if _, ok := sourceKey(line); !ok {
		return ""
	}
	return line
}

// sourceKey compares URLs as citations: scheme and host case, a fragment and
// a trailing slash do not distinguish them.
func sourceKey(raw string) (string, bool) {
	if len(raw) > maxResultURLBytes || !utf8.ValidString(raw) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u.String(), true
}

var reportURL = regexp.MustCompile(`https?://[^\s<>"\x60]+`)

// citedURLs lists the URLs a report mentions, in order and without repeats.
func citedURLs(report string) []string {
	var cited []string
	seen := map[string]bool{}
	for _, raw := range reportURL.FindAllString(report, -1) {
		raw = trimCitation(raw)
		key, ok := sourceKey(raw)
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		cited = append(cited, raw)
	}
	return cited
}

// trimCitation removes sentence punctuation and unbalanced closing brackets
// that end a URL written in prose or Markdown.
func trimCitation(raw string) string {
	for raw != "" {
		last := raw[len(raw)-1]
		switch {
		case strings.IndexByte(".,;:!?*'", last) >= 0:
		case last == ')' && strings.Count(raw, "(") < strings.Count(raw, ")"):
		case last == ']' && strings.Count(raw, "[") < strings.Count(raw, "]"):
		case last == '}' && strings.Count(raw, "{") < strings.Count(raw, "}"):
		default:
			return raw
		}
		raw = raw[:len(raw)-1]
	}
	return raw
}

var markdownHeading = regexp.MustCompile(`^(#{1,6})[ \t]+(.*?)[ \t#]*$`)

// reportSection returns the body under the first heading whose title starts
// with name, up to the next heading of the same or a higher level.
func reportSection(report, name string) (string, bool) {
	lines := strings.SplitAfter(report, "\n")
	fenced := false
	level, start, offset := 0, -1, 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
		}
		if match := markdownHeading.FindStringSubmatch(strings.TrimRight(line, "\r\n")); match != nil && !fenced {
			if start >= 0 && len(match[1]) <= level {
				return report[start:offset], true
			}
			title := strings.ToLower(strings.Trim(match[2], " \t*_:"))
			if start < 0 && strings.HasPrefix(title, name) {
				level, start = len(match[1]), offset+len(line)
			}
		}
		offset += len(line)
	}
	if start < 0 {
		return "", false
	}
	return report[start:], true
}

var limitationBullet = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+(.*)$`)

// reportLimitations splits the report's Limitations section into items.
func reportLimitations(report string) []string {
	body, ok := reportSection(report, "limitation")
	if !ok {
		return nil
	}
	var items []string
	for _, line := range strings.Split(body, "\n") {
		if match := limitationBullet.FindStringSubmatch(line); match != nil {
			items = append(items, strings.TrimSpace(match[1]))
		} else if line = strings.TrimSpace(line); line == "" {
			continue
		} else if len(items) == 0 {
			items = append(items, line)
		} else {
			items[len(items)-1] += " " + line
		}
	}
	bounded := items[:0]
	for _, item := range items {
		if item == "" {
			continue
		}
		if len(bounded) == maxReportLimitations {
			break
		}
		if len(item) > maxReportLimitationLen {
			item = utf8Prefix(item, maxReportLimitationLen-len("…")) + "…"
		}
		bounded = append(bounded, item)
	}
	return bounded
}

func utf8Prefix(value string, n int) string {
	if n <= 0 {
		return ""
	}
	if n >= len(value) {
		return value
	}
	for n > 0 && !utf8.RuneStart(value[n]) {
		n--
	}
	return value[:n]
}

var wrapUpPhrases = map[string]string{
	delegation.WrapUpTime:    "90% of its time budget",
	delegation.WrapUpTokens:  "90% of its token budget",
	delegation.WrapUpSteps:   "its model-response limit",
	delegation.WrapUpContext: "90% of its context budget",
}

// subagentNextSteps is what the harness can tell the parent about extending
// a child: whether its composition has continue_research, and which attempt
// a continuation extended.
type subagentNextSteps struct {
	continuable bool
	continues   string
}

// buildSubagentResult fills the inline result from the child's final report
// and Web evidence, then bounds it to limit bytes. The summary is cut first,
// because the full report stays readable; lists lose their least useful
// entries next. Every cut is stated in the limitations. Every turn's evidence
// verifies citations, but only pages fetched during this attempt are listed
// uncited or counted as salvage: earlier turns' results already listed theirs.
func buildSubagentResult(r *delegation.Result, hasReport bool, report string, evidence []webURL, limit int, next subagentNextSteps) {
	citedKeys := map[string]bool{}
	var unverified []string
	known := map[string]bool{}
	for _, e := range evidence {
		key, _ := sourceKey(e.url)
		known[key] = true
	}
	for _, raw := range citedURLs(report) {
		key, _ := sourceKey(raw)
		citedKeys[key] = true
		if !known[key] {
			unverified = append(unverified, raw)
		}
	}
	var cited, uncited []delegation.Source
	fetched := 0
	for _, e := range evidence {
		key, _ := sourceKey(e.url)
		if e.fetched && e.recent {
			fetched++
		}
		switch {
		case citedKeys[key]:
			cited = append(cited, delegation.Source{URL: e.url, Fetched: e.fetched, Cited: true})
		case e.fetched && e.recent:
			uncited = append(uncited, delegation.Source{URL: e.url, Fetched: true})
		}
	}
	omitted := 0
	trim := func(list *[]delegation.Source, n int) {
		for len(*list) > n {
			*list = (*list)[:len(*list)-1]
			omitted++
		}
	}
	trim(&cited, maxResultSources)
	trim(&uncited, maxResultSources-len(cited))
	if len(unverified) > maxResultUnverified {
		omitted += len(unverified) - maxResultUnverified
		unverified = unverified[:maxResultUnverified]
	}

	var harness []string
	switch r.Status {
	case "succeeded":
	case delegation.StatePartial:
		phrase := wrapUpPhrases[r.Reason]
		if phrase == "" {
			phrase = "its budget"
		}
		if next.continuable {
			harness = append(harness, "Stopped at "+phrase+" and wrapped up without tools; the report covers only the work done before then. continue_research can extend this child from its report with a fresh budget; a new idempotency key starts over.")
		} else {
			harness = append(harness, "Stopped at "+phrase+" and wrapped up without tools; the report covers only the work done before then. A new idempotency key is required for another attempt.")
		}
	default:
		if next.continues != "" {
			harness = append(harness, fmt.Sprintf("Continuation did not complete; continue_research can still extend the child from execution %s.", next.continues))
		} else {
			harness = append(harness, "Assignment did not complete; a new idempotency key is required for another attempt.")
		}
	}
	if !hasReport && r.Status != "succeeded" {
		if fetched > 0 {
			harness = append(harness, fmt.Sprintf("No report was produced. Before stopping, the child fetched %d page(s), listed in sources and not reviewed in any report.", fetched))
		} else {
			harness = append(harness, "No report was produced.")
		}
	}
	if len(unverified) > 0 {
		harness = append(harness, "URLs in unverified_urls are cited in the report but were never fetched or returned by a search.")
	}
	var fromReport []string
	if hasReport {
		summary, ok := reportSection(report, "summary")
		if summary = strings.TrimSpace(summary); !ok || summary == "" {
			summary = strings.TrimSpace(report)
		}
		r.Summary = summary
		r.ReportBytes = len(report)
		fromReport = reportLimitations(report)
	}
	compose := func() {
		r.Sources = append(append([]delegation.Source(nil), cited...), uncited...)
		r.UnverifiedURLs = unverified
		r.Limitations = append([]string(nil), harness...)
		if omitted > 0 {
			r.Limitations = append(r.Limitations, fmt.Sprintf("%d more source URL(s) were omitted from this inline result.", omitted))
		}
		if r.SummaryTruncated {
			r.Limitations = append(r.Limitations, "The inline summary was cut to fit the result limit; read the full report with read_subagent_report.")
		}
		r.Limitations = append(r.Limitations, fromReport...)
		if len(r.Limitations) == 0 {
			r.Limitations = nil
		}
	}
	for {
		compose()
		b, _ := json.Marshal(r)
		over := len(b) - limit
		if over <= 0 {
			return
		}
		switch {
		case r.Summary != "" && !r.SummaryTruncated:
			r.SummaryTruncated = true
		case r.Summary != "":
			r.Summary = utf8Prefix(r.Summary, len(r.Summary)-over)
		case len(fromReport) > 0:
			fromReport = fromReport[:len(fromReport)-1]
		case len(uncited) > 0:
			trim(&uncited, len(uncited)-1)
		case len(unverified) > 0:
			unverified = unverified[:len(unverified)-1]
			omitted++
		case len(cited) > 0:
			trim(&cited, len(cited)-1)
		case len(harness) > 0:
			harness = harness[:len(harness)-1]
		default:
			return
		}
	}
}

// RecordSubagentWrapUp durably marks a running child as wrapping up at a
// budget before its tool-free final call. An accepted final answer then
// settles as partial, including through recovery after a crash.
func (s *Store) RecordSubagentWrapUp(ctx context.Context, id, reason string) error {
	if wrapUpPhrases[reason] == "" {
		return errors.New("invalid subagent wrap-up reason")
	}
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
		if err != nil {
			return err
		}
		if a.State != "running" {
			return delegation.ErrAuthority
		}
		if a.WrapUp != nil {
			return nil
		}
		if err = s.authorizeSubagentChild(ctx, conn, a.Child.ID); err != nil {
			return err
		}
		a.WrapUp = &delegation.WrapUp{Reason: reason, At: s.now().UTC()}
		return writeSubagent(ctx, conn, a)
	})
}

// ReadSubagentReport pages a child's stored final report for the parent
// session that delegated it, under the same current access checks as
// InspectSubagent. Pages break only at UTF-8 boundaries.
func (s *Store) ReadSubagentReport(ctx context.Context, p delegation.Parent, id string, offset, limit int) (delegation.ReportPage, error) {
	if limit == 0 {
		limit = defaultReportPageBytes
	}
	if offset < 0 || limit < minReportPageBytes || limit > maxReportPageBytes {
		return delegation.ReportPage{}, fmt.Errorf("invalid report page: offset must be nonnegative and limit %d to %d bytes", minReportPageBytes, maxReportPageBytes)
	}
	var page delegation.ReportPage
	err := s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		a, err := s.inspectSubagent(ctx, conn, p, id)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("no delegated research execution %q belongs to this session", id)
		}
		if err != nil {
			return err
		}
		if a.FinalEventID == "" {
			return fmt.Errorf("execution %q has no stored report (status %s)", id, a.State)
		}
		var report string
		if err = conn.QueryRowContext(ctx, `SELECT content FROM events WHERE id=? AND session_id=? AND event_type='assistant_message'`, a.FinalEventID, a.Child.ID).Scan(&report); err != nil {
			return fmt.Errorf("load stored report: %w", err)
		}
		if offset > len(report) || (offset < len(report) && !utf8.RuneStart(report[offset])) {
			return fmt.Errorf("offset must be within the %d-byte report at a UTF-8 boundary; use next_offset", len(report))
		}
		page = delegation.ReportPage{ExecutionID: a.ID, Status: a.State, TotalBytes: len(report), Offset: offset}
		end := offset + len(utf8Prefix(report[offset:], limit))
		for {
			page.End, page.Text, page.NextOffset = end, report[offset:end], nil
			if end < len(report) {
				next := end
				page.NextOffset = &next
			}
			encoded, err := json.Marshal(page)
			if err != nil {
				return err
			}
			if len(encoded) <= maxReportPageEnvelope || end-offset <= utf8.UTFMax {
				return nil
			}
			end = offset + len(utf8Prefix(report[offset:end], (end-offset)/2))
		}
	})
	return page, err
}
