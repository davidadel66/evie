package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type goldExpectation struct {
	Label   string      `json:"label"`
	Meaning meaning     `json:"meaning"`
	Sources []reference `json:"sources"`
	Context []reference `json:"context"`
}
type goldCase struct {
	CaseID        string            `json:"case_id"`
	Expected      []goldExpectation `json:"expected"`
	NoMemoryLabel string            `json:"no_memory_label"`
	Forbidden     string            `json:"forbidden"`
	Uncertainty   string            `json:"uncertainty"`
}
type adjudication struct {
	GoldMatches     []int    `json:"gold_matches,omitempty"`
	CaseID          string   `json:"case_id"`
	CandidateSHA256 string   `json:"candidate_sha256"`
	Label           string   `json:"label"`
	Errors          []string `json:"errors"`
	Note            string   `json:"note"`
}
type panel struct {
	Proposals           int            `json:"proposals"`
	SupportedUseful     int            `json:"supported_useful"`
	RequiredMatches     int            `json:"required_matches"`
	RequiredDenominator int            `json:"required_denominator"`
	Unsupported         int            `json:"unsupported"`
	Unwanted            int            `json:"unwanted"`
	Unadjudicated       int            `json:"unadjudicated"`
	Omissions           int            `json:"confirmed_required_omissions"`
	Errors              map[string]int `json:"errors"`
	PrecisionLower      *float64       `json:"precision_confirmed_lower_bound"`
	PrecisionUpper      *float64       `json:"precision_possible_upper_bound"`
	RecallLower         *float64       `json:"recall_confirmed_lower_bound"`
}
type unresolved struct {
	RawProposal     json.RawMessage   `json:"raw_proposal"`
	CaseID          string            `json:"case_id"`
	CandidateSHA256 string            `json:"candidate_sha256"`
	Candidate       candidate         `json:"candidate"`
	Gold            []goldExpectation `json:"reviewed_expected"`
	Forbidden       string            `json:"forbidden"`
	Occurrences     int               `json:"occurrences"`
}
type scoreSummary struct {
	WireVersion        string         `json:"wire_version,omitempty"`
	Rejections         map[string]int `json:"structural_rejections,omitempty"`
	Report             string         `json:"report"`
	ReportSHA256       string         `json:"report_sha256"`
	Mode               string         `json:"mode"`
	PlannedRuns        int            `json:"planned_runs"`
	AttemptedRuns      int            `json:"attempted_runs"`
	UnexecutedRuns     int            `json:"unexecuted_runs"`
	Failures           map[string]int `json:"statuses"`
	UnparseableRawRuns int            `json:"unparseable_raw_runs"`
	Raw                panel          `json:"raw"`
	Retained           panel          `json:"retained"`
	LatencyP50MS       float64        `json:"latency_p50_ms"`
	LatencyP95MS       float64        `json:"latency_p95_ms"`
	LatencyMaxMS       float64        `json:"latency_max_ms"`
}

func sourceKey(refs []reference) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		b, _ := json.Marshal(r)
		parts = append(parts, string(b))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}
func canonicalProposal(raw []byte) []byte {
	var v any
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return raw
	}
	b, _ := json.Marshal(v)
	return b
}
func candidateDigest(c candidate) string {
	b, _ := json.Marshal(c)
	return digest(canonicalProposal(b))
}
func fraction(a, b int) *float64 {
	if b == 0 {
		return nil
	}
	v := float64(a) / float64(b)
	return &v
}

func scoreReports(paths, goldPath, approvalPath, adjudicationPath, output string) error {
	goldBytes, err := readBounded(goldPath, 2<<20)
	if err != nil {
		return err
	}
	var corpusGold struct {
		Cases []goldCase `json:"cases"`
	}
	if err = json.Unmarshal(goldBytes, &corpusGold); err != nil {
		return err
	}
	approvalBytes, err := readBounded(approvalPath, 32<<10)
	if err != nil {
		return err
	}
	var approval struct {
		Status     string            `json:"status"`
		Reviewer   string            `json:"reviewer"`
		ReviewedAt string            `json:"reviewed_at"`
		Approved   map[string]string `json:"approved_file_sha256"`
	}
	if json.Unmarshal(approvalBytes, &approval) != nil || approval.Status != "human_approved" || approval.Reviewer == "" || approval.ReviewedAt == "" || approval.Approved[filepath.Base(goldPath)] != digest(goldBytes) {
		return errors.New("matching actual human approval is required for gold scoring")
	}
	gold := map[string]goldCase{}
	for _, g := range corpusGold.Cases {
		gold[g.CaseID] = g
	}
	sourcePath := strings.Replace(goldPath, ".gold.json", ".json", 1)
	sourceBytes, err := readBounded(sourcePath, 2<<20)
	if err != nil {
		return err
	}
	if approval.Approved[filepath.Base(sourcePath)] != digest(sourceBytes) {
		return errors.New("source corpus differs from human approval")
	}
	var reviewedSources corpus
	if json.Unmarshal(sourceBytes, &reviewedSources) != nil {
		return errors.New("invalid reviewed sources")
	}
	scopes := map[string]string{}
	windows := map[string]window{}
	histories := map[string]history{}
	for _, h := range reviewedSources.Histories {
		for _, w := range h.Windows {
			scopes[w.ID] = w.Input.Scope
			windows[w.ID] = w
			histories[w.ID] = h
		}
	}
	adjudicated := map[string]adjudication{}
	adjudicationSHA := ""
	if adjudicationPath != "" {
		b, err := readBounded(adjudicationPath, 2<<20)
		if err != nil {
			return err
		}
		var a struct {
			GoldSHA256        string         `json:"gold_sha256"`
			PacketFile        string         `json:"packet_file"`
			PacketSHA256      string         `json:"packet_sha256"`
			SourceScoreFile   string         `json:"source_score_file"`
			SourceScoreSHA256 string         `json:"source_score_sha256"`
			Status            string         `json:"status"`
			Reviewer          string         `json:"reviewer"`
			ReviewedAt        string         `json:"reviewed_at"`
			Decisions         []adjudication `json:"decisions"`
		}
		if json.Unmarshal(b, &a) != nil || a.Status != "human_approved" || a.Reviewer == "" || a.ReviewedAt == "" {
			return errors.New("actual human output adjudication required")
		}
		if a.GoldSHA256 != digest(goldBytes) {
			return errors.New("adjudication gold binding mismatch")
		}
		for _, f := range []struct{ name, hash string }{{a.PacketFile, a.PacketSHA256}, {a.SourceScoreFile, a.SourceScoreSHA256}} {
			if f.name == "" || filepath.IsAbs(f.name) || filepath.Clean(f.name) != f.name || strings.HasPrefix(f.name, "..") {
				return errors.New("invalid adjudication evidence path")
			}
			evidence, err := readBounded(filepath.Join(filepath.Dir(adjudicationPath), f.name), 2<<20)
			if err != nil || digest(evidence) != f.hash {
				return errors.New("adjudication packet/source-score hash mismatch")
			}
		}
		adjudicationSHA = digest(b)
		for _, d := range a.Decisions {
			if len(d.GoldMatches) > 1 {
				return errors.New("one atomic proposal cannot claim multiple gold matches")
			}
			if d.Label != "unsupported" && d.Label != "unwanted_but_true" && d.Label != "optional_useful" && d.Label != "required_useful" {
				return errors.New("unknown adjudication label")
			}
			adjudicated[d.CaseID+"/"+d.CandidateSHA256] = d
		}
	}
	unknown := map[string]*unresolved{}
	var summaries []scoreSummary
	for _, path := range strings.Split(paths, ",") {
		b, err := readBounded(path, 4<<20)
		if err != nil {
			return err
		}
		var r report
		if json.Unmarshal(b, &r) != nil || r.Version != "evie-extraction-spike-report-v1" {
			return errors.New("invalid report")
		}
		matchedCorpus := false
		for file, hash := range approval.Approved {
			if !strings.Contains(file, ".gold.") && strings.HasSuffix(file, ".json") && hash == r.CorpusSHA256 {
				matchedCorpus = true
			}
		}
		if !matchedCorpus {
			return errors.New("report source corpus differs from reviewed corpus")
		}
		if r.WireVersion != "" && !isCompactWire(r.WireVersion) {
			return errors.New("unknown report wire version")
		}
		s := scoreSummary{WireVersion: r.WireVersion, Rejections: map[string]int{}, Report: path, ReportSHA256: digest(b), Mode: r.Mode, PlannedRuns: len(r.PlannedCases) * r.Repetitions, AttemptedRuns: len(r.Runs), Failures: map[string]int{}, Raw: panel{Errors: map[string]int{}}, Retained: panel{Errors: map[string]int{}}}
		s.UnexecutedRuns = s.PlannedRuns - s.AttemptedRuns
		latencies := []float64{}
		for _, run := range r.Runs {
			if isCompactWire(r.WireVersion) {
				var err error
				run, err = verifyCompactRun(r, run, histories[run.CaseID], windows[run.CaseID], reviewedSources.EvidencePolicy)
				if err != nil {
					return err
				}
				for _, p := range run.Proposals {
					if p.Rejection != "" {
						s.Rejections[strings.SplitN(p.Rejection, ":", 2)[0]]++
					}
				}
			}
			g, ok := gold[run.CaseID]
			if !ok {
				return fmt.Errorf("missing reviewed gold for %s", run.CaseID)
			}
			s.Failures[run.Status]++
			latencies = append(latencies, run.LatencyMS)
			required := 0
			for _, e := range g.Expected {
				if e.Label == "required_useful" {
					required++
				}
			}
			s.Raw.RequiredDenominator += required
			s.Retained.RequiredDenominator += required
			var raw struct {
				Candidates []json.RawMessage `json:"candidates"`
			}
			parseable := json.Unmarshal([]byte(run.Raw), &raw) == nil && raw.Candidates != nil
			rawShapeOK := validateShape(run.Raw) == nil
			if isCompactWire(r.WireVersion) {
				rawShapeOK = compactShape(run.Raw, run.CaseID) == nil
			}
			if !parseable {
				s.UnparseableRawRuns++
			}
			for axis := 0; axis < 2; axis++ {
				p := &s.Raw
				candidates := raw.Candidates
				if axis == 1 {
					p = &s.Retained
					candidates = nil
					for _, pr := range run.Proposals {
						if pr.Retained {
							b, _ := json.Marshal(pr.Candidate)
							if isCompactWire(r.WireVersion) {
								b = pr.WireRaw
							}
							candidates = append(candidates, b)
						}
					}
				}
				matchedRequired := map[int]bool{}
				unknownThisRun := 0
				for _, rawCandidate := range candidates {
					var c candidate
					decoded := json.Unmarshal(rawCandidate, &c) == nil
					if isCompactWire(r.WireVersion) {
						var err error
						if compactResponseBound(run.Raw, run.CaseID) {
							c, err = expandCompact(rawCandidate, run.Compact.Seal)
							decoded = err == nil
						} else {
							c = candidate{}
							decoded = false
						}
					}
					rawIdentity := digest(canonicalProposal(rawCandidate))
					p.Proposals++
					found := -1
					for i, e := range g.Expected {
						if decoded && (axis == 1 || rawShapeOK) && c.Scope == scopes[run.CaseID] && c.meaning == e.Meaning && sourceKey(c.Sources) == sourceKey(e.Sources) && sourceKey(c.Context) == sourceKey(e.Context) {
							found = i
							break
						}
					}
					if found >= 0 {
						p.SupportedUseful++
						if g.Expected[found].Label == "required_useful" {
							matchedRequired[found] = true
						}
						continue
					}
					key := run.CaseID + "/" + rawIdentity
					if d, ok := adjudicated[key]; ok {
						switch d.Label {
						case "unsupported":
							p.Unsupported++
						case "unwanted_but_true":
							p.Unwanted++
						case "optional_useful", "required_useful":
							p.SupportedUseful++
							for _, index := range d.GoldMatches {
								if index < 0 || index >= len(g.Expected) || g.Expected[index].Label != "required_useful" {
									return errors.New("adjudicated gold match index is invalid")
								}
								matchedRequired[index] = true
							}
						}
						for _, category := range d.Errors {
							p.Errors[category]++
						}
					} else {
						p.Unadjudicated++
						unknownThisRun++
						if axis == 0 {
							if u := unknown[key]; u != nil {
								u.Occurrences++
							} else {
								unknown[key] = &unresolved{CaseID: run.CaseID, CandidateSHA256: rawIdentity, RawProposal: canonicalProposal(rawCandidate), Candidate: c, Gold: g.Expected, Forbidden: g.Forbidden, Occurrences: 1}
							}
						}
					}
				}
				p.RequiredMatches += len(matchedRequired)
				if unknownThisRun == 0 {
					p.Omissions += required - len(matchedRequired)
				}
			}
		}
		for _, p := range []*panel{&s.Raw, &s.Retained} {
			p.PrecisionLower = fraction(p.SupportedUseful, p.Proposals)
			p.PrecisionUpper = fraction(p.SupportedUseful+p.Unadjudicated, p.Proposals)
			p.RecallLower = fraction(p.RequiredMatches, p.RequiredDenominator)
		}
		sort.Float64s(latencies)
		if len(latencies) > 0 {
			s.LatencyP50MS = latencies[(len(latencies)-1)*50/100]
			s.LatencyP95MS = latencies[(len(latencies)-1)*95/100]
			s.LatencyMaxMS = latencies[len(latencies)-1]
		}
		summaries = append(summaries, s)
	}
	keys := make([]string, 0, len(unknown))
	for key := range unknown {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	list := make([]unresolved, 0, len(keys))
	for _, key := range keys {
		list = append(list, *unknown[key])
	}
	status := "human_reviewed_listed_meanings"
	if len(list) > 0 {
		status = "output_adjudication_pending"
	}
	result := map[string]any{"version": "evie-extraction-spike-score-v2", "status": status, "gold_sha256": digest(goldBytes), "annotation_record_sha256": digest(approvalBytes), "adjudication_file_sha256": adjudicationSHA, "reports": summaries, "unadjudicated": list, "metric_policy": "Raw means JSON-decodable proposals, including proposals rejected by declared shape/source checks. Compact wire proposals retain their wire identity and denominator even if expansion fails; exact credit requires verified canonical expansion, not just higher structural retention. Unparseable runs remain explicit failures and required-recall attempts. Confirmed exact reviewed meanings/sources are matched; all unlisted meanings require human adjudication. Unexecuted planned runs are reported separately, never filled in. Structural retention is not entailment. Precision intervals bound unadjudicated proposals, not statistical confidence intervals."}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return errors.Join(err, f.Close())
}
