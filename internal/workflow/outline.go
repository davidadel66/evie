package workflow

import (
	"errors"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// StepOutline is one step of a definition as the owner reads it in the web
// UI (workspace-activity.spec.md): its id, kind and description, the steps
// it can lead to, its retry ceiling, and the lines of workflow.yaml that
// define it.
type StepOutline struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Description string   `json:"description,omitempty"`
	Next        []string `json:"next,omitempty"`
	MaxAttempts int      `json:"max_attempts,omitempty"`
	FirstLine   int      `json:"first_line"`
	LastLine    int      `json:"last_line"`
}

// DefinitionOutline is a definition's description and its steps in YAML
// order.
type DefinitionOutline struct {
	Description string        `json:"description,omitempty"`
	Steps       []StepOutline `json:"steps"`
}

// Outline reads workflow.yaml's description and steps under the same
// accepted subset as validation, without validating anything else, so any
// version that was once approved can still be outlined.
func Outline(data []byte) (DefinitionOutline, error) {
	def, problems := parseDefinition(data)
	if len(problems) > 0 {
		return DefinitionOutline{}, errors.New("workflow.yaml: " + problems[0].Message)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return DefinitionOutline{}, err
	}
	items := stepNodes(&doc)
	out := make([]StepOutline, len(def.Steps))
	for i, s := range def.Steps {
		o := StepOutline{ID: s.ID, Kind: s.Kind, Description: s.Description}
		add := func(next string) {
			if next != "" && !slices.Contains(o.Next, next) {
				o.Next = append(o.Next, next)
			}
		}
		add(s.Next)
		for _, c := range s.Cases {
			add(c.Next)
		}
		add(s.Default)
		if s.Retry != nil {
			o.MaxAttempts = s.Retry.MaxAttempts
		}
		if i < len(items) {
			o.FirstLine, o.LastLine = items[i].Line, lastLine(items[i])
		}
		out[i] = o
	}
	return DefinitionOutline{Description: def.Description, Steps: out}, nil
}

// stepNodes is the steps sequence's items.
func stepNodes(doc *yaml.Node) []*yaml.Node {
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "steps" && root.Content[i+1].Kind == yaml.SequenceNode {
			return root.Content[i+1].Content
		}
	}
	return nil
}

// lastLine is the last line a node's text occupies, counting a block
// scalar's lines, which start on the line after its indicator.
func lastLine(n *yaml.Node) int {
	last := n.Line
	if n.Kind == yaml.ScalarNode && n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		last += 1 + strings.Count(strings.TrimRight(n.Value, "\n"), "\n")
	}
	for _, c := range n.Content {
		last = max(last, lastLine(c))
	}
	return last
}
