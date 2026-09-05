package eviedb

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// This comparison projection is separate from the original candidate/request
// encodings. It never edits them and never supplies acceptance authority.
type compilerRecurrenceEncoding struct {
	Version     string                     `json:"version"`
	Policies    []string                   `json:"policies"`
	Destination string                     `json:"destination"`
	Session     memory.SessionID           `json:"session"`
	Root        memory.EventID             `json:"root,omitempty"`
	Proposal    memory.ExtractorCandidate  `json:"proposal"`
	Entities    []memory.SemanticEntity    `json:"entities"`
	Aliases     []memory.SemanticAlias     `json:"aliases"`
	Predicates  []memory.SemanticPredicate `json:"predicates"`
	Support     []memory.CompilerSource    `json:"support"`
	Context     []memory.CompilerSource    `json:"context"`
}

func compilerRecurrenceCanonical(g memory.CompilerGeneration, r memory.CompilerRequest, c memory.MemoryCandidate) (exact, related []byte, err error) {
	// Detach every nested pointer before normalizing display/order/time fields.
	var p memory.ExtractorCandidate
	if err = json.Unmarshal(compilerJSON(c.Proposal), &p); err != nil {
		return
	}
	if p.Identity != nil {
		p.Identity.Confidence = nil
		p.Identity.Uncertainty = ""
	}
	canonicalTime := func(v *time.Time) *time.Time {
		if v == nil {
			return nil
		}
		x := v.UTC()
		return &x
	}
	p.ValidTime.From = canonicalTime(p.ValidTime.From)
	p.ValidTime.To = canonicalTime(p.ValidTime.To)
	if p.Temporal != nil && p.Temporal.Correction != nil {
		p.Temporal.Correction.EffectiveTime = canonicalTime(p.Temporal.Correction.EffectiveTime)
		sort.Slice(p.Temporal.Correction.Modes, func(i, j int) bool { return p.Temporal.Correction.Modes[i] < p.Temporal.Correction.Modes[j] })
	}
	sortCanonical(p.Support)
	sortCanonical(p.Context)
	out := compilerRecurrenceEncoding{Version: "compiler-recurrence-v2", Policies: []string{g.EvidencePolicy, g.SecretPolicy, g.ClosurePolicy, g.WindowPolicy, g.EntityPolicy, g.PredicatePolicy, g.ValidationPolicy, g.EquivalencePolicy, g.EffectPolicy}, Destination: r.Window.Selection.Destination, Session: r.Window.Selection.SessionID, Root: r.Window.Selection.RootID, Proposal: p, Entities: []memory.SemanticEntity{}, Aliases: []memory.SemanticAlias{}, Predicates: []memory.SemanticPredicate{}, Support: append([]memory.CompilerSource{}, c.Support...), Context: append([]memory.CompilerSource{}, c.Context...)}
	bound := map[memory.SemanticID]bool{}
	for _, id := range []memory.SemanticID{p.Proposition.SubjectEntityID, p.Proposition.Object.EntityID} {
		if id != "" {
			bound[id] = true
		}
	}
	// Unresolved mentions already contain their exact source-bound placeholder.
	// Current name matches are review options, not an immutable model binding:
	// accepting a new Entity must not manufacture new original-output meaning.
	for _, e := range r.Entities {
		if bound[e.ID] {
			e.Create = false
			out.Entities = append(out.Entities, e)
			delete(bound, e.ID)
		}
	}
	if len(bound) != 0 {
		return nil, nil, errors.New("recurrence missing bound Entity")
	}
	if p.Proposition.PredicateID != "" {
		for _, predicate := range r.Predicates {
			if predicate.ID == p.Proposition.PredicateID {
				predicate.Create = false
				out.Predicates = append(out.Predicates, predicate)
			}
		}
		if len(out.Predicates) != 1 {
			return nil, nil, errors.New("recurrence missing bound Predicate")
		}
	}
	sortCanonical(out.Entities)
	sortCanonical(out.Aliases)
	sortCanonical(out.Predicates)
	sortCanonical(out.Support)
	sortCanonical(out.Context)
	exact = compilerJSON(out)
	// This second key can explain a changed evidence attachment. Unresolved
	// mentions keep their source locators, never relating different people by name.
	out.Root = ""
	out.Session = ""
	out.Proposal.Support = []memory.EvidenceLocator{}
	out.Proposal.Context = []memory.EvidenceLocator{}
	out.Support = []memory.CompilerSource{}
	out.Context = []memory.CompilerSource{}
	related = compilerJSON(out)
	return
}

func sortCanonical[T any](items []T) {
	sort.Slice(items, func(i, j int) bool { return string(compilerJSON(items[i])) < string(compilerJSON(items[j])) })
}
