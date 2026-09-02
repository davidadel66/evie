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
	v3ManifestSHA256 = "39b31c674edd5f3646ebbca6bd7758c4030ee9fa964aa3adfeb7ffb9599bda97"
	v3FixtureSHA256  = "e8d76202fc041aa62fe5eb8ebf4423f3fc3e62609a9bc73abce26ece47e7794e"
	v4ManifestSHA256 = "ff38511891b0ad885dd9e58bc782780b8beefad5d40d2cbcee12be5a043e802c"
	v4FixtureSHA256  = "85809af1b4af6580bee822138c6b31e889acc877d7f5d1b3887199cf096abc75"
	v4ReuseSHA256    = "88c181d1e9016278fb4682874257fbb79f478676f1f470f5b27769b221f7ba76"
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

func TestSemanticPromotionEncodingV4FixtureAndFrozenPriorContracts(t *testing.T) {
	fixtures := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "semantic-memory")
	for path, want := range map[string]string{
		filepath.Join(fixtures, "v1", "manifest.schema.json"):  v1ManifestSHA256,
		filepath.Join(fixtures, "v1", "literal-claim.json"):    v1FixtureSHA256,
		filepath.Join(fixtures, "v2", "manifest.schema.json"):  v2ManifestSHA256,
		filepath.Join(fixtures, "v2", "claim-correction.json"): v2FixtureSHA256,
		filepath.Join(fixtures, "v3", "manifest.schema.json"):  v3ManifestSHA256,
		filepath.Join(fixtures, "v3", "lifecycle.json"):        v3FixtureSHA256,
	} {
		assertFileSHA256(t, path, want)
	}

	schemaBytes, err := os.ReadFile(filepath.Join(fixtures, "v4", "manifest.schema.json"))
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
		t.Fatal(err)
	}
	if schema.Properties["fixture_schema_version"].Const != 4 {
		t.Fatal("v4 fixture schema does not require version 4")
	}
	for _, definition := range []string{"entityMapping", "promotion", "effectV4", "proposalV4", "operationV4", "operation"} {
		if len(schema.Definitions[definition]) == 0 {
			t.Fatalf("v4 fixture schema omits %s", definition)
		}
	}
	var effectRules struct {
		Properties map[string]struct {
			Type     string `json:"type"`
			MinItems *int   `json:"minItems"`
			MaxItems *int   `json:"maxItems"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema.Definitions["effectV4"], &effectRules); err != nil {
		t.Fatal(err)
	}
	if minimum := effectRules.Properties["source_links"].MinItems; minimum != nil && *minimum > 0 {
		t.Fatalf("v4 fixture schema rejects an accepted all-reuse Promotion with zero created Source Links: minItems=%d", *minimum)
	}
	if effectRules.Properties["source_links"].Type != "array" ||
		promotionArrayCountAllowed(effectRules.Properties["claims"].MinItems, effectRules.Properties["claims"].MaxItems, 2) ||
		promotionArrayCountAllowed(effectRules.Properties["predicates"].MinItems, effectRules.Properties["predicates"].MaxItems, 1) {
		t.Fatal("v4 fixture schema failed positive/negative Promotion effect array bounds")
	}
	for _, prior := range []string{
		`../v1/manifest.schema.json#/$defs/operation`,
		`../v2/manifest.schema.json#/$defs/operationV2`,
		`../v3/manifest.schema.json#/$defs/operationV3`,
	} {
		assertJSONContains(t, schema.Definitions["operation"], prior)
	}

	fixtureBytes, err := os.ReadFile(filepath.Join(fixtures, "v4", "scope-promotion.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int `json:"fixture_schema_version"`
		Operations    []struct {
			SchemaVersion  int                        `json:"schema_version"`
			Proposal       canonicalPromotionProposal `json:"proposal"`
			ProposalSHA256 string                     `json:"proposal_sha256"`
			EffectSHA256   string                     `json:"effect_sha256"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 4 || len(fixture.Operations) != 1 || fixture.Operations[0].SchemaVersion != 4 {
		t.Fatalf("v4 fixture envelope = %+v", fixture)
	}
	operation := fixture.Operations[0]
	if operation.Proposal.Kind != "promote_claim" || len(operation.Proposal.PriorRevisions) != 2 ||
		len(operation.Proposal.Effect.Promotions) != 1 || len(operation.Proposal.Effect.Claims) != 1 ||
		len(operation.Proposal.Effect.SourceLinks) != 1 {
		t.Fatalf("v4 Promotion effect is incomplete: %+v", operation.Proposal.Effect)
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
		t.Fatalf("v4 canonical hashes = %s / %s, fixture = %s / %s",
			proposalHash, effectHash, operation.ProposalSHA256, operation.EffectSHA256)
	}

	reuseBytes, err := os.ReadFile(filepath.Join(fixtures, "v4", "all-reuse-scope-promotion.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reuseFixture struct {
		SchemaVersion int `json:"fixture_schema_version"`
		Operations    []struct {
			SchemaVersion  int                        `json:"schema_version"`
			Proposal       canonicalPromotionProposal `json:"proposal"`
			ProposalSHA256 string                     `json:"proposal_sha256"`
			EffectSHA256   string                     `json:"effect_sha256"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(reuseBytes, &reuseFixture); err != nil {
		t.Fatal(err)
	}
	if reuseFixture.SchemaVersion != 4 || len(reuseFixture.Operations) != 1 || reuseFixture.Operations[0].SchemaVersion != 4 {
		t.Fatalf("v4 all-reuse fixture envelope = %+v", reuseFixture)
	}
	reuse := reuseFixture.Operations[0]
	if len(reuse.Proposal.Effect.Entities) != 0 || len(reuse.Proposal.Effect.Claims) != 0 ||
		len(reuse.Proposal.Effect.SourceLinks) != 0 || len(reuse.Proposal.Effect.Promotions) != 1 {
		t.Fatalf("v4 all-reuse Promotion effect = %+v", reuse.Proposal.Effect)
	}
	reuseProposalHash, _, err := semanticHash(reuse.Proposal)
	if err != nil {
		t.Fatal(err)
	}
	reuseEffectHash, _, err := semanticHash(reuse.Proposal.Effect)
	if err != nil {
		t.Fatal(err)
	}
	if reuseProposalHash != reuse.ProposalSHA256 || reuseEffectHash != reuse.EffectSHA256 {
		t.Fatalf("v4 all-reuse canonical hashes = %s / %s, fixture = %s / %s",
			reuseProposalHash, reuseEffectHash, reuse.ProposalSHA256, reuse.EffectSHA256)
	}
}

func TestSemanticGraphLinkEncodingV5FixtureAndFrozenPriorContracts(t *testing.T) {
	fixtures := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "semantic-memory")
	for path, want := range map[string]string{
		filepath.Join(fixtures, "v1", "manifest.schema.json"):           v1ManifestSHA256,
		filepath.Join(fixtures, "v1", "literal-claim.json"):             v1FixtureSHA256,
		filepath.Join(fixtures, "v2", "manifest.schema.json"):           v2ManifestSHA256,
		filepath.Join(fixtures, "v2", "claim-correction.json"):          v2FixtureSHA256,
		filepath.Join(fixtures, "v3", "manifest.schema.json"):           v3ManifestSHA256,
		filepath.Join(fixtures, "v3", "lifecycle.json"):                 v3FixtureSHA256,
		filepath.Join(fixtures, "v4", "manifest.schema.json"):           v4ManifestSHA256,
		filepath.Join(fixtures, "v4", "scope-promotion.json"):           v4FixtureSHA256,
		filepath.Join(fixtures, "v4", "all-reuse-scope-promotion.json"): v4ReuseSHA256,
	} {
		assertFileSHA256(t, path, want)
	}
	schemaBytes, err := os.ReadFile(filepath.Join(fixtures, "v5", "manifest.schema.json"))
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
		t.Fatal(err)
	}
	if schema.Properties["fixture_schema_version"].Const != 5 {
		t.Fatal("v5 fixture schema does not require version 5")
	}
	for _, definition := range []string{"endpoint", "graphLinkV5", "effectV5", "proposalV5", "operationV5", "graphLifecycleChangeV5", "graphLifecycleEffectV5", "graphLifecycleProposalV5", "graphLifecycleOperationV5", "compoundGraphLifecycleChangeV5", "compoundGraphLifecycleEffectV5", "compoundGraphLifecycleProposalV5", "compoundGraphLifecycleOperationV5", "operation"} {
		if len(schema.Definitions[definition]) == 0 {
			t.Fatalf("v5 fixture schema omits %s", definition)
		}
	}
	for _, prior := range []string{`../v1/manifest.schema.json#/$defs/operation`, `../v2/manifest.schema.json#/$defs/operationV2`, `../v3/manifest.schema.json#/$defs/operationV3`, `../v4/manifest.schema.json#/$defs/operationV4`} {
		assertJSONContains(t, schema.Definitions["operation"], prior)
	}
	fixtureBytes, err := os.ReadFile(filepath.Join(fixtures, "v5", "graph-links.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int `json:"fixture_schema_version"`
		Operations    []struct {
			SchemaVersion  int                    `json:"schema_version"`
			Proposal       canonicalGraphProposal `json:"proposal"`
			ProposalSHA256 string                 `json:"proposal_sha256"`
			EffectSHA256   string                 `json:"effect_sha256"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 5 || len(fixture.Operations) != 1 || fixture.Operations[0].SchemaVersion != 5 {
		t.Fatalf("v5 fixture envelope = %+v", fixture)
	}
	operation := fixture.Operations[0]
	if operation.Proposal.Kind != "create_graph_link" || len(operation.Proposal.Effect.GraphLinks) != 1 || len(operation.Proposal.Effect.Transitions) != 1 || operation.Proposal.Effect.GraphLinks[0].Relation != "contradiction" {
		t.Fatalf("v5 Graph Link effect is incomplete: %+v", operation.Proposal.Effect)
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
		t.Fatalf("v5 canonical hashes = %s / %s, fixture = %s / %s", proposalHash, effectHash, operation.ProposalSHA256, operation.EffectSHA256)
	}
	compoundBytes, err := os.ReadFile(filepath.Join(fixtures, "v5", "compound-entity-lifecycle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var compound struct {
		SchemaVersion int `json:"fixture_schema_version"`
		Operations    []struct {
			SchemaVersion  int                        `json:"schema_version"`
			Proposal       canonicalLifecycleProposal `json:"proposal"`
			ProposalSHA256 string                     `json:"proposal_sha256"`
			EffectSHA256   string                     `json:"effect_sha256"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(compoundBytes, &compound); err != nil {
		t.Fatal(err)
	}
	if compound.SchemaVersion != 5 || len(compound.Operations) != 1 || compound.Operations[0].SchemaVersion != 5 {
		t.Fatalf("v5 compound lifecycle fixture envelope = %+v", compound)
	}
	compoundOperation := compound.Operations[0]
	if compoundOperation.Proposal.Kind != "retire_memory" || len(compoundOperation.Proposal.Effect.Transitions) != 4 ||
		compoundOperation.Proposal.Effect.Transitions[0].ObjectKind != "entity" ||
		compoundOperation.Proposal.Effect.Transitions[3].ObjectKind != "graph_link" {
		t.Fatalf("v5 compound lifecycle effect is incomplete: %+v", compoundOperation.Proposal.Effect)
	}
	compoundProposalHash, _, err := semanticHash(compoundOperation.Proposal)
	if err != nil {
		t.Fatal(err)
	}
	compoundEffectHash, _, err := semanticHash(compoundOperation.Proposal.Effect)
	if err != nil {
		t.Fatal(err)
	}
	if compoundProposalHash != compoundOperation.ProposalSHA256 || compoundEffectHash != compoundOperation.EffectSHA256 {
		t.Fatalf("v5 compound lifecycle hashes = %s / %s, fixture = %s / %s", compoundProposalHash, compoundEffectHash, compoundOperation.ProposalSHA256, compoundOperation.EffectSHA256)
	}
}

func promotionArrayCountAllowed(minimum, maximum *int, count int) bool {
	return (minimum == nil || count >= *minimum) && (maximum == nil || count <= *maximum)
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
