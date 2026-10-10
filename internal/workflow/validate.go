package workflow

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // schedules validate against the same IANA database on every host

	"gopkg.in/yaml.v3"
)

// Problem is one reason a definition is invalid, at a definition path such
// as steps.research.max_parallel.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// InvalidError lists every problem found, in a deterministic order.
type InvalidError struct {
	Problems []Problem
}

func (e *InvalidError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.Path + ": " + p.Message
	}
	return "invalid Workflow Definition:\n" + strings.Join(lines, "\n")
}

// Validated is a definition that passed every rule, with its executable hash
// and review report. It is what activation (sub-stage 2) will approve.
type Validated struct {
	definition *Definition
	source     []byte
	readme     []byte
	files      map[string][]byte
	steps      map[string]*node
	order      []string
	inputs     map[string]*inputInfo
	limits     runLimits
	hash       string
	report     Report
}

// Hash is the executable hash: "sha256:" and 64 hex digits.
func (v *Validated) Hash() string { return v.hash }

// Report is the review report.
func (v *Validated) Report() Report { return v.report }

type runLimits struct {
	duration time.Duration
	attempts int
	parallel int
	tokens   int64
}

type inputInfo struct {
	name       string
	spec       InputSpec
	t          *Type
	def        any
	hasDefault bool
}

// node is one compiled step, or a map step's item.
type node struct {
	step     *Step
	id       string
	path     string
	kind     string
	isItem   bool
	attempts int
	backoff  time.Duration
	args     map[string]arg
	output   *Type
	fn       *Function
	cap      *CapabilityContract
	schema   *Type
	budget   time.Duration
	tokens   int64
	item     *node
	over     *Ref
	elem     *Type
	cases    []compiledCase
	next     []string
	known    bool
}

type arg struct {
	ref     *Ref
	literal any
	isConst bool
	t       *Type
}

type compiledCase struct {
	ref      Ref
	op       string
	value    any
	hasValue bool
	next     string
}

var (
	namePattern       = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	semverPattern     = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	modelPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._:-]*$`)
	opaquePattern     = regexp.MustCompile(`^[^\s\x00-\x1f\x7f]{1,256}$`)
)

var workers = []string{WorkerResearch, WorkerExplore, WorkerBuild}

var conditionOps = []string{"eq", "ne", "gt", "gte", "lt", "lte", "empty", "not_empty"}

// fieldsByKind are the fields each step kind accepts besides id, kind and
// description; a map item accepts the same without id and next.
var fieldsByKind = map[string][]string{
	KindDeterministic: {"next", "retry", "input", "function"},
	KindCapability:    {"next", "retry", "input", "capability", "contract", "connection", "resource"},
	KindAI:            {"next", "retry", "input", "model_policy", "prompt", "output_schema"},
	KindAgent:         {"next", "retry", "input", "worker", "model_policy", "objective", "output_schema", "budget"},
	KindMap:           {"next", "over", "max_items", "max_parallel", "on_item_failure", "min_succeeded", "item"},
	KindBranch:        {"cases", "default"},
	KindInterrupt:     {"next", "input", "message", "response_schema"},
}

var itemKinds = []string{KindDeterministic, KindAI, KindAgent}

type validator struct {
	src          Source
	readme       []byte
	env          Environment
	def          *Definition
	problems     []Problem
	warnings     []string
	files        map[string][]byte
	schemas      map[string]*Type
	schemaErrs   map[string]string
	functions    map[string]Function
	inputs       map[string]*inputInfo
	limits       runLimits
	limitsOK     bool
	steps        map[string]*node
	list         []*node
	preds        map[string][]string
	reachable    map[string]bool
	order        []string
	acyclic      bool
	dom          map[string]map[string]bool
	usedPolicies map[string]bool
	usedInputs   map[string]bool
	outputs      []outputInfo
	worstTokens  int64
	worstTries   int
}

type outputInfo struct {
	name   string
	ref    Ref
	always bool
}

// Validate reads and validates one definition against the environment.
// Invalid definitions return an *InvalidError listing every problem.
func Validate(src Source, env Environment) (*Validated, error) {
	if env.Workers.Deadline <= 0 || env.Workers.Tokens <= 0 {
		return nil, errors.New("workflow validation needs the operator's worker ceiling (deadline and token budget)")
	}
	if env.Capabilities == nil {
		env.Capabilities = NewCatalog()
	}
	data, err := src.ReadFile("workflow.yaml")
	if err != nil {
		msg := err.Error()
		if isNotExist(err) {
			msg = "not found"
		}
		return nil, &InvalidError{Problems: []Problem{{Path: "workflow.yaml", Message: msg}}}
	}
	def, problems := parseDefinition(data)
	if len(problems) > 0 {
		return nil, &InvalidError{Problems: problems}
	}
	v := &validator{
		src: src, env: env, def: def,
		files: map[string][]byte{}, schemas: map[string]*Type{}, schemaErrs: map[string]string{},
		functions: reviewedFunctions(), inputs: map[string]*inputInfo{}, steps: map[string]*node{},
		usedPolicies: map[string]bool{}, usedInputs: map[string]bool{},
	}
	v.run()
	if len(v.problems) > 0 {
		return nil, &InvalidError{Problems: v.problems}
	}
	result := &Validated{
		definition: def, source: data, readme: v.readme, files: v.files, steps: v.steps, order: v.order,
		inputs: v.inputs, limits: v.limits,
	}
	if result.hash, err = executableHash(def, v.files, v.functions); err != nil {
		return nil, err
	}
	result.report = v.buildReport(result.hash)
	return result, nil
}

func (v *validator) add(path, format string, args ...any) {
	v.problems = append(v.problems, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
}

func (v *validator) run() {
	if readme, err := v.src.ReadFile("README.md"); err != nil {
		v.add("README.md", "required beside workflow.yaml to explain the workflow to its reviewer")
	} else {
		// An empty README is present: its digest is the empty digest on
		// every Source.
		v.readme = append([]byte{}, readme...)
	}
	v.checkHeader()
	v.checkLimits()
	v.checkInputs()
	v.checkPolicies()
	v.collectSteps()
	v.checkGraph()
	if v.acyclic {
		for _, id := range v.order {
			if n := v.steps[id]; v.reachable[id] && n.known {
				v.resolve(n, n, nil)
			}
		}
	}
	v.checkOutputs()
	v.checkAuthority()
	v.checkRunKey()
	v.checkSchedule()
	v.checkUsage()
}

func (v *validator) checkHeader() {
	switch v.def.SchemaVersion {
	case 0:
		v.add("schema_version", "required; this Evie reads schema_version %d", SchemaVersion)
	case SchemaVersion:
	default:
		v.add("schema_version", "unsupported schema_version %d; this Evie reads %d", v.def.SchemaVersion, SchemaVersion)
	}
	if !namePattern.MatchString(v.def.Name) {
		v.add("name", "%q must be 1 to 64 lowercase letters, digits and hyphens, starting with a letter", v.def.Name)
	}
	switch r := v.def.Retention; {
	case r == nil:
		v.add("retention", "required: classification ordinary or sensitive")
	case r.Classification != "ordinary" && r.Classification != "sensitive":
		v.add("retention.classification", "unknown classification %s; use ordinary or sensitive", r.Classification)
	}
}

func parsePositiveDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%q is not a positive duration such as 90s, 10m or 2h", s)
	}
	return d, nil
}

func (v *validator) checkLimits() {
	l := v.def.Limits
	if l == nil {
		v.add("limits", "required: max_duration, max_attempts, max_parallel and max_tokens")
		return
	}
	ok := true
	if l.MaxDuration == "" {
		v.add("limits.max_duration", "required")
		ok = false
	} else if d, err := parsePositiveDuration(l.MaxDuration); err != nil {
		v.add("limits.max_duration", "%v", err)
		ok = false
	} else if d > maxRunDuration {
		v.add("limits.max_duration", "%s is above %s", formatDuration(d), formatDuration(maxRunDuration))
		ok = false
	} else {
		v.limits.duration = d
	}
	check := func(field string, value, limit int64) bool {
		switch {
		case value == 0:
			v.add("limits."+field, "required")
		case value < 0:
			v.add("limits."+field, "must be positive")
		case value > limit:
			v.add("limits."+field, "%s is above %s", grouped(value), grouped(limit))
		default:
			return true
		}
		return false
	}
	ok = check("max_attempts", int64(l.MaxAttempts), maxRunAttempts) && ok
	ok = check("max_parallel", int64(l.MaxParallel), maxRunParallel) && ok
	ok = check("max_tokens", l.MaxTokens, maxRunTokens) && ok
	v.limits.attempts, v.limits.parallel, v.limits.tokens = l.MaxAttempts, l.MaxParallel, l.MaxTokens
	v.limitsOK = ok
}

func (v *validator) checkInputs() {
	for _, name := range sortedKeys(v.def.Inputs) {
		spec := v.def.Inputs[name]
		p := "inputs." + name
		if !identifierPattern.MatchString(name) {
			v.add(p, "%q is not a valid input name; use lowercase letters, digits and _", name)
			continue
		}
		var t *Type
		switch spec.Type {
		case "string":
			t = StringType()
		case "integer":
			t = IntegerType()
		case "number":
			t = NumberType()
		case "boolean":
			t = BooleanType()
		case "":
			v.add(p+".type", "required: string, integer, number or boolean")
			continue
		default:
			v.add(p+".type", "unknown type %s; use string, integer, number or boolean", spec.Type)
			continue
		}
		if spec.Required && spec.Default.Kind != 0 {
			v.add(p, "an input is either required or has a default, not both")
		}
		if spec.MaxLength != nil {
			if t.Kind != KindString {
				v.add(p+".max_length", "applies only to string inputs")
			} else if *spec.MaxLength < 1 {
				v.add(p+".max_length", "must be positive")
			}
			t.MaxLength = spec.MaxLength
		}
		for _, b := range []struct {
			field string
			bound *float64
		}{{"minimum", spec.Minimum}, {"maximum", spec.Maximum}} {
			if b.bound != nil && t.Kind != KindInteger && t.Kind != KindNumber {
				v.add(p+"."+b.field, "applies only to integer and number inputs")
			}
		}
		t.Minimum, t.Maximum = spec.Minimum, spec.Maximum
		if t.Minimum != nil && t.Maximum != nil && *t.Minimum > *t.Maximum {
			v.add(p, "minimum is above maximum")
		}
		for i := range spec.Enum {
			e, err := nodeValue(&spec.Enum[i])
			if err == nil {
				err = checkValue(&Type{Kind: t.Kind}, e)
			}
			if err != nil {
				v.add(fmt.Sprintf("%s.enum[%d]", p, i), "%v", err)
				continue
			}
			t.Enum = append(t.Enum, e)
		}
		info := &inputInfo{name: name, spec: spec, t: t}
		if spec.Default.Kind != 0 {
			d, err := nodeValue(&spec.Default)
			if err == nil {
				err = checkValue(t, d)
			}
			if err != nil {
				v.add(p+".default", "%v", err)
			}
			info.def, info.hasDefault = d, true
		}
		v.inputs[name] = info
	}
}

func (v *validator) checkPolicies() {
	for _, name := range sortedKeys(v.def.ModelPolicies) {
		m := v.def.ModelPolicies[name]
		p := "model_policies." + name
		if !identifierPattern.MatchString(name) {
			v.add(p, "%q is not a valid policy name; use lowercase letters, digits and _", name)
		}
		if m.Version < 1 {
			v.add(p+".version", "required: 1 or more, raised whenever the policy changes")
		}
		switch m.Provider {
		case "":
			v.add(p+".provider", "required")
		case "openrouter":
		default:
			v.add(p+".provider", "unknown provider %s; this Evie supports openrouter", m.Provider)
		}
		switch {
		case m.Model == "":
			v.add(p+".model", "required")
		case !modelPattern.MatchString(m.Model):
			v.add(p+".model", "%q is not an exact model id such as vendor/model-1.2", m.Model)
		case strings.HasSuffix(strings.ToLower(m.Model), "latest"):
			v.add(p+".model", "%s is a floating alias; pin an exact model version", m.Model)
		case strings.HasPrefix(strings.ToLower(m.Model), "openrouter/"):
			// Sub-stage 7: OpenRouter's routers choose a model per request.
			v.add(p+".model", "%s routes each request to a model of its choosing; name an exact model", m.Model)
		case strings.HasSuffix(strings.ToLower(m.Model), ":batch"):
			v.add(p+".model", "%s needs OpenRouter's asynchronous Batch API, which AI steps do not use; name an exact model without :batch", m.Model)
		}
		if m.Temperature != nil && (*m.Temperature < 0 || *m.Temperature > 2) {
			v.add(p+".temperature", "%s is outside 0 to 2", formatNumber(*m.Temperature))
		}
		switch {
		case m.MaxOutputTokens == 0:
			v.add(p+".max_output_tokens", "required")
		case m.MaxOutputTokens < 0 || m.MaxOutputTokens > maxOutputTokens:
			v.add(p+".max_output_tokens", "must be 1 to %s", grouped(maxOutputTokens))
		case v.limitsOK && m.MaxOutputTokens > v.limits.tokens:
			v.add(p+".max_output_tokens", "%s is above limits.max_tokens %s", grouped(m.MaxOutputTokens), grouped(v.limits.tokens))
		}
	}
}

func (v *validator) collectSteps() {
	if len(v.def.Steps) == 0 {
		v.add("steps", "a workflow needs at least one step")
	}
	for i := range v.def.Steps {
		s := &v.def.Steps[i]
		switch {
		case s.ID == "":
			v.add(fmt.Sprintf("steps[%d].id", i), "required")
			continue
		case !identifierPattern.MatchString(s.ID):
			v.add(fmt.Sprintf("steps[%d].id", i), "%s is not a valid step id; use lowercase letters, digits and _", s.ID)
			continue
		}
		p := "steps." + s.ID
		if _, dup := v.steps[s.ID]; dup {
			v.add(p, "duplicate step id %s", s.ID)
			continue
		}
		n := v.compile(s, p, false)
		v.steps[s.ID] = n
		v.list = append(v.list, n)
	}
}

// compile checks one step or item on its own: kind, fields, files,
// registry, catalog, access and bounds. References and types need the graph
// and are resolved later.
func (v *validator) compile(s *Step, p string, isItem bool) *node {
	n := &node{step: s, id: s.ID, path: p, kind: s.Kind, isItem: isItem, attempts: 1}
	allowed, known := fieldsByKind[s.Kind]
	switch {
	case s.Kind == "":
		v.add(p+".kind", "required")
		return n
	case !known:
		v.add(p+".kind", "unknown step kind %s; use deterministic, capability, ai, agent, map, branch or interrupt", s.Kind)
		return n
	case isItem && !slices.Contains(itemKinds, s.Kind):
		v.add(p+".kind", "an item cannot be a %s; use deterministic, ai or agent", s.Kind)
		return n
	}
	where := s.Kind + " steps"
	accepted := append([]string{"kind", "description"}, allowed...)
	if isItem {
		where = "map items"
		accepted = slices.DeleteFunc(accepted, func(f string) bool { return f == "next" })
	} else {
		accepted = append(accepted, "id")
	}
	for _, key := range sortedKeys(s.keys) {
		if !slices.Contains(accepted, key) {
			v.add(p, "field %s does not apply to %s", key, where)
		}
	}
	n.known = true
	n.attempts, n.backoff = v.retryPolicy(s.Retry, p)
	switch s.Kind {
	case KindDeterministic:
		v.compileFunction(n)
	case KindCapability:
		v.compileCapability(n)
	case KindAI:
		v.usePolicy(s.ModelPolicy, p+".model_policy")
		v.loadPrompt(s.Prompt, p+".prompt")
		if s.OutputSchema == "" {
			v.add(p+".output_schema", "required: AI steps return typed proposals")
		} else {
			n.schema = v.loadSchema(s.OutputSchema, p+".output_schema")
		}
		n.output = n.schema
	case KindAgent:
		v.compileAgent(n)
	case KindMap:
		v.compileMap(n)
	case KindBranch:
		v.compileBranch(n)
	case KindInterrupt:
		switch {
		case s.Message == "":
			v.add(p+".message", "required: what the owner is asked")
		case len(s.Message) > maxMessageBytes:
			v.add(p+".message", "is longer than %d bytes", maxMessageBytes)
		}
		if s.ResponseSchema == "" {
			v.add(p+".response_schema", "required: the typed answer the owner gives")
		} else {
			n.schema = v.loadSchema(s.ResponseSchema, p+".response_schema")
		}
		n.output = n.schema
	}
	return n
}

func (v *validator) retryPolicy(r *Retry, p string) (int, time.Duration) {
	if r == nil {
		return 1, 0
	}
	attempts := r.MaxAttempts
	switch {
	case attempts == 0:
		v.add(p+".retry.max_attempts", "required")
		attempts = 1
	case attempts < 1 || attempts > maxStepAttempts:
		v.add(p+".retry.max_attempts", "%d is outside 1 to %d", attempts, maxStepAttempts)
		attempts = 1
	}
	var backoff time.Duration
	if r.Backoff != "" {
		d, err := parsePositiveDuration(r.Backoff)
		switch {
		case err != nil:
			v.add(p+".retry.backoff", "%v", err)
		case d > maxBackoff:
			v.add(p+".retry.backoff", "%s is above %s", formatDuration(d), formatDuration(maxBackoff))
		case attempts == 1:
			v.add(p+".retry.backoff", "a backoff needs max_attempts above 1")
		default:
			backoff = d
		}
	}
	return attempts, backoff
}

func (v *validator) usePolicy(name, p string) {
	switch {
	case name == "":
		v.add(p, "required: a named Model Policy")
	case !hasKey(v.def.ModelPolicies, name):
		v.add(p, "unknown model policy %s", name)
	default:
		v.usedPolicies[name] = true
	}
}

func hasKey[V any](m map[string]V, k string) bool {
	_, ok := m[k]
	return ok
}

func (v *validator) loadFile(file, p, ext, role string, limit int) ([]byte, bool) {
	parts := strings.Split(file, "/")
	switch {
	case strings.Contains(file, `\`):
		v.add(p, "%s: separate folders with /", file)
	case strings.HasPrefix(file, "/") || (len(file) > 1 && file[1] == ':'):
		v.add(p, "%s must be relative to the definition", file)
	case path.Clean(file) != file || slices.Contains(parts, ".."):
		v.add(p, "%s must be a clean path inside the definition", file)
	case !strings.HasSuffix(file, ext):
		v.add(p, "%s: %s must be %s files", file, role, ext)
	case file == "README.md" || file == "workflow.yaml":
		v.add(p, "%s is not a %s file", file, role)
	default:
		if data, ok := v.files[file]; ok {
			return data, true
		}
		data, err := v.src.ReadFile(file)
		switch {
		case err != nil && isNotExist(err):
			v.add(p, "%s not found", file)
		case err != nil && strings.Contains(err.Error(), "escapes"):
			v.add(p, "%s must be a clean path inside the definition", file)
		case err != nil:
			v.add(p, "%s: %v", file, err)
		case len(data) > limit:
			v.add(p, "%s is larger than %d KiB", file, limit>>10)
		default:
			v.files[file] = data
			return data, true
		}
	}
	return nil, false
}

func (v *validator) loadPrompt(file, p string) {
	if file == "" {
		v.add(p, "required: a Markdown prompt file")
		return
	}
	v.loadFile(file, p, ".md", "prompts", maxPromptBytes)
}

// loadSchema reads one schema, whose root must be an object.
func (v *validator) loadSchema(file, p string) *Type {
	if msg, failed := v.schemaErrs[file]; failed {
		v.add(p, "%s", msg)
		return nil
	}
	if t, ok := v.schemas[file]; ok {
		return t
	}
	data, ok := v.loadFile(file, p, ".json", "schemas", maxSchemaBytes)
	if !ok {
		return nil
	}
	t, _, err := parseSchema(data)
	if err == nil && t.Kind != KindObject {
		err = errors.New("the root must be an object")
	}
	if err != nil {
		v.schemaErrs[file] = fmt.Sprintf("%s: %v", file, err)
		delete(v.files, file)
		v.add(p, "%s", v.schemaErrs[file])
		return nil
	}
	v.schemas[file] = t
	return t
}

func (v *validator) compileFunction(n *node) {
	id, p := n.step.Function, n.path+".function"
	switch f, ok := v.functions[id]; {
	case id == "":
		v.add(p, "required: a reviewed function as name@version")
	case !strings.Contains(id, "@"):
		v.add(p, "name a function with its version, such as %s@1", id)
	case !ok:
		v.add(p, "%s is not in the reviewed registry (%s)", id, strings.Join(sortedKeys(v.functions), ", "))
	default:
		n.fn = &f
		n.output = f.Result
	}
}

func (v *validator) compileCapability(n *node) {
	s, p := n.step, n.path
	for _, f := range [][2]string{{"capability", s.Capability}, {"contract", s.Contract}, {"connection", s.Connection}, {"resource", s.Resource}} {
		if f[1] == "" {
			v.add(p+"."+f[0], "required")
		}
	}
	if s.Capability == "" {
		return
	}
	if strings.HasPrefix(s.Capability, "subagents.") {
		v.add(p+".capability", "workflows cannot call %s: agent work uses agent steps, and a workflow cannot merge a build branch in v1", s.Capability)
		return
	}
	if !capabilityPattern.MatchString(s.Capability) {
		v.add(p+".capability", "%q is not a canonical Capability ID such as plugin.operation", s.Capability)
		return
	}
	c, ok := v.env.Capabilities.Capability(s.Capability)
	if !ok {
		v.add(p+".capability", "%s is not in the capability catalog", s.Capability)
		return
	}
	if s.Contract != "" && s.Contract != c.Contract {
		v.add(p+".contract", "%s offers contract %s, not %s", s.Capability, c.Contract, s.Contract)
	}
	n.cap = &c
	n.output = c.Output
	if n.output == nil {
		n.output = ObjectType()
	}
	if len(c.Connections) > 0 && s.Connection != "" && !slices.Contains(c.Connections, s.Connection) {
		v.add(p+".connection", "%s is served on connection %s, not %s", s.Capability, strings.Join(c.Connections, ", "), s.Connection)
	}
	if len(c.Resources) > 0 && s.Resource != "" && !slices.Contains(c.Resources, s.Resource) {
		v.add(p+".resource", "%s serves resource %s, not %s", s.Capability, strings.Join(c.Resources, ", "), s.Resource)
	}
	if c.ResourcePrefix != "" && s.Resource != "" && (!strings.HasPrefix(s.Resource, c.ResourcePrefix) || s.Resource == c.ResourcePrefix) {
		v.add(p+".resource", "%s serves resources %s<name>, not %s", s.Capability, c.ResourcePrefix, s.Resource)
	}
	if s.Connection != "" && !v.env.AccessUnchecked {
		conn, ok := v.env.Access.connection(s.Connection)
		switch {
		case !ok:
			v.add(p+".connection", "connection %s is outside Workspace Access", s.Connection)
		case s.Resource != "" && !conn.allows(s.Resource):
			v.add(p+".resource", "resource %s on %s is outside Workspace Access", s.Resource, s.Connection)
		}
	}
	if s.Connection != "" && s.Resource != "" && !v.authorityCovers(s.Capability, s.Connection, s.Resource) {
		v.add(p, "%s on %s %s is not covered by the requested Standing Authority", s.Capability, s.Connection, s.Resource)
	}
	if c.Effect && n.attempts > 1 && !c.Idempotent {
		v.add(p+".retry", "%s is an effect that is not proven idempotent; it cannot retry", s.Capability)
	}
}

func (v *validator) authorityCovers(capability, connection, resource string) bool {
	if v.def.Authority == nil {
		return false
	}
	for _, op := range v.def.Authority.Operations {
		if op.Capability == capability && op.Connection == connection && slices.Contains(op.Resources, resource) {
			return true
		}
	}
	return false
}

func (v *validator) compileAgent(n *node) {
	s, p := n.step, n.path
	switch {
	case s.Worker == "":
		v.add(p+".worker", "required: research, explore or build")
	case !slices.Contains(workers, s.Worker):
		v.add(p+".worker", "unknown worker preset %s; use %s", s.Worker, strings.Join(workers, ", "))
	}
	v.usePolicy(s.ModelPolicy, p+".model_policy")
	if s.Objective == "" {
		v.add(p+".objective", "required: a Markdown prompt file with the worker's objective")
	} else {
		v.loadPrompt(s.Objective, p+".objective")
	}
	if s.OutputSchema != "" {
		n.schema = v.loadSchema(s.OutputSchema, p+".output_schema")
	}
	n.output = agentResultType(n.schema)
	if s.Worker == WorkerBuild && !hasKey(s.Input, "task_id") {
		v.add(p+".input", "a build worker needs a task_id input naming the Task it works on")
	}
	b := s.Budget
	if b == nil {
		v.add(p+".budget", "required: time and tokens for each attempt")
		return
	}
	if b.Time == "" {
		v.add(p+".budget.time", "required")
	} else if d, err := parsePositiveDuration(b.Time); err != nil {
		v.add(p+".budget.time", "%v", err)
	} else {
		switch {
		case d > v.env.Workers.Deadline:
			v.add(p+".budget.time", "%s is above the operator's worker deadline %s", formatDuration(d), formatDuration(v.env.Workers.Deadline))
		case v.limitsOK && d > v.limits.duration:
			v.add(p+".budget.time", "%s is above limits.max_duration %s", formatDuration(d), formatDuration(v.limits.duration))
		}
		n.budget = d
	}
	switch {
	case b.Tokens == 0:
		v.add(p+".budget.tokens", "required")
	case b.Tokens < 0:
		v.add(p+".budget.tokens", "must be positive")
	case b.Tokens > v.env.Workers.Tokens:
		v.add(p+".budget.tokens", "%s is above the operator's worker token budget %s", grouped(b.Tokens), grouped(v.env.Workers.Tokens))
	case v.limitsOK && b.Tokens > v.limits.tokens:
		v.add(p+".budget.tokens", "%s is above limits.max_tokens %s", grouped(b.Tokens), grouped(v.limits.tokens))
	}
	n.tokens = b.Tokens
}

// reportWithheld says where an agent step's full report is.
const reportWithheld = "an agent step's output keeps the worker's summary, sources, limitations and typed result; " +
	"its full report stays in the worker's report store under execution_id (evie workflows inspect shows each worker's execution)"

// agentResultType is the worker result contract as a workflow sees it. Since
// Stage 14 sub-stage 10 it names the worker execution and the report's size
// instead of carrying the full report, which no step may read; a run pinned
// to evie.workflow.runtime/5 still records the report as it did.
func agentResultType(result *Type) *Type {
	report := StringType()
	report.Withheld = reportWithheld
	fields := []Field{
		{Name: "status", Type: &Type{Kind: KindString, Enum: []any{"succeeded", "partial"}}},
		{Name: "summary", Type: StringType()},
		{Name: "execution_id", Type: StringType(), Optional: true},
		{Name: "report_bytes", Type: IntegerType(), Optional: true},
		{Name: "report", Type: report, Optional: true},
		{Name: "sources", Type: ArrayType(ObjectType(Field{Name: "url", Type: StringType()}, Field{Name: "cited", Type: BooleanType()}))},
		{Name: "limitations", Type: ArrayType(StringType())},
		{Name: "tokens", Type: IntegerType()},
	}
	if result != nil {
		fields = append(fields, Field{Name: "result", Type: result})
	}
	return ObjectType(fields...)
}

func (v *validator) compileMap(n *node) {
	s, p := n.step, n.path
	if s.Over == "" {
		v.add(p+".over", "required: a list in a prior step's output")
	} else if r, err := parseRef(s.Over); err != nil {
		v.add(p+".over", "%v", err)
	} else if r.Root != refSteps {
		v.add(p+".over", "must reference a list in a prior step's output, not %s", s.Over)
	} else {
		n.over = &r
	}
	itemsOK := false
	switch {
	case !s.keys["max_items"]:
		v.add(p+".max_items", "required")
	case s.MaxItems < 1 || s.MaxItems > maxMapItems:
		v.add(p+".max_items", "%d is outside 1 to %d", s.MaxItems, maxMapItems)
	default:
		itemsOK = true
	}
	switch {
	case !s.keys["max_parallel"]:
		v.add(p+".max_parallel", "required")
	case s.MaxParallel < 1:
		v.add(p+".max_parallel", "must be positive")
	case v.limitsOK && s.MaxParallel > v.limits.parallel:
		v.add(p+".max_parallel", "%d exceeds limits.max_parallel %d", s.MaxParallel, v.limits.parallel)
	}
	switch s.OnItemFailure {
	case "":
		v.add(p+".on_item_failure", "required: fail or partial")
	case "fail":
		if s.keys["min_succeeded"] {
			v.add(p+".min_succeeded", "applies only with on_item_failure: partial")
		}
	case "partial":
		switch {
		case !s.keys["min_succeeded"]:
			v.add(p+".min_succeeded", "required with on_item_failure: partial")
		case s.MinSucceeded < 0:
			// 0 (2026-10-04) lets the step succeed whatever its items'
			// outcomes, so a later step can treat failures as data.
			v.add(p+".min_succeeded", "must be 0 or more")
		case itemsOK && s.MinSucceeded > s.MaxItems:
			v.add(p+".min_succeeded", "%d is above max_items %d", s.MinSucceeded, s.MaxItems)
		}
	default:
		v.add(p+".on_item_failure", "unknown policy %s; use fail or partial", s.OnItemFailure)
	}
	if s.Item == nil {
		v.add(p+".item", "required: the step each element runs")
		return
	}
	n.item = v.compile(s.Item, p+".item", true)
	n.item.id = n.id
}

func (v *validator) compileBranch(n *node) {
	s, p := n.step, n.path
	if len(s.Cases) == 0 {
		v.add(p+".cases", "a branch needs at least one case")
	}
	if s.Default == "" {
		v.add(p+".default", "required: the step to take when no case matches")
	}
	for i, c := range s.Cases {
		cp := fmt.Sprintf("%s.cases[%d]", p, i)
		if c.Next == "" {
			v.add(cp+".next", "required")
		}
		if c.When == nil {
			v.add(cp+".when", "required")
			continue
		}
		cc := compiledCase{op: c.When.Op, next: c.Next}
		if c.When.Ref == "" {
			v.add(cp+".when.ref", "required")
		} else if r, err := parseRef(c.When.Ref); err != nil {
			v.add(cp+".when.ref", "%v", err)
		} else {
			cc.ref = r
		}
		unary := c.When.Op == "empty" || c.When.Op == "not_empty"
		switch {
		case c.When.Op == "":
			v.add(cp+".when.op", "required: %s", strings.Join(conditionOps, ", "))
		case !slices.Contains(conditionOps, c.When.Op):
			v.add(cp+".when.op", "unknown operator %s; use %s", c.When.Op, strings.Join(conditionOps, ", "))
		case unary && c.When.Value.Kind != 0:
			v.add(cp+".when.value", "%s takes no value", c.When.Op)
		case !unary && c.When.Value.Kind == 0:
			v.add(cp+".when.value", "required with %s", c.When.Op)
		}
		if c.When.Value.Kind != 0 {
			value, err := nodeValue(&c.When.Value)
			if err != nil {
				v.add(cp+".when.value", "%v", err)
			}
			cc.value, cc.hasValue = value, true
		}
		n.cases = append(n.cases, cc)
	}
}

// edges lists a step's successors with the field that names each.
func (n *node) edges() [][2]string {
	s := n.step
	var out [][2]string
	if n.kind == KindBranch {
		for i, c := range s.Cases {
			if c.Next != "" {
				out = append(out, [2]string{fmt.Sprintf("%s.cases[%d].next", n.path, i), c.Next})
			}
		}
		if s.Default != "" {
			out = append(out, [2]string{n.path + ".default", s.Default})
		}
	} else if s.Next != "" {
		out = append(out, [2]string{n.path + ".next", s.Next})
	}
	return out
}

func (v *validator) checkGraph() {
	start := v.def.Start
	switch {
	case start == "":
		v.add("start", "required: the first step")
	case v.steps[start] == nil:
		v.add("start", "unknown step %s", start)
	}
	v.preds = map[string][]string{}
	for _, n := range v.list {
		for _, e := range n.edges() {
			if v.steps[e[1]] == nil {
				v.add(e[0], "unknown step %s", e[1])
				continue
			}
			if !slices.Contains(n.next, e[1]) {
				n.next = append(n.next, e[1])
				v.preds[e[1]] = append(v.preds[e[1]], n.id)
			}
		}
	}
	v.acyclic = true
	state := map[string]int{}
	var stack []string
	var visit func(id string)
	visit = func(id string) {
		state[id] = 1
		stack = append(stack, id)
		for _, next := range v.steps[id].next {
			switch state[next] {
			case 0:
				visit(next)
			case 1:
				i := slices.Index(stack, next)
				cycle := append(slices.Clone(stack[i:]), next)
				v.add("steps."+next, "cycle %s; v1 graphs allow no cycles (retries are per step)", strings.Join(cycle, " -> "))
				v.acyclic = false
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
	}
	for _, n := range v.list {
		if state[n.id] == 0 {
			visit(n.id)
		}
	}
	v.reachable = map[string]bool{}
	if v.steps[start] != nil {
		queue := []string{start}
		v.reachable[start] = true
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			for _, next := range v.steps[id].next {
				if !v.reachable[next] {
					v.reachable[next] = true
					queue = append(queue, next)
				}
			}
		}
		for _, n := range v.list {
			if !v.reachable[n.id] {
				v.add(n.path, "unreachable from start %s", start)
			}
		}
	}
	if !v.acyclic {
		return
	}
	indegree := map[string]int{}
	for _, n := range v.list {
		for _, next := range n.next {
			indegree[next]++
		}
	}
	done := map[string]bool{}
	for len(v.order) < len(v.list) {
		for _, n := range v.list {
			if !done[n.id] && indegree[n.id] == 0 {
				done[n.id] = true
				v.order = append(v.order, n.id)
				for _, next := range n.next {
					indegree[next]--
				}
				break
			}
		}
	}
	v.dom = map[string]map[string]bool{}
	for _, id := range v.order {
		if !v.reachable[id] {
			continue
		}
		var acc map[string]bool
		if id != start {
			for _, p := range v.preds[id] {
				if !v.reachable[p] {
					continue
				}
				if acc == nil {
					acc = map[string]bool{}
					for k := range v.dom[p] {
						acc[k] = true
					}
					continue
				}
				for k := range acc {
					if !v.dom[p][k] {
						delete(acc, k)
					}
				}
			}
		}
		if acc == nil {
			acc = map[string]bool{}
		}
		acc[id] = true
		v.dom[id] = acc
	}
}

// dominates reports whether step a runs before step b on every path.
func (v *validator) dominates(a, b string) bool { return a != b && v.dom[b][a] }

// refType resolves a reference's type from the point of view of anchor (the
// step, or the map that owns the item).
func (v *validator) refType(r Ref, anchor *node, item *Type) (*Type, error) {
	switch r.Root {
	case refInputs:
		in, ok := v.inputs[r.Name]
		if !ok {
			return nil, fmt.Errorf("%s: unknown input %s", r.Raw, r.Name)
		}
		v.usedInputs[r.Name] = true
		t, err := walkType(in.t, r.Path)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", r.Raw, err)
		}
		return t, nil
	case refSteps:
		target, ok := v.steps[r.Name]
		switch {
		case !ok:
			return nil, fmt.Errorf("%s: unknown step %s", r.Raw, r.Name)
		case anchor != nil && target.id == anchor.id:
			return nil, fmt.Errorf("%s: a step cannot read its own output", r.Raw)
		case anchor != nil && !v.dominates(target.id, anchor.id):
			return nil, fmt.Errorf("%s: %s does not run before %s on every path", r.Raw, r.Name, anchor.id)
		case target.kind == KindBranch:
			return nil, fmt.Errorf("%s: branch steps have no output", r.Raw)
		case target.output == nil:
			return AnyType(), nil
		}
		t, err := walkType(target.output, r.Path)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", r.Raw, err)
		}
		return t, nil
	case refItem:
		if item == nil {
			return nil, fmt.Errorf("%s: $item is only available inside a map item", r.Raw)
		}
		t, err := walkType(item, r.Path)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", r.Raw, err)
		}
		return t, nil
	}
	return nil, fmt.Errorf("%s: $schedule values are only available in schedule.inputs", r.Raw)
}

func (v *validator) compileArg(p string, value yaml.Node, anchor *node, item *Type) (arg, bool) {
	val, err := nodeValue(&value)
	if err != nil {
		v.add(p, "%v", err)
		return arg{}, false
	}
	if s, ok := val.(string); ok && isRefText(s) {
		r, err := parseRef(s)
		if err != nil {
			v.add(p, "%v", err)
			return arg{}, false
		}
		t, err := v.refType(r, anchor, item)
		if err != nil {
			v.add(p, "%v", err)
			return arg{}, false
		}
		return arg{ref: &r, t: t}, true
	}
	return arg{literal: val, isConst: true, t: typeOfValue(val)}, true
}

// resolve checks a step's references and types, in graph order.
func (v *validator) resolve(n *node, anchor *node, item *Type) {
	s := n.step
	n.args = map[string]arg{}
	ok := true
	for _, name := range sortedKeys(s.Input) {
		a, good := v.compileArg(n.path+".input."+name, s.Input[name], anchor, item)
		ok = ok && good
		if good {
			n.args[name] = a
		}
	}
	switch n.kind {
	case KindDeterministic:
		if n.fn != nil {
			v.checkFunctionArgs(n, ok)
		}
	case KindCapability:
		if n.cap != nil {
			v.checkCapabilityInput(n)
		}
	case KindMap:
		v.resolveMap(n)
	case KindBranch:
		v.resolveBranch(n)
	}
}

func (v *validator) checkFunctionArgs(n *node, complete bool) {
	f := n.fn
	static := map[string]StaticArg{}
	for _, name := range sortedKeys(n.step.Input) {
		if !slices.ContainsFunc(f.Params, func(p Param) bool { return p.Name == name }) {
			v.add(n.path+".input."+name, "%s takes no argument %s", f.ID(), name)
		}
	}
	for _, param := range f.Params {
		p := n.path + ".input." + param.Name
		if !hasKey(n.step.Input, param.Name) {
			v.add(n.path+".input", "missing argument %s for %s", param.Name, f.ID())
			continue
		}
		a, good := n.args[param.Name]
		if !good {
			continue
		}
		if a.isConst {
			if err := checkValue(param.Type, a.literal); err != nil {
				v.add(p, "%v", err)
				continue
			}
			if err := param.checkBounds(a.literal); err != nil {
				v.add(p, "%v", err)
				continue
			}
		} else if err := assignable(a.t, param.Type); err != nil {
			v.add(p, "%s cannot take %s: %v", f.ID(), a.ref.Raw, err)
			continue
		}
		static[param.Name] = StaticArg{Type: a.t, Literal: a.literal, IsConst: a.isConst}
	}
	if complete && f.Refine != nil {
		n.output = f.Refine(static)
	}
}

func (v *validator) checkCapabilityInput(n *node) {
	in := n.cap.Input
	if in == nil {
		in = ObjectType()
	}
	for _, name := range in.Required {
		if !hasKey(n.step.Input, name) {
			v.add(n.path+".input", "missing %s for %s", name, n.cap.ID)
		}
	}
	for _, name := range sortedKeys(n.step.Input) {
		want, declared := in.Properties[name]
		if !declared {
			v.add(n.path+".input."+name, "%s has no input %s", n.cap.ID, name)
			continue
		}
		a, ok := n.args[name]
		if !ok {
			continue
		}
		var err error
		if a.isConst {
			err = checkValue(want, a.literal)
		} else {
			err = assignable(a.t, want)
		}
		if err != nil {
			v.add(n.path+".input."+name, "%v", err)
		}
	}
}

func (v *validator) resolveMap(n *node) {
	if n.over == nil {
		return
	}
	t, err := v.refType(*n.over, n, nil)
	if err != nil {
		v.add(n.path+".over", "%v", err)
		return
	}
	switch {
	case t.isAny():
		n.elem = AnyType()
	case t.Kind != KindArray:
		v.add(n.path+".over", "%s is not a list (it is %s)", n.over.Raw, t.describe())
		return
	default:
		n.elem = t.Items
	}
	s := n.step
	if s.MaxItems >= 1 && s.MaxItems <= maxMapItems {
		switch {
		case t.MaxItems != nil && *t.MaxItems > int64(s.MaxItems):
			v.add(n.path+".over", "the list can hold %d items; max_items %d", *t.MaxItems, s.MaxItems)
		case t.MaxItems == nil:
			v.warnings = append(v.warnings, fmt.Sprintf("%s.over: the list's type does not bound its length; a list longer than max_items %d fails the step", n.path, s.MaxItems))
		}
	}
	if n.item == nil || !n.item.known {
		return
	}
	v.resolve(n.item, n, n.elem)
	if n.item.output == nil {
		return
	}
	n.output = mapOutputType(n.elem, n.item.output, s.MaxItems)
}

func mapOutputType(elem, out *Type, maxItems int) *Type {
	bound := int64(maxItems)
	results := ArrayType(ObjectType(Field{Name: "index", Type: IntegerType()}, Field{Name: "item", Type: elem}, Field{Name: "output", Type: out}))
	failures := ArrayType(ObjectType(Field{Name: "index", Type: IntegerType()}, Field{Name: "item", Type: elem}, Field{Name: "error", Type: StringType()}))
	results.MaxItems, failures.MaxItems = &bound, &bound
	return ObjectType(
		Field{Name: "results", Type: results},
		Field{Name: "failures", Type: failures},
		Field{Name: "succeeded", Type: IntegerType()},
		Field{Name: "failed", Type: IntegerType()},
	)
}

func (v *validator) resolveBranch(n *node) {
	for i, c := range n.cases {
		cp := fmt.Sprintf("%s.cases[%d].when", n.path, i)
		if c.ref.Raw == "" || !slices.Contains(conditionOps, c.op) {
			continue
		}
		t, err := v.refType(c.ref, n, nil)
		if err != nil {
			v.add(cp+".ref", "%v", err)
			continue
		}
		if t.isAny() {
			continue
		}
		switch c.op {
		case "empty", "not_empty":
			if t.Kind != KindArray && t.Kind != KindString {
				v.add(cp, "%s applies to lists and text; %s is %s", c.op, c.ref.Raw, t.describe())
			}
		case "gt", "gte", "lt", "lte":
			if t.Kind != KindInteger && t.Kind != KindNumber {
				v.add(cp, "%s compares numbers; %s is %s", c.op, c.ref.Raw, t.describe())
			} else if _, ok := number(c.value); c.hasValue && !ok {
				v.add(cp+".value", "%s needs a number", c.op)
			}
		case "eq", "ne":
			if t.Kind == KindArray || t.Kind == KindObject {
				v.add(cp, "%s compares single values; %s is %s", c.op, c.ref.Raw, t.describe())
			} else if c.hasValue {
				if err := checkValue(&Type{Kind: t.Kind, Enum: t.Enum}, c.value); err != nil {
					v.add(cp+".value", "%v", err)
				}
			}
		}
	}
}

func (v *validator) terminals() []string {
	var out []string
	for _, id := range v.order {
		if v.reachable[id] && len(v.steps[id].next) == 0 {
			out = append(out, id)
		}
	}
	return out
}

func (v *validator) checkOutputs() {
	for _, name := range sortedKeys(v.def.Outputs) {
		p := "outputs." + name
		if !identifierPattern.MatchString(name) {
			v.add(p, "%q is not a valid output name; use lowercase letters, digits and _", name)
			continue
		}
		r, err := parseRef(v.def.Outputs[name])
		if err != nil {
			v.add(p, "%v", err)
			continue
		}
		info := outputInfo{name: name, ref: r, always: true}
		switch r.Root {
		case refInputs:
			if _, err := v.refType(r, nil, nil); err != nil {
				v.add(p, "%v", err)
			}
		case refSteps:
			target := v.steps[r.Name]
			if target == nil {
				v.add(p, "%s: unknown step %s", r.Raw, r.Name)
				continue
			}
			if !v.acyclic {
				continue
			}
			if _, err := v.refType(r, nil, nil); err != nil {
				v.add(p, "%v", err)
				continue
			}
			for _, t := range v.terminals() {
				if t != target.id && !v.dominates(target.id, t) {
					info.always = false
				}
			}
		default:
			v.add(p, "outputs read $inputs or $steps, not %s", r.Raw)
			continue
		}
		v.outputs = append(v.outputs, info)
	}
}

func (v *validator) checkAuthority() {
	if v.def.Authority == nil {
		return
	}
	seen := map[string]bool{}
	for i, op := range v.def.Authority.Operations {
		p := fmt.Sprintf("authority.operations[%d]", i)
		switch {
		case op.Capability == "":
			v.add(p+".capability", "required")
		case strings.HasPrefix(op.Capability, "subagents."):
			v.add(p+".capability", "workflows cannot request %s", op.Capability)
		default:
			if _, ok := v.env.Capabilities.Capability(op.Capability); !ok {
				v.add(p+".capability", "%s is not in the capability catalog", op.Capability)
			}
		}
		conn, connOK := v.env.Access.connection(op.Connection)
		switch {
		case op.Connection == "":
			v.add(p+".connection", "required")
		case !opaquePattern.MatchString(op.Connection):
			v.add(p+".connection", "%q is not a Connection ID", op.Connection)
		case v.env.AccessUnchecked:
			connOK = false
		case !connOK:
			v.add(p+".connection", "connection %s is outside Workspace Access", op.Connection)
		}
		if len(op.Resources) == 0 {
			v.add(p+".resources", "list the exact resources")
		}
		for j, r := range op.Resources {
			switch {
			case !opaquePattern.MatchString(r):
				v.add(p+".resources", "%q is not a resource selector", r)
			case slices.Index(op.Resources, r) != j:
				v.add(p+".resources", "lists %s twice", r)
			case connOK && !conn.allows(r):
				v.add(p+".resources", "resource %s on %s is outside Workspace Access", r, op.Connection)
			}
		}
		switch {
		case op.MaxCallsPerRun == 0:
			v.add(p+".max_calls_per_run", "required")
		case op.MaxCallsPerRun < 0 || op.MaxCallsPerRun > maxCallsPerRun:
			v.add(p+".max_calls_per_run", "must be 1 to %d", maxCallsPerRun)
		}
		key := op.Capability + "\x00" + op.Connection
		if seen[key] {
			v.add(p, "repeats %s on %s; list its resources in one operation", op.Capability, op.Connection)
		}
		seen[key] = true
		calls, intents, used := 0, 0, map[string]bool{}
		for _, n := range v.list {
			if n.kind == KindCapability && n.step.Capability == op.Capability && n.step.Connection == op.Connection {
				calls++
				used[n.step.Resource] = true
				if n.cap != nil && n.cap.Effect {
					intents += n.attempts
				}
			}
		}
		// The runtime counts every Effect Intent, retries included, and
		// refuses one past the bound before calling the provider.
		if op.MaxCallsPerRun > 0 && calls <= op.MaxCallsPerRun && intents > op.MaxCallsPerRun {
			v.warnings = append(v.warnings, fmt.Sprintf("%s: with retries its steps may send up to %d Effect Intents of %s on %s, but max_calls_per_run %d refuses each one past it before the provider is called",
				p, intents, op.Capability, op.Connection, op.MaxCallsPerRun))
		}
		if calls == 0 {
			v.add(p, "no step uses %s on %s; request only what the workflow needs", op.Capability, op.Connection)
			continue
		}
		for _, r := range op.Resources {
			if !used[r] {
				v.add(p, "resource %s: no step uses it", r)
			}
		}
		if op.MaxCallsPerRun > 0 && calls > op.MaxCallsPerRun {
			v.add(p, "%d steps call %s on %s; max_calls_per_run is %d", calls, op.Capability, op.Connection, op.MaxCallsPerRun)
		}
	}
}

func (v *validator) checkRunKey() {
	rk := v.def.RunKey
	if rk == nil {
		v.add("run_key", "required: the inputs that identify one logical execution")
		return
	}
	if len(rk.Inputs) == 0 {
		v.add("run_key.inputs", "name at least one input")
	}
	if len(rk.Inputs) > maxRunKeyInputs {
		v.add("run_key.inputs", "names %d inputs; at most %d", len(rk.Inputs), maxRunKeyInputs)
	}
	for i, name := range rk.Inputs {
		in, ok := v.inputs[name]
		switch {
		case slices.Index(rk.Inputs, name) != i:
			v.add("run_key.inputs", "%s appears more than once", name)
		case !ok:
			v.add("run_key.inputs", "unknown input %s", name)
		case !in.spec.Required && !in.hasDefault:
			v.add("run_key.inputs", "%s is optional without a default; a run key input must always have a value", name)
		case in.t.Kind == KindNumber:
			v.add("run_key.inputs", "%s is a number; run keys use string, integer or boolean inputs", name)
		default:
			v.usedInputs[name] = true
		}
	}
}

func (v *validator) checkSchedule() {
	s := v.def.Schedule
	if s == nil {
		return
	}
	if s.Cron == "" {
		v.add("schedule.cron", "required")
	} else if err := validateCron(s.Cron); err != nil {
		v.add("schedule.cron", "%v", err)
	}
	switch {
	case s.Timezone == "" || s.Timezone == "Local":
		v.add("schedule.timezone", "name an IANA timezone such as America/New_York")
	default:
		if _, err := time.LoadLocation(s.Timezone); err != nil {
			v.add("schedule.timezone", "unknown timezone %s", s.Timezone)
		}
	}
	switch c := s.CatchUp; {
	case c == nil:
		v.add("schedule.catch_up", "required: policy skip, or each_missed with max_runs")
	case c.Policy == "skip":
		if c.MaxRuns != 0 {
			v.add("schedule.catch_up.max_runs", "applies only with each_missed")
		}
	case c.Policy == "each_missed":
		switch {
		case c.MaxRuns == 0:
			v.add("schedule.catch_up.max_runs", "required with each_missed (1 to %d)", maxCatchUpRuns)
		case c.MaxRuns < 0 || c.MaxRuns > maxCatchUpRuns:
			v.add("schedule.catch_up.max_runs", "%d is outside 1 to %d", c.MaxRuns, maxCatchUpRuns)
		}
	default:
		v.add("schedule.catch_up.policy", "unknown policy %q; use skip or each_missed", c.Policy)
	}
	fed := map[string]bool{}
	for _, name := range sortedKeys(s.Inputs) {
		p := "schedule.inputs." + name
		in, ok := v.inputs[name]
		if !ok {
			v.add(p, "unknown input %s", name)
			continue
		}
		node := s.Inputs[name]
		val, err := nodeValue(&node)
		if err != nil {
			v.add(p, "%v", err)
			continue
		}
		if text, isText := val.(string); isText && isRefText(text) {
			r, err := parseRef(text)
			switch {
			case err != nil:
				v.add(p, "%v", err)
			case r.Root != refSchedule:
				v.add(p, "schedule inputs take literals, $schedule.date or $schedule.time, not %s", text)
			case in.t.Kind != KindString:
				v.add(p, "%s is text; %s is %s", text, name, in.t.Kind)
			default:
				fed[name] = true
			}
			continue
		}
		if err := checkValue(in.t, val); err != nil {
			v.add(p, "%v", err)
		}
	}
	for _, name := range sortedKeys(v.inputs) {
		if in := v.inputs[name]; in.spec.Required && !hasKey(s.Inputs, name) {
			v.add("schedule.inputs", "missing required input %s", name)
		}
	}
	if v.def.RunKey != nil && !slices.ContainsFunc(v.def.RunKey.Inputs, func(n string) bool { return fed[n] }) {
		v.add("run_key", "a scheduled workflow's run key must include an input set from $schedule, or every scheduled run shares one key")
	}
}

var cronFields = []struct {
	name     string
	min, max int
}{{"minute", 0, 59}, {"hour", 0, 23}, {"day of month", 1, 31}, {"month", 1, 12}, {"day of week", 0, 6}}

// validateCron accepts five numeric fields with *, lists, ranges and steps;
// names such as MON are refused.
func validateCron(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != len(cronFields) {
		return fmt.Errorf("%q needs five fields: minute hour day-of-month month day-of-week", expr)
	}
	for i, field := range fields {
		spec := cronFields[i]
		for _, part := range strings.Split(field, ",") {
			base, step, stepped := strings.Cut(part, "/")
			if stepped {
				if n, err := strconv.Atoi(step); err != nil || n < 1 || step != strconv.Itoa(n) {
					return fmt.Errorf("%s field %q: step %s must be a positive number", spec.name, field, step)
				}
			}
			if base == "*" {
				continue
			}
			lo, hi, ranged := strings.Cut(base, "-")
			if !ranged {
				hi = lo
			}
			a, errA := strconv.Atoi(lo)
			b, errB := strconv.Atoi(hi)
			if errA != nil || errB != nil || lo != strconv.Itoa(a) || hi != strconv.Itoa(b) {
				return fmt.Errorf("%s field %q: %s is not a number or range", spec.name, field, base)
			}
			if a < spec.min || b > spec.max || a > b {
				return fmt.Errorf("%s field %q: %s is outside %d-%d", spec.name, field, base, spec.min, spec.max)
			}
		}
	}
	return nil
}

func (v *validator) checkUsage() {
	for _, name := range sortedKeys(v.def.ModelPolicies) {
		if !v.usedPolicies[name] {
			v.warnings = append(v.warnings, fmt.Sprintf("model_policies.%s: no step uses it", name))
		}
	}
	for _, o := range v.outputs {
		if o.ref.Root == refInputs {
			v.usedInputs[o.ref.Name] = true
		}
	}
	for _, name := range sortedKeys(v.inputs) {
		if !v.usedInputs[name] {
			v.warnings = append(v.warnings, fmt.Sprintf("inputs.%s: no step, run key or output uses it", name))
		}
	}
	for _, id := range v.order {
		n := v.steps[id]
		switch n.kind {
		case KindBranch:
		case KindMap:
			if n.item != nil {
				items := n.step.MaxItems
				v.worstTries += items * n.item.attempts
				if n.item.kind == KindAgent {
					v.worstTokens += int64(items*n.item.attempts) * n.item.tokens
				}
			}
		case KindAgent:
			v.worstTries += n.attempts
			v.worstTokens += int64(n.attempts) * n.tokens
		default:
			v.worstTries += n.attempts
		}
	}
	if !v.limitsOK {
		return
	}
	if v.worstTokens > v.limits.tokens {
		v.warnings = append(v.warnings, fmt.Sprintf("worst-case agent tokens %s exceed limits.max_tokens %s; a run stops at the limit", grouped(v.worstTokens), grouped(v.limits.tokens)))
	}
	if v.worstTries > v.limits.attempts {
		v.warnings = append(v.warnings, fmt.Sprintf("worst-case attempts %d exceed limits.max_attempts %d; a run stops at the limit", v.worstTries, v.limits.attempts))
	}
}
