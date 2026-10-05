package workflow

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Migration and retention support (Stage 14 sub-stage 12, 2026-10-04;
// ADR 0025, 0030, 0045). A Successor Run continues an unfinished run's
// state under a newer approved version only when that state is valid
// there: PlanResume decides it from the definition alone, and Sensitivity
// says what a version classifies as sensitive for masking and redaction.
// Nothing here stores, runs or redacts anything.

// ResumePlan is how a Successor Run continues a predecessor's state under
// this version: the step it resumes at, its accepted input under this
// version, the recorded step outputs it carries, and what it leaves out.
type ResumePlan struct {
	Resume string `json:"resume"`
	RunKey string `json:"run_key"`
	// Inputs are the predecessor's inputs accepted under this version
	// (defaults applied); State the carried step outputs, by step ID.
	Inputs map[string]any `json:"-"`
	State  map[string]any `json:"-"`
	// Carried names the carried steps; Dropped what is not carried and
	// why (an input or step this version does not have, or a step output
	// no later step reads and that no longer matches this version).
	Carried []string     `json:"carried"`
	Dropped []ResumeDrop `json:"dropped,omitempty"`
}

// ResumeDrop is one input or step output a successor does not carry.
type ResumeDrop struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ResumeError refuses a migration with every reason, each at the
// definition path it concerns: the precise difference between the
// predecessor's state and this version's reads.
type ResumeError struct {
	Problems []Problem `json:"problems"`
}

func (e *ResumeError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.Path + ": " + p.Message
	}
	return "the run's state does not satisfy this version:\n" + strings.Join(lines, "\n")
}

// stepRead is one reference to a step output that a step from the resume
// point on (or a run output) reads.
type stepRead struct {
	reader string
	ref    Ref
	// optional reads (run outputs) may name a step that never ran.
	optional bool
}

// PlanResume checks whether a run that stopped before step resume, with
// the accepted inputs and recorded step outputs given, can continue under
// this version, and returns what its successor carries. logicalKey is the
// run's logical Run Key: this version must derive the same one from the
// inputs, since a successor continues the same logical execution (ADR
// 0021, 0030). Every step this version runs from the resume point on must
// find, in the recorded outputs, the fields it reads with the types it
// expects; a recorded step this version would run again after the resume
// point is refused, so nothing completed is repeated. Inputs and steps
// this version does not declare are dropped, never carried.
func (v *Validated) PlanResume(resume string, inputs map[string]any, state map[string]any, logicalKey string) (ResumePlan, error) {
	plan := ResumePlan{Resume: resume, State: map[string]any{}, Carried: []string{}}
	var problems []Problem
	add := func(path, format string, args ...any) {
		problems = append(problems, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
	}
	start, ok := v.steps[resume]
	if !ok {
		add("resume", "this version has no step %s, where the run would continue", resume)
		return plan, &ResumeError{Problems: problems}
	}
	// The steps the successor runs: the resume point and everything after.
	after := map[string]bool{resume: true}
	queue := []string{start.id}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, next := range v.steps[id].next {
			if !after[next] {
				after[next] = true
				queue = append(queue, next)
			}
		}
	}

	declared := map[string]json.RawMessage{}
	for _, name := range sortedKeys(inputs) {
		if _, ok := v.inputs[name]; !ok {
			plan.Dropped = append(plan.Dropped, ResumeDrop{Name: "inputs." + name, Reason: "this version does not declare it"})
			continue
		}
		data, err := canonicalBytes(inputs[name])
		if err != nil {
			add("inputs."+name, "the recorded value cannot be read: %v", err)
			continue
		}
		declared[name] = data
	}
	accepted, err := v.AcceptInputs(declared, nil)
	if err != nil {
		add("inputs", "%v", err)
	} else {
		plan.Inputs = accepted
		if plan.RunKey = v.RunKey(accepted); plan.RunKey != logicalKey {
			add("run_key", "this version derives Run Key %s from the run's input, not %s; a successor continues the same logical execution", plan.RunKey, logicalKey)
		}
	}

	candidates := map[string]bool{}
	for _, id := range sortedKeys(state) {
		switch {
		case v.steps[id] == nil:
			plan.Dropped = append(plan.Dropped, ResumeDrop{Name: "steps." + id, Reason: "this version has no step " + id})
		case after[id]:
			add("steps."+id, "the run already ran it, but this version runs it again after the resume point %s; a successor would repeat it", resume)
		default:
			candidates[id] = true
		}
	}

	read := map[string]bool{}
	for _, r := range v.readsAfter(after) {
		if after[r.ref.Name] {
			continue
		}
		read[r.ref.Name] = true
		value, ran := state[r.ref.Name]
		if !ran {
			if !r.optional {
				add(r.reader, "reads %s, but the run has no recorded output of steps.%s: this version runs it before the resume point %s", r.ref.Raw, r.ref.Name, resume)
			}
			continue
		}
		if !candidates[r.ref.Name] {
			continue
		}
		if err := checkRead(v.steps[r.ref.Name].output, value, r.ref.Path); err != nil {
			add(r.reader, "reads %s, which the run's recorded output of steps.%s does not satisfy: %v", r.ref.Raw, r.ref.Name, err)
		}
	}
	for _, id := range sortedKeys(candidates) {
		if !read[id] {
			// No step after the resume point reads it: it is carried only
			// while it still matches this version's output of the step.
			if err := checkValue(v.steps[id].output, state[id]); err != nil {
				plan.Dropped = append(plan.Dropped, ResumeDrop{Name: "steps." + id, Reason: fmt.Sprintf("no step after the resume point reads it, and it does not match this version's output: %v", err)})
				continue
			}
		}
		plan.State[id] = state[id]
		plan.Carried = append(plan.Carried, id)
	}
	if len(problems) > 0 {
		return plan, &ResumeError{Problems: problems}
	}
	return plan, nil
}

// readsAfter lists every step output read by a step in after (its input,
// a map's list and its item's input, a branch's cases) and by the run's
// outputs, in a deterministic order.
func (v *Validated) readsAfter(after map[string]bool) []stepRead {
	var out []stepRead
	argReads := func(n *node, path string) {
		for _, name := range sortedKeys(n.args) {
			if a := n.args[name]; !a.isConst && a.ref != nil && a.ref.Root == refSteps {
				out = append(out, stepRead{reader: path + ".input." + name, ref: *a.ref})
			}
		}
	}
	for _, id := range v.order {
		n := v.steps[id]
		if n == nil || !after[id] {
			continue
		}
		argReads(n, n.path)
		if n.over != nil && n.over.Root == refSteps {
			out = append(out, stepRead{reader: n.path + ".over", ref: *n.over})
		}
		if n.item != nil {
			argReads(n.item, n.path+".item")
		}
		for i, c := range n.cases {
			if c.ref.Root == refSteps {
				out = append(out, stepRead{reader: fmt.Sprintf("%s.cases[%d]", n.path, i), ref: c.ref})
			}
		}
	}
	for _, name := range sortedKeys(v.definition.Outputs) {
		r, err := parseRef(v.definition.Outputs[name])
		if err == nil && r.Root == refSteps {
			out = append(out, stepRead{reader: "outputs." + name, ref: r, optional: true})
		}
	}
	return out
}

// checkRead checks one field path of a recorded step output against the
// type this version reads it as. A field the type marks optional may be
// absent, as it reads at run time.
func checkRead(t *Type, value any, path []string) error {
	for i, name := range path {
		where := strings.Join(path[:i+1], ".")
		if t.isAny() {
			return nil
		}
		next, ok := t.field(name)
		if !ok {
			return fmt.Errorf("this version's output has no field %s", where)
		}
		m, isObject := value.(map[string]any)
		if !isObject {
			return fmt.Errorf("%s: expected an object holding it, got %s", where, valueKind(value))
		}
		field, present := m[name]
		if !present {
			if !t.requires(name) {
				// An absent optional field reads as nothing, as it does
				// at run time.
				return nil
			}
			return fmt.Errorf("missing field %s", where)
		}
		t, value = next, field
	}
	if err := checkValue(t, value); err != nil {
		where := strings.Join(path, ".")
		if where == "" {
			return err
		}
		return fmt.Errorf("%s: %v", where, strings.TrimPrefix(err.Error(), "value: "))
	}
	return nil
}

// Sensitivity is what a version classifies as sensitive in its runs'
// records (ADR 0045): its retention classification, the inputs it marks
// sensitive, and where those inputs reach a step's resolved input (by step
// ID; ItemArgs for a map step's items), a branch's recorded reads (by the
// reference text) and the run's outputs. Under the "sensitive"
// classification every value a run records is sensitive.
type Sensitivity struct {
	Classification string              `json:"classification"`
	Inputs         []string            `json:"inputs,omitempty"`
	StepArgs       map[string][]string `json:"step_args,omitempty"`
	ItemArgs       map[string][]string `json:"item_args,omitempty"`
	BranchReads    map[string][]string `json:"branch_reads,omitempty"`
	Outputs        []string            `json:"outputs,omitempty"`
}

// Retention classifications.
const (
	ClassificationOrdinary  = "ordinary"
	ClassificationSensitive = "sensitive"
)

// Sensitivity is this version's sensitivity.
func (v *Validated) Sensitivity() Sensitivity { return sensitivityOf(v.definition) }

// ParseSensitivity reads a definition's sensitivity from its workflow.yaml
// without validating it against an environment, so the sensitivity of a
// version recorded long ago is known even when its capabilities or the
// Workspace's access have changed since.
func ParseSensitivity(src Source) (Sensitivity, error) {
	data, err := src.ReadFile("workflow.yaml")
	if err != nil {
		return Sensitivity{}, err
	}
	def, problems := parseDefinition(data)
	if len(problems) > 0 {
		return Sensitivity{}, &InvalidError{Problems: problems}
	}
	return sensitivityOf(def), nil
}

func sensitivityOf(def *Definition) Sensitivity {
	s := Sensitivity{Classification: ClassificationOrdinary}
	if def.Retention != nil && def.Retention.Classification != "" {
		s.Classification = def.Retention.Classification
	}
	sensitive := map[string]bool{}
	for _, name := range sortedKeys(def.Inputs) {
		if def.Inputs[name].Sensitive {
			sensitive[name] = true
			s.Inputs = append(s.Inputs, name)
		}
	}
	if len(sensitive) == 0 {
		return s
	}
	reads := func(text string) bool {
		r, err := parseRef(text)
		return err == nil && r.Root == refInputs && sensitive[r.Name]
	}
	args := func(step *Step) []string {
		var out []string
		for _, name := range sortedKeys(step.Input) {
			node := step.Input[name]
			if node.Kind == yaml.ScalarNode && reads(node.Value) {
				out = append(out, name)
			}
		}
		return out
	}
	add := func(m *map[string][]string, id string, names []string) {
		if len(names) == 0 {
			return
		}
		if *m == nil {
			*m = map[string][]string{}
		}
		(*m)[id] = names
	}
	for i := range def.Steps {
		step := &def.Steps[i]
		add(&s.StepArgs, step.ID, args(step))
		if step.Item != nil {
			add(&s.ItemArgs, step.ID, args(step.Item))
		}
		var branch []string
		for _, c := range step.Cases {
			if c.When != nil && reads(c.When.Ref) && !slices.Contains(branch, c.When.Ref) {
				branch = append(branch, c.When.Ref)
			}
		}
		add(&s.BranchReads, step.ID, branch)
	}
	for _, name := range sortedKeys(def.Outputs) {
		if reads(def.Outputs[name]) {
			s.Outputs = append(s.Outputs, name)
		}
	}
	return s
}
