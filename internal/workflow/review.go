package workflow

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// This file is what activation (Stage 14 sub-stage 2) needs from a validated
// definition: its exact files, the requirements one approval binds besides
// the executable hash, the authority ceiling check, and the review diff
// between two versions. It stays pure: storage, Git and approval live
// outside this package.

// Name is the definition's name, its identity within a Workspace.
func (v *Validated) Name() string { return v.definition.Name }

// Files returns a copy of the definition's reviewed files by slash path:
// workflow.yaml, README.md and every prompt and schema it references, with
// the exact bytes validation read. Nothing else in the definition's
// directory belongs to it.
func (v *Validated) Files() map[string][]byte {
	out := map[string][]byte{"workflow.yaml": slices.Clone(v.source), "README.md": slices.Clone(v.readme)}
	for name, data := range v.files {
		out[name] = slices.Clone(data)
	}
	return out
}

// Requirements are what one Workflow Approval binds besides the executable
// hash: the requested Standing Authority in canonical order with its own
// hash, the worker presets agent steps run (checked against the Workspace's
// allowances, which are not authority), and the schedule the same approval
// activates (ADR 0008, 0020, 0027).
type Requirements struct {
	Authority     []AuthorityReport `json:"authority"`
	AuthorityHash string            `json:"authority_hash"`
	Workers       []string          `json:"workers"`
	Schedule      *ScheduleReport   `json:"schedule,omitempty"`
}

// authorityFormat names the canonical document the authority hash covers.
const authorityFormat = "evie.workflow.authority/1"

// RequirementsOf derives a report's requirements.
func RequirementsOf(r Report) Requirements {
	req := Requirements{Authority: canonicalAuthority(r.Authority), Workers: []string{}, Schedule: r.Schedule}
	req.AuthorityHash = AuthorityHash(req.Authority)
	for _, w := range r.Workers {
		if !slices.Contains(req.Workers, w.Worker) {
			req.Workers = append(req.Workers, w.Worker)
		}
	}
	sort.Strings(req.Workers)
	return req
}

// canonicalAuthority sorts operations by capability and connection and each
// operation's resources, without changing the caller's slices.
func canonicalAuthority(ops []AuthorityReport) []AuthorityReport {
	out := make([]AuthorityReport, 0, len(ops))
	for _, op := range ops {
		op.Resources = sortedStrings(op.Resources)
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Capability+"\x00"+out[i].Connection < out[j].Capability+"\x00"+out[j].Connection
	})
	return out
}

// AuthorityHash identifies exact requested Standing Authority: "sha256:"
// over a canonical JSON document of its operations. Order does not matter;
// any change to a capability, connection, resource or call bound does.
func AuthorityHash(ops []AuthorityReport) string {
	ops = canonicalAuthority(ops)
	doc := struct {
		Format     string   `json:"format"`
		Operations []normOp `json:"operations"`
	}{Format: authorityFormat, Operations: []normOp{}}
	for _, op := range ops {
		doc.Operations = append(doc.Operations, normOp{op.Capability, op.Connection, op.Resources, op.MaxCallsPerRun})
	}
	data, err := canonicalBytes(doc)
	if err != nil {
		// Strings and integers always encode; a failure is a programming error.
		panic(fmt.Sprintf("workflow: canonical authority: %v", err))
	}
	return "sha256:" + digest(data)
}

// AuthorityOutside lists every requested operation, connection or resource
// that Workspace Access does not allow. Standing Authority must be a subset
// of current Workspace Access (ADR 0027); an empty result means it is.
func AuthorityOutside(ops []AuthorityReport, access WorkspaceAccess) []string {
	var out []string
	for _, op := range canonicalAuthority(ops) {
		conn, ok := access.connection(op.Connection)
		if !ok {
			out = append(out, fmt.Sprintf("%s on %s: connection %s is outside Workspace Access", op.Capability, op.Connection, op.Connection))
			continue
		}
		var missing []string
		for _, r := range op.Resources {
			if !conn.allows(r) {
				missing = append(missing, r)
			}
		}
		if len(missing) > 0 {
			out = append(out, fmt.Sprintf("%s on %s: resource %s is outside Workspace Access", op.Capability, op.Connection, strings.Join(missing, ", ")))
		}
	}
	return out
}

// NormalizeAccess returns Workspace Access in canonical form, connections
// and resources sorted, or an error naming the first malformed entry.
// Connection IDs and resource selectors are opaque, exact and non-empty,
// with no spaces or control characters; each connection lists at least one
// resource and nothing appears twice.
func NormalizeAccess(a WorkspaceAccess) (WorkspaceAccess, error) {
	out := WorkspaceAccess{Connections: []ConnectionAccess{}}
	seen := map[string]bool{}
	for _, c := range a.Connections {
		if !opaquePattern.MatchString(c.ID) {
			return WorkspaceAccess{}, fmt.Errorf("%q is not a Connection ID", c.ID)
		}
		if seen[c.ID] {
			return WorkspaceAccess{}, fmt.Errorf("connection %s is listed twice", c.ID)
		}
		seen[c.ID] = true
		if len(c.Resources) == 0 {
			return WorkspaceAccess{}, fmt.Errorf("connection %s lists no resources", c.ID)
		}
		resources := sortedStrings(c.Resources)
		for i, r := range resources {
			if !opaquePattern.MatchString(r) {
				return WorkspaceAccess{}, fmt.Errorf("%q on %s is not a resource selector", r, c.ID)
			}
			if i > 0 && resources[i-1] == r {
				return WorkspaceAccess{}, fmt.Errorf("resource %s on %s is listed twice", r, c.ID)
			}
		}
		out.Connections = append(out.Connections, ConnectionAccess{ID: c.ID, Resources: resources})
	}
	sort.Slice(out.Connections, func(i, j int) bool { return out.Connections[i].ID < out.Connections[j].ID })
	return out, nil
}

// ReviewDiff is what changed between the version a reviewer knows and a
// proposal: executable changes by report section, authority changes, and
// whether documentation (README or descriptions) changed. A proposal whose
// executable hash equals the known version's has no executable or
// authority change (authority is part of the hash).
type ReviewDiff struct {
	FromHash             string        `json:"from_hash,omitempty"`
	ToHash               string        `json:"to_hash"`
	ExecutableChanged    bool          `json:"executable_changed"`
	Executable           []SectionDiff `json:"executable"`
	Authority            []string      `json:"authority"`
	DocumentationChanged bool          `json:"documentation_changed"`
}

// SectionDiff lists one report section's removed and added lines.
type SectionDiff struct {
	Section string   `json:"section"`
	Removed []string `json:"removed,omitempty"`
	Added   []string `json:"added,omitempty"`
}

// DiffReports compares the known version from (nil for a new definition)
// with the proposal to.
func DiffReports(from *Report, to Report) ReviewDiff {
	d := ReviewDiff{ToHash: to.ExecutableHash, Executable: []SectionDiff{}, Authority: []string{}}
	var before []section
	var beforeAuthority []AuthorityReport
	if from != nil {
		d.FromHash = from.ExecutableHash
		before = reviewSections(*from)
		beforeAuthority = from.Authority
		d.DocumentationChanged = readmeDigest(*from) != readmeDigest(to) || from.Description != to.Description
	}
	d.ExecutableChanged = from == nil || from.ExecutableHash != to.ExecutableHash
	after := reviewSections(to)
	for i, s := range after {
		var old []string
		if before != nil {
			old = before[i].lines
		}
		removed, added := lineDiff(old, s.lines)
		if len(removed) > 0 || len(added) > 0 {
			d.Executable = append(d.Executable, SectionDiff{Section: s.title, Removed: removed, Added: added})
		}
	}
	d.Authority = authorityDiff(beforeAuthority, to.Authority)
	return d
}

// Text renders the diff for the terminal.
func (d ReviewDiff) Text() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	switch {
	case d.FromHash == "":
		line("New definition %s", d.ToHash)
	case !d.ExecutableChanged:
		line("No executable change: %s is the known version's hash", d.ToHash)
	default:
		line("Executable hash %s -> %s", d.FromHash, d.ToHash)
	}
	if d.DocumentationChanged {
		line("Documentation changed (README or descriptions; not part of the hash)")
	}
	if len(d.Executable) > 0 {
		line("Executable changes")
		for _, s := range d.Executable {
			line("  %s", s.Section)
			for _, l := range s.Removed {
				line("    - %s", l)
			}
			for _, l := range s.Added {
				line("    + %s", l)
			}
		}
	}
	if len(d.Authority) > 0 {
		line("Authority changes")
		for _, l := range d.Authority {
			line("  %s", l)
		}
	} else if d.ExecutableChanged {
		line("Authority changes: none")
	}
	return b.String()
}

func readmeDigest(r Report) string {
	for _, f := range r.Files {
		if !f.Hashed && f.Path == "README.md" {
			return f.Digest
		}
	}
	return ""
}

type section struct {
	title string
	lines []string
}

// reviewSections renders every behavior-affecting part of a report as
// lines, one section per report part, always in the same order. Requested
// authority is diffed separately; README, descriptions, warnings and derived
// worst-case bounds are left out.
func reviewSections(r Report) []section {
	var graph []string
	if r.Start != "" {
		graph = append(graph, "start: "+r.Start)
	}
	for _, g := range r.Graph {
		l := g.Line
		if g.Item != "" {
			l += "; " + g.Item
		}
		if len(g.Input) > 0 {
			l += "; input " + strings.Join(g.Input, ", ")
		}
		graph = append(graph, l)
	}
	var inputs, outputs, contracts, workers, functions, resources, retries, policies, files []string
	for _, in := range r.Inputs {
		inputs = append(inputs, in.Name+": "+in.Summary)
	}
	for _, o := range r.Outputs {
		when := "always"
		if !o.Always {
			when = "only when " + o.Step + " runs"
		}
		outputs = append(outputs, fmt.Sprintf("%s: %s (%s)", o.Name, o.From, when))
	}
	for _, c := range r.Capabilities {
		kind := "read"
		if c.Effect {
			kind = "effect"
			if c.Idempotent {
				kind += ", idempotent"
			}
		}
		contracts = append(contracts, fmt.Sprintf("%s %s: %s", c.ID, c.Contract, kind))
	}
	for _, w := range r.Workers {
		l := fmt.Sprintf("%s: %s worker, %s, %s / %s tokens each", w.Step, w.Worker, plural(w.Attempts, "attempt"), w.BudgetTime, grouped(w.BudgetTokens))
		if w.MaxItems > 0 {
			l += fmt.Sprintf(", up to %d items", w.MaxItems)
		}
		workers = append(workers, l)
	}
	for _, f := range r.Functions {
		functions = append(functions, fmt.Sprintf("%s (%s)", f.ID, strings.Join(f.Steps, ", ")))
	}
	for _, res := range r.Resources {
		resources = append(resources, fmt.Sprintf("%s %s: %s (steps: %s)", res.Connection, res.Resource, res.Access, strings.Join(res.Steps, ", ")))
	}
	for _, rp := range r.Retries {
		l := fmt.Sprintf("%s: %s", rp.Step, plural(rp.Attempts, "attempt"))
		if rp.Backoff != "" {
			l += ", backoff " + rp.Backoff
		}
		retries = append(retries, l)
	}
	for _, m := range r.ModelPolicies {
		l := fmt.Sprintf("%s v%d: %s %s", m.Name, m.Version, m.Provider, m.Model)
		if m.Temperature != nil {
			l += ", temperature " + formatNumber(*m.Temperature)
		}
		policies = append(policies, l+", max output "+grouped(m.MaxOutputTokens)+" tokens")
	}
	schedule := []string{"none (manual starts only)"}
	if s := r.Schedule; s != nil {
		catchUp := "skip missed runs"
		if s.CatchUp == "each_missed" {
			catchUp = fmt.Sprintf("catch up each missed run (at most %d)", s.MaxRuns)
		}
		schedule = []string{fmt.Sprintf("%s %s, %s, inputs %s", s.Cron, s.Timezone, catchUp, strings.Join(s.Inputs, ", "))}
	}
	retention := "classification " + r.Retention.Classification
	if len(r.Retention.SensitiveInputs) > 0 {
		retention += "; sensitive inputs: " + strings.Join(r.Retention.SensitiveInputs, ", ")
	}
	limits := fmt.Sprintf("max duration %s, %d attempts, %d at once, %s tokens", r.Limits.MaxDuration, r.Limits.MaxAttempts, r.Limits.MaxParallel, grouped(r.Limits.MaxTokens))
	for _, f := range r.Files {
		if f.Hashed {
			files = append(files, f.Path+" "+f.Digest)
		}
	}
	return []section{
		{"Workflow", []string{fmt.Sprintf("%s (schema %d)", r.Workflow, r.SchemaVersion)}},
		{"Graph", graph},
		{"Inputs", inputs},
		{"Outputs", outputs},
		{"Run key", []string{strings.Join(r.RunKey, ", ")}},
		{"Capability contracts", contracts},
		{"Workers", workers},
		{"Deterministic functions", functions},
		{"Resources", resources},
		{"Retry policies", retries},
		{"Model policies", policies},
		{"Schedule", schedule},
		{"Retention", []string{retention}},
		{"Limits", []string{limits}},
		{"Prompts and schemas", files},
	}
}

// lineDiff returns the lines only in before and only in after, each in its
// own order.
func lineDiff(before, after []string) (removed, added []string) {
	in := func(list []string, l string) bool { return slices.Contains(list, l) }
	for _, l := range before {
		if !in(after, l) {
			removed = append(removed, l)
		}
	}
	for _, l := range after {
		if !in(before, l) {
			added = append(added, l)
		}
	}
	return removed, added
}

func authorityLine(op AuthorityReport) string {
	return fmt.Sprintf("%s on %s: %s, at most %s per run", op.Capability, op.Connection, strings.Join(op.Resources, ", "), plural(op.MaxCallsPerRun, "call"))
}

// authorityDiff lists requested operations added (+), removed (-) and
// changed (~), keyed by capability and connection.
func authorityDiff(before, after []AuthorityReport) []string {
	key := func(op AuthorityReport) string { return op.Capability + "\x00" + op.Connection }
	old := map[string]AuthorityReport{}
	for _, op := range canonicalAuthority(before) {
		old[key(op)] = op
	}
	out := []string{}
	seen := map[string]bool{}
	for _, op := range canonicalAuthority(after) {
		k := key(op)
		seen[k] = true
		prev, ok := old[k]
		if !ok {
			out = append(out, "+ "+authorityLine(op))
			continue
		}
		var changes []string
		removed, added := lineDiff(prev.Resources, op.Resources)
		for _, r := range added {
			changes = append(changes, "+"+r)
		}
		for _, r := range removed {
			changes = append(changes, "-"+r)
		}
		if prev.MaxCallsPerRun != op.MaxCallsPerRun {
			changes = append(changes, fmt.Sprintf("max calls per run %d -> %d", prev.MaxCallsPerRun, op.MaxCallsPerRun))
		}
		if len(changes) > 0 {
			out = append(out, fmt.Sprintf("~ %s on %s: %s", op.Capability, op.Connection, strings.Join(changes, "; ")))
		}
	}
	for _, op := range canonicalAuthority(before) {
		if !seen[key(op)] {
			out = append(out, "- "+authorityLine(op))
		}
	}
	return out
}
