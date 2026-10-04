package workflow

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Capability reads (Stage 14 sub-stage 5): what the runtime needs from a
// validated definition to pin and call a capability step, and the contract
// identity it pins.

// A contract's schema hash identifies its typed input and output and its
// read or effect classification; it is the same however the contract was
// built and changes with any of them, but not with the contract version
// label or the connections and resources a provider serves, which are
// pinned and checked separately.
func TestWorkflowCapabilitySchemaHashIdentifiesTheContractSchema(t *testing.T) {
	base := func() CapabilityContract {
		return CapabilityContract{ID: "ledger.month_total", Contract: "1.0.0",
			Input:  ObjectType(Field{Name: "month", Type: StringType()}, Field{Name: "account", Type: StringType(), Optional: true}),
			Output: ObjectType(Field{Name: "total_cents", Type: IntegerType()}, Field{Name: "count", Type: IntegerType()})}
	}
	hash := base().SchemaSHA256()
	if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
		t.Fatalf("schema hash %q is not 64 lowercase hex digits", hash)
	}
	reordered := base()
	reordered.Output = ObjectType(Field{Name: "count", Type: IntegerType()}, Field{Name: "total_cents", Type: IntegerType()})
	reordered.Output.Required = []string{"total_cents", "count"}
	reordered.Contract = "1.0.1"
	reordered.Connections, reordered.Resources = []string{"ledger-local"}, []string{"book"}
	if got := reordered.SchemaSHA256(); got != hash {
		t.Fatalf("the same schema hashes to %s and %s", got, hash)
	}
	for name, change := range map[string]func(*CapabilityContract){
		"output field":  func(c *CapabilityContract) { c.Output.Properties["count"] = StringType() },
		"input bound":   func(c *CapabilityContract) { c.Input.Properties["month"].MaxLength = ptr(int64(7)) },
		"optional":      func(c *CapabilityContract) { c.Input.Required = []string{"account", "month"} },
		"effect":        func(c *CapabilityContract) { c.Effect = true },
		"idempotent":    func(c *CapabilityContract) { c.Idempotent = true },
		"missing input": func(c *CapabilityContract) { c.Input = nil },
	} {
		c := base()
		change(&c)
		if c.SchemaSHA256() == hash {
			t.Fatalf("%s: the schema hash did not change", name)
		}
	}
}

// An effect's reconciliation and Resource Conflict Key rule are part of its
// schema (Stage 14 sub-stage 6), so a run pinned to it notices a change;
// they leave every hash pinned before them unchanged: reads, and effects
// declaring neither.
func TestWorkflowCapabilitySchemaHashCoversEffectDeclarations(t *testing.T) {
	read := CapabilityContract{ID: "ledger.month_total", Contract: "1.0.0",
		Input:  ObjectType(Field{Name: "month", Type: StringType()}, Field{Name: "account", Type: StringType(), Optional: true}),
		Output: ObjectType(Field{Name: "total_cents", Type: IntegerType()}, Field{Name: "count", Type: IntegerType()})}
	if got := read.SchemaSHA256(); got != "e3b1b1878a6d64f918b871150b6aba52d816e89b2a87ff3491a521c6445113cd" {
		t.Fatalf("a read's pinned schema hash changed to %s", got)
	}
	effect := read
	effect.Effect, effect.Idempotent = true, true
	if got := effect.SchemaSHA256(); got != "5594bcce1c4523ab0e48283bc973e6ab511abb856c530cde5f72ad555c714e0c" {
		t.Fatalf("an effect's schema hash without reconciliation or conflict rule changed to %s", got)
	}
	seen := map[string]bool{effect.SchemaSHA256(): true}
	for name, change := range map[string]func(*CapabilityContract){
		"reconciles":       func(c *CapabilityContract) { c.Reconciles = true },
		"conflict keys":    func(c *CapabilityContract) { c.ConflictKeys = "resource" },
		"conflict account": func(c *CapabilityContract) { c.ConflictKeys = "account" },
	} {
		c := effect
		change(&c)
		if seen[c.SchemaSHA256()] {
			t.Fatalf("%s: the schema hash did not change", name)
		}
		seen[c.SchemaSHA256()] = true
	}
}

// A provider that declares the connections and resources it serves refuses
// a step naming any other, at validation as well as at run time.
func TestWorkflowValidationRefusesConnectionsAndResourcesTheProviderDoesNotServe(t *testing.T) {
	served := func(env *Environment) {
		list, _ := env.Capabilities.Capability("notes.list")
		appendNote, _ := env.Capabilities.Capability("notes.append")
		list.Connections, list.Resources = []string{"conn-other"}, []string{"notebook:archive"}
		env.Capabilities = NewCatalog(list, appendNote)
	}
	runValidationCases(t, []validationCase{
		{name: "connection the provider does not serve", def: "notes-digest", env: served,
			fragments: []string{"steps.read.connection", "notes.list", "conn-other", "conn-notes"}},
		{name: "resource the provider does not serve", def: "notes-digest", env: served,
			fragments: []string{"steps.read.resource", "notebook:archive", "notebook:inbox"}},
	})
	env := testEnvironment()
	list, _ := env.Capabilities.Capability("notes.list")
	appendNote, _ := env.Capabilities.Capability("notes.append")
	list.Connections, list.Resources = []string{"conn-notes"}, []string{"notebook:inbox"}
	env.Capabilities = NewCatalog(list, appendNote)
	mustValidate(t, filepath.Join("testdata", "notes-digest"), env)
}

// A review outside any Workspace (the standalone validate and simulate
// commands) checks capability steps against the catalog but not against
// Workspace Access, which only a Workspace has; every other rule applies.
func TestWorkflowReviewOutsideAWorkspaceSkipsOnlyWorkspaceAccess(t *testing.T) {
	env := testEnvironment()
	env.Access = WorkspaceAccess{}
	if _, err := validateDir(filepath.Join("testdata", "notes-digest"), env); err == nil {
		t.Fatal("a definition outside Workspace Access validated")
	}
	env.AccessUnchecked = true
	mustValidate(t, filepath.Join("testdata", "notes-digest"), env)
	dir := copyDefinition(t, "notes-digest")
	edit(t, dir, "workflow.yaml", "    resource: \"notebook:digest\"\n", "    resource: \"notebook:inbox\"\n")
	_, err := validateDir(dir, env)
	wantProblem(t, err, "steps.write", "Standing Authority")
	env.Capabilities = NewCatalog()
	_, err = validateDir(filepath.Join("testdata", "notes-digest"), env)
	wantProblem(t, err, "steps.read.capability", "not in the capability catalog")
}

// The runtime reads each capability step's plan and binding from the
// validated definition, resolves its input without absent optional
// fields, and checks a provider's normalized output against the contract.
func TestWorkflowCapabilityStepsInputAndOutput(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment())
	steps := v.CapabilitySteps()
	want := []CapabilityStep{
		{Step: "read", Capability: "notes.list", Contract: "1.0.0", Connection: "conn-notes", Resource: "notebook:inbox"},
		{Step: "write", Capability: "notes.append", Contract: "1.0.0", Connection: "conn-notes", Resource: "notebook:digest", Effect: true},
	}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("capability steps = %+v", steps)
	}
	plan, ok := v.Step("read")
	if !ok || plan.Kind != KindCapability || plan.Capability != "notes.list" || plan.Connection != "conn-notes" || plan.Resource != "notebook:inbox" ||
		plan.Effect || plan.Attempts != 3 || plan.Backoff.Seconds() != 2 || plan.Next != "check" {
		t.Fatalf("read plan = %+v", plan)
	}
	inputs := map[string]any{"day": "2026-10-05"}
	input, err := v.CapabilityInput("read", inputs, map[string]any{})
	if err != nil || !reflect.DeepEqual(input, map[string]any{"day": "2026-10-05"}) {
		t.Fatalf("read input = %v, %v", input, err)
	}
	if _, err := v.CapabilityInput("read", map[string]any{"day": int64(5)}, map[string]any{}); err == nil || !strings.Contains(err.Error(), "day") {
		t.Fatalf("an input outside the contract was accepted: %v", err)
	}
	if _, err := v.CapabilityInput("write", inputs, map[string]any{}); err == nil || !strings.Contains(err.Error(), "summarize") {
		t.Fatalf("an input from a step that has not run was accepted: %v", err)
	}
	if _, err := v.CapabilityInput("check", inputs, map[string]any{}); err == nil {
		t.Fatal("a branch was treated as a capability step")
	}
	good := map[string]any{"notes": []any{map[string]any{"id": "n1", "text": "Call the bank."}}}
	if err := v.CheckCapabilityOutput("read", good); err != nil {
		t.Fatalf("a contract output was refused: %v", err)
	}
	for name, bad := range map[string]any{
		"missing field":  map[string]any{},
		"wrong type":     map[string]any{"notes": "none"},
		"extra field":    map[string]any{"notes": []any{}, "cursor": "x"},
		"not an object":  []any{},
		"nested problem": map[string]any{"notes": []any{map[string]any{"id": int64(1), "text": "x"}}},
	} {
		if err := v.CheckCapabilityOutput("read", bad); err == nil {
			t.Fatalf("%s: malformed output was accepted", name)
		}
	}
}
