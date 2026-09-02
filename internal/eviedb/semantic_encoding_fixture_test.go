package eviedb

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	v1ManifestSHA256 = "7e125fcdbe4a89abb48a7861ef51552dcb5d88d20896e516e3f912677b933510"
	v1FixtureSHA256  = "d5b77d3204310fc9a4eb22a038738faac8551b4989dc28c15b453fbb73db0b56"
	v2ManifestSHA256 = "65d2e91245d037d500a7f4d22de00938ed2e98f70bd48ffd45af3f58c0ca47e2"
	v2FixtureSHA256  = "b87b5e7501ab14bd4d0c61e32b5edc88e77ac2487a9b155ba6cac34e200feb05"
)

func TestSemanticCorrectionEncodingV2FixtureAndFrozenV1Contract(t *testing.T) {
	fixtures := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "semantic-memory")
	assertFileSHA256(t, filepath.Join(fixtures, "v1", "manifest.schema.json"), v1ManifestSHA256)
	assertFileSHA256(t, filepath.Join(fixtures, "v1", "literal-claim.json"), v1FixtureSHA256)

	schemaBytes, err := os.ReadFile(filepath.Join(fixtures, "v2", "manifest.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Const int `json:"const"`
		} `json:"properties"`
		Definitions map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("parse v2 schema: %v", err)
	}
	if schema.Properties["fixture_schema_version"].Const != 2 {
		t.Fatal("v2 fixture schema does not require version 2")
	}
	for _, definition := range []string{"correction", "effectV2", "proposalV2", "operationV2", "operation"} {
		if len(schema.Definitions[definition]) == 0 {
			t.Fatalf("v2 fixture schema omits %s", definition)
		}
	}
	assertJSONContains(t, schema.Definitions["operation"], `../v1/manifest.schema.json#/$defs/operation`)
	rules := parseClosedCorrectionEffectRules(t, schema.Definitions["effectV2"])

	fixtureBytes, err := os.ReadFile(filepath.Join(fixtures, "v2", "claim-correction.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int `json:"fixture_schema_version"`
		Operations    []struct {
			SchemaVersion  int                         `json:"schema_version"`
			Proposal       canonicalCorrectionProposal `json:"proposal"`
			ProposalSHA256 string                      `json:"proposal_sha256"`
			EffectSHA256   string                      `json:"effect_sha256"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("parse v2 correction fixture: %v", err)
	}
	if fixture.SchemaVersion != 2 || len(fixture.Operations) != 1 || fixture.Operations[0].SchemaVersion != 2 {
		t.Fatalf("v2 fixture envelope = %+v", fixture)
	}
	operation := fixture.Operations[0]
	if operation.Proposal.Kind != "correct_claim" || len(operation.Proposal.Effect.Corrections) != 1 ||
		len(operation.Proposal.Effect.Claims) != 1 || len(operation.Proposal.Effect.SourceLinks) != 1 ||
		len(operation.Proposal.Effect.Transitions) != 3 {
		t.Fatalf("v2 correction effect is incomplete: %+v", operation.Proposal.Effect)
	}
	if err := validateClosedCorrectionEffect(rules, operation.Proposal.Effect); err != nil {
		t.Fatalf("valid v2 correction fixture rejected by schema rules: %v", err)
	}
	for name, mutate := range map[string]func(*canonicalCorrectionEffect){
		"second scope":        func(effect *canonicalCorrectionEffect) { effect.Scopes = append(effect.Scopes, "global") },
		"predicate":           func(effect *canonicalCorrectionEffect) { effect.Predicates = append(effect.Predicates, struct{}{}) },
		"entity":              func(effect *canonicalCorrectionEffect) { effect.Entities = append(effect.Entities, struct{}{}) },
		"alias":               func(effect *canonicalCorrectionEffect) { effect.Aliases = append(effect.Aliases, struct{}{}) },
		"missing Claim":       func(effect *canonicalCorrectionEffect) { effect.Claims = nil },
		"second Claim":        func(effect *canonicalCorrectionEffect) { effect.Claims = append(effect.Claims, effect.Claims[0]) },
		"missing Source Link": func(effect *canonicalCorrectionEffect) { effect.SourceLinks = nil },
		"second Source Link": func(effect *canonicalCorrectionEffect) {
			effect.SourceLinks = append(effect.SourceLinks, effect.SourceLinks[0])
		},
		"Graph Link": func(effect *canonicalCorrectionEffect) { effect.GraphLinks = append(effect.GraphLinks, struct{}{}) },
		"reordered transitions": func(effect *canonicalCorrectionEffect) {
			effect.Transitions[0], effect.Transitions[1] = effect.Transitions[1], effect.Transitions[0]
		},
		"fourth transition": func(effect *canonicalCorrectionEffect) {
			effect.Transitions = append(effect.Transitions, effect.Transitions[0])
		},
		"missing correction": func(effect *canonicalCorrectionEffect) { effect.Corrections = nil },
		"second correction": func(effect *canonicalCorrectionEffect) {
			effect.Corrections = append(effect.Corrections, effect.Corrections[0])
		},
	} {
		t.Run("schema rejects "+name, func(t *testing.T) {
			invalid := cloneCorrectionEffect(t, operation.Proposal.Effect)
			mutate(&invalid)
			if err := validateClosedCorrectionEffect(rules, invalid); err == nil {
				t.Fatalf("closed v2 schema accepted %s", name)
			}
		})
	}
	proposalHash, _, err := semanticHash(operation.Proposal)
	if err != nil {
		t.Fatal(err)
	}
	effectHash, _, err := semanticHash(operation.Proposal.Effect)
	if err != nil {
		t.Fatal(err)
	}
	if proposalHash != operation.ProposalSHA256 || effectHash != operation.EffectSHA256 {
		t.Fatalf("v2 canonical hashes = %s / %s, fixture = %s / %s",
			proposalHash, effectHash, operation.ProposalSHA256, operation.EffectSHA256)
	}
}

func TestSemanticLifecycleEncodingV3FixtureAndFrozenPriorContracts(t *testing.T) {
	fixtures := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "semantic-memory")
	assertFileSHA256(t, filepath.Join(fixtures, "v1", "manifest.schema.json"), v1ManifestSHA256)
	assertFileSHA256(t, filepath.Join(fixtures, "v1", "literal-claim.json"), v1FixtureSHA256)
	assertFileSHA256(t, filepath.Join(fixtures, "v2", "manifest.schema.json"), v2ManifestSHA256)
	assertFileSHA256(t, filepath.Join(fixtures, "v2", "claim-correction.json"), v2FixtureSHA256)

	schemaBytes, err := os.ReadFile(filepath.Join(fixtures, "v3", "manifest.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Const int `json:"const"`
		} `json:"properties"`
		Definitions map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("parse v3 schema: %v", err)
	}
	if schema.Properties["fixture_schema_version"].Const != 3 {
		t.Fatal("v3 fixture schema does not require version 3")
	}
	for _, definition := range []string{"lifecycleChange", "effectV3", "proposalV3", "operationV3", "operation"} {
		if len(schema.Definitions[definition]) == 0 {
			t.Fatalf("v3 fixture schema omits %s", definition)
		}
	}
	assertJSONContains(t, schema.Definitions["operation"], `../v1/manifest.schema.json#/$defs/operation`)
	assertJSONContains(t, schema.Definitions["operation"], `../v2/manifest.schema.json#/$defs/operationV2`)

	fixtureBytes, err := os.ReadFile(filepath.Join(fixtures, "v3", "lifecycle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int `json:"fixture_schema_version"`
		Operations    []struct {
			SchemaVersion  int                        `json:"schema_version"`
			Proposal       canonicalLifecycleProposal `json:"proposal"`
			ProposalSHA256 string                     `json:"proposal_sha256"`
			EffectSHA256   string                     `json:"effect_sha256"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("parse v3 lifecycle fixture: %v", err)
	}
	if fixture.SchemaVersion != 3 || len(fixture.Operations) != 4 {
		t.Fatalf("v3 fixture envelope = %+v", fixture)
	}
	wantKinds := []string{"retire_memory", "restore_memory", "retract_source", "restore_source"}
	for index, operation := range fixture.Operations {
		if operation.SchemaVersion != 3 || operation.Proposal.Kind != wantKinds[index] ||
			len(operation.Proposal.Effect.Transitions) == 0 || len(operation.Proposal.Effect.LifecycleChange) != 1 {
			t.Fatalf("v3 operation %d is incomplete: %+v", index, operation)
		}
		change := operation.Proposal.Effect.LifecycleChange[0]
		if kind, err := lifecycleKind(change.Action); err != nil || kind != operation.Proposal.Kind ||
			operation.Proposal.Effect.Transitions[0].ObjectKind != string(change.ObjectKind) ||
			operation.Proposal.Effect.Transitions[0].ObjectID != change.ObjectID {
			t.Fatalf("v3 operation %d target/action mismatch: %+v", index, operation)
		}
		proposalHash, _, err := semanticHash(operation.Proposal)
		if err != nil {
			t.Fatal(err)
		}
		effectHash, _, err := semanticHash(operation.Proposal.Effect)
		if err != nil {
			t.Fatal(err)
		}
		if proposalHash != operation.ProposalSHA256 || effectHash != operation.EffectSHA256 {
			t.Fatalf("v3 operation %d canonical hashes = %s / %s, fixture = %s / %s",
				index, proposalHash, effectHash, operation.ProposalSHA256, operation.EffectSHA256)
		}
	}
}

type correctionEffectArrayRule struct {
	MinItems    *int `json:"minItems"`
	MaxItems    *int `json:"maxItems"`
	PrefixItems []struct {
		AllOf []struct {
			Properties map[string]struct {
				Const string `json:"const"`
			} `json:"properties"`
		} `json:"allOf"`
	} `json:"prefixItems"`
}

type closedCorrectionEffectRules struct {
	Arrays      map[string]correctionEffectArrayRule
	Transitions []struct{ ObjectKind, State string }
}

func parseClosedCorrectionEffectRules(t *testing.T, raw json.RawMessage) closedCorrectionEffectRules {
	t.Helper()
	var definition struct {
		Required   []string                             `json:"required"`
		Properties map[string]correctionEffectArrayRule `json:"properties"`
	}
	if err := json.Unmarshal(raw, &definition); err != nil {
		t.Fatal(err)
	}
	wantFields := []string{"scopes", "predicates", "entities", "aliases", "claims", "source_links", "graph_links", "transitions", "corrections"}
	if len(definition.Required) != len(wantFields) {
		t.Fatalf("v2 effect required fields = %v", definition.Required)
	}
	rules := closedCorrectionEffectRules{Arrays: definition.Properties}
	transitionRule := definition.Properties["transitions"]
	for _, prefix := range transitionRule.PrefixItems {
		var transition struct{ ObjectKind, State string }
		for _, part := range prefix.AllOf {
			if value := part.Properties["object_kind"].Const; value != "" {
				transition.ObjectKind = value
			}
			if value := part.Properties["state"].Const; value != "" {
				transition.State = value
			}
		}
		rules.Transitions = append(rules.Transitions, transition)
	}
	if len(rules.Transitions) != 3 {
		t.Fatalf("v2 transition schema = %+v", rules.Transitions)
	}
	return rules
}

func validateClosedCorrectionEffect(rules closedCorrectionEffectRules, effect canonicalCorrectionEffect) error {
	lengths := map[string]int{
		"scopes": len(effect.Scopes), "predicates": len(effect.Predicates), "entities": len(effect.Entities),
		"aliases": len(effect.Aliases), "claims": len(effect.Claims), "source_links": len(effect.SourceLinks),
		"graph_links": len(effect.GraphLinks), "transitions": len(effect.Transitions), "corrections": len(effect.Corrections),
	}
	for field, count := range lengths {
		rule, ok := rules.Arrays[field]
		if !ok {
			return fmt.Errorf("missing schema rule for %s", field)
		}
		if rule.MinItems != nil && count < *rule.MinItems {
			return fmt.Errorf("%s has %d items below minimum %d", field, count, *rule.MinItems)
		}
		if rule.MaxItems != nil && count > *rule.MaxItems {
			return fmt.Errorf("%s has %d items above maximum %d", field, count, *rule.MaxItems)
		}
	}
	for i, want := range rules.Transitions {
		if i >= len(effect.Transitions) || effect.Transitions[i].ObjectKind != want.ObjectKind ||
			string(effect.Transitions[i].State) != want.State {
			return fmt.Errorf("transition %d does not match schema", i)
		}
	}
	return nil
}

func cloneCorrectionEffect(t *testing.T, effect canonicalCorrectionEffect) canonicalCorrectionEffect {
	t.Helper()
	encoded, err := json.Marshal(effect)
	if err != nil {
		t.Fatal(err)
	}
	var clone canonicalCorrectionEffect
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func assertFileSHA256(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(contents))
	if got != want {
		t.Fatalf("frozen v1 contract %s SHA-256 = %s, want %s", path, got, want)
	}
}

func assertJSONContains(t *testing.T, raw json.RawMessage, want string) {
	t.Helper()
	if !json.Valid(raw) || !strings.Contains(string(raw), want) {
		t.Fatalf("JSON definition does not contain %q: %s", want, raw)
	}
}
