package memory

import (
	"encoding/json"
	"errors"
	"slices"
)

// The appended prompt is immutable under this policy ID. Change the ID when changing its text.
const CompilerScopePolicyV1 = "memory-applicability-v1"

const MemoryScopeRecommendationInstructions = `Recommend a destination for each supported memory candidate: "everywhere", "workspace", or "session". Choose from the meaning, not merely where it was mentioned or who it describes. Use everywhere for enduring general personal preferences, workspace for facts limited to the current workspace or project, and session for temporary conversational instructions. "I prefer concise answers" is everywhere; "For finance, show the calculations" is workspace; "Keep this conversation brief" is session. General is a workspace name, not Global. Honor explicit qualifiers; quoted examples, hypotheticals and other people's preferences are not global facts about the owner. Prefer the narrower supported scope when uncertain. Never invent a workspace or session ID. Destination is a recommendation requiring owner approval; preserve exact evidence and keep source context separate. Omit candidates that are unsupported or not worth remembering.`

func CompilerSystemPrompt(g CompilerGeneration) string {
	if g.ScopePolicy == CompilerScopePolicyV1 {
		return g.Prompt + "\n\n" + MemoryScopeRecommendationInstructions
	}
	return g.Prompt
}

// Scope-aware generations require an explicit destination in the candidate schema.
// Reject incompatible pinned schemas before dispatch rather than producing output
// the kernel must reject. This policy uses the inline candidates/items shape.
func validateCompilerScopeSchema(schema json.RawMessage) error {
	var root struct {
		Properties struct {
			Candidates struct {
				Items struct {
					Properties map[string]json.RawMessage
					Required   []string
				}
			}
		}
	}
	if err := json.Unmarshal(schema, &root); err != nil {
		return errors.New("scope policy requires an inline candidate schema")
	}
	item := root.Properties.Candidates.Items
	raw, ok := item.Properties["destination"]
	var d struct {
		Type string
		Enum []string
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return errors.New("invalid destination schema")
	}
	if !ok || d.Type != "string" || !slices.Contains(item.Required, "destination") || len(d.Enum) != 3 || !slices.Contains(d.Enum, "everywhere") || !slices.Contains(d.Enum, "workspace") || !slices.Contains(d.Enum, "session") {
		return errors.New("scope policy requires candidates.items.destination with the everywhere, workspace, session enum and required field")
	}
	return nil
}
