package workflow

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// The accepted YAML subset (2026-10-04). workflow.yaml is one document of
// mappings, sequences and scalars. Anything that can smuggle structure
// (anchors, aliases, merge keys, explicit tags, duplicate keys) or silently
// convert a value (nulls, YAML 1.1 booleans, hexadecimal, octal, exponents,
// dates, a number where text is expected) is refused with its line, so the
// reviewed text means exactly one thing.
var (
	plainInteger = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	plainDecimal = regexp.MustCompile(`^-?(0|[1-9][0-9]*)\.[0-9]+$`)
	// ambiguousWords read as booleans in YAML 1.1 and as text in YAML 1.2.
	ambiguousWords = map[string]bool{"y": true, "n": true, "yes": true, "no": true, "on": true, "off": true}
)

var yamlNodeType = reflect.TypeOf(yaml.Node{})

// parseDefinition parses workflow.yaml under the accepted subset.
func parseDefinition(data []byte) (*Definition, []Problem) {
	const file = "workflow.yaml"
	if len(data) > maxDefinitionBytes {
		return nil, []Problem{{Path: file, Message: fmt.Sprintf("is %d KiB; at most %d KiB", (len(data)+1023)>>10, maxDefinitionBytes>>10)}}
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, []Problem{{Path: file, Message: "is empty"}}
		}
		return nil, []Problem{{Path: file, Message: err.Error()}}
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, []Problem{{Path: file, Message: "must contain exactly one YAML document"}}
	}
	var problems []Problem
	checkNodes(&doc, &problems)
	if len(problems) > 0 {
		return nil, problems
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	typeCheck(root, reflect.TypeOf(Definition{}), "", &problems)
	if len(problems) > 0 {
		return nil, problems
	}
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	var def Definition
	if err := strict.Decode(&def); err != nil {
		return nil, []Problem{{Path: file, Message: err.Error()}}
	}
	recordStepKeys(root, &def)
	return &def, nil
}

// checkNodes applies the structural rules every node must follow.
func checkNodes(n *yaml.Node, problems *[]Problem) {
	add := func(format string, args ...any) {
		*problems = append(*problems, Problem{Path: "workflow.yaml", Message: fmt.Sprintf("line %d: ", n.Line) + fmt.Sprintf(format, args...)})
	}
	if n.Anchor != "" {
		add("anchors (&%s) are not accepted; write the value out", n.Anchor)
	}
	if n.Kind != yaml.DocumentNode && n.Kind != yaml.AliasNode && n.Style&yaml.TaggedStyle != 0 {
		add("explicit tag %s is not accepted", n.Tag)
		return
	}
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			checkNodes(c, problems)
		}
	case yaml.AliasNode:
		add("aliases (*%s) are not accepted; write the value out", n.Value)
	case yaml.MappingNode:
		seen := map[string]int{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			switch {
			case k.Kind == yaml.ScalarNode && (k.Tag == "!!merge" || (k.Value == "<<" && k.Style == 0)):
				*problems = append(*problems, Problem{Path: "workflow.yaml", Message: fmt.Sprintf("line %d: merge keys (<<) are not accepted", k.Line)})
				continue
			case k.Kind != yaml.ScalarNode || k.ShortTag() != "!!str":
				*problems = append(*problems, Problem{Path: "workflow.yaml", Message: fmt.Sprintf("line %d: mapping keys must be text", k.Line)})
			default:
				if first, dup := seen[k.Value]; dup {
					*problems = append(*problems, Problem{Path: "workflow.yaml", Message: fmt.Sprintf("line %d: duplicate key %s (first at line %d)", k.Line, k.Value, first)})
				} else {
					seen[k.Value] = k.Line
				}
			}
			checkNodes(k, problems)
			checkNodes(v, problems)
		}
	case yaml.ScalarNode:
		if n.Style != 0 {
			return // quoted, literal or folded: text
		}
		switch n.ShortTag() {
		case "!!null":
			add("null values are not accepted; leave the field out")
		case "!!bool":
			if n.Value != "true" && n.Value != "false" {
				add("%s: write booleans as true or false", n.Value)
			}
		case "!!int":
			if !plainInteger.MatchString(n.Value) {
				add("%s: write integers in plain decimal", n.Value)
			} else if _, err := strconv.ParseInt(n.Value, 10, 64); err != nil {
				add("%s is out of range", n.Value)
			}
		case "!!float":
			if !plainDecimal.MatchString(n.Value) {
				add("%s: write decimals as digits with one point, such as 0.5", n.Value)
			}
		case "!!timestamp":
			add("%s reads as a date; quote it", n.Value)
		case "!!str":
			if ambiguousWords[strings.ToLower(n.Value)] {
				add("%s is read differently by YAML 1.1 and 1.2; quote it, or write true or false", n.Value)
			}
		default:
			add("%s is not an accepted value", n.Value)
		}
	}
}

// typeCheck walks the node tree beside the Go type it decodes into, refusing
// unknown fields and any scalar whose YAML type differs from the field's.
func typeCheck(n *yaml.Node, t reflect.Type, path string, problems *[]Problem) {
	where := path
	if where == "" {
		where = "workflow.yaml"
	}
	add := func(format string, args ...any) {
		*problems = append(*problems, Problem{Path: where, Message: fmt.Sprintf("line %d: ", n.Line) + fmt.Sprintf(format, args...)})
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == yamlNodeType {
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		if n.Kind != yaml.MappingNode {
			add("expected a mapping")
			return
		}
		fields := map[string]reflect.StructField{}
		for i := range t.NumField() {
			f := t.Field(i)
			if name, _, _ := strings.Cut(f.Tag.Get("yaml"), ","); name != "" && name != "-" {
				fields[name] = f
			}
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			f, ok := fields[k.Value]
			if !ok {
				*problems = append(*problems, Problem{Path: where, Message: fmt.Sprintf("line %d: field %s not found; the schema is closed", k.Line, k.Value)})
				continue
			}
			typeCheck(v, f.Type, joinPath(path, k.Value), problems)
		}
	case reflect.Map:
		if n.Kind != yaml.MappingNode {
			add("expected a mapping")
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			typeCheck(n.Content[i+1], t.Elem(), joinPath(path, n.Content[i].Value), problems)
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			add("expected a list")
			return
		}
		for i, c := range n.Content {
			elemPath := fmt.Sprintf("%s[%d]", path, i)
			if t.Elem() == reflect.TypeOf(Step{}) {
				if id := mappingValue(c, "id"); id != "" {
					elemPath = joinPath(path, id)
				}
			}
			typeCheck(c, t.Elem(), elemPath, problems)
		}
	case reflect.String:
		if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!str" {
			add("expected text; quote %q to write it as text", n.Value)
		}
	case reflect.Bool:
		if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!bool" {
			add("expected true or false")
		}
	case reflect.Int, reflect.Int64:
		if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!int" {
			add("expected an integer in plain decimal")
		}
	case reflect.Float64:
		if n.Kind != yaml.ScalarNode || (n.ShortTag() != "!!int" && n.ShortTag() != "!!float") {
			add("expected a number")
		}
	}
}

func mappingValue(n *yaml.Node, key string) string {
	if n.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key && n.Content[i+1].Kind == yaml.ScalarNode {
			return n.Content[i+1].Value
		}
	}
	return ""
}

func mappingChild(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func mappingKeys(n *yaml.Node) map[string]bool {
	keys := map[string]bool{}
	if n != nil && n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			keys[n.Content[i].Value] = true
		}
	}
	return keys
}

// recordStepKeys notes which fields each step and item wrote, so validation
// can refuse fields that belong to another kind.
func recordStepKeys(root *yaml.Node, def *Definition) {
	steps := mappingChild(root, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return
	}
	for i, n := range steps.Content {
		if i >= len(def.Steps) {
			return
		}
		def.Steps[i].keys = mappingKeys(n)
		if item := mappingChild(n, "item"); item != nil && def.Steps[i].Item != nil {
			def.Steps[i].Item.keys = mappingKeys(item)
		}
	}
}

// nodeValue converts a checked YAML value into a canonical value.
func nodeValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		switch n.ShortTag() {
		case "!!str":
			return n.Value, nil
		case "!!int":
			return strconv.ParseInt(n.Value, 10, 64)
		case "!!float":
			return strconv.ParseFloat(n.Value, 64)
		case "!!bool":
			return n.Value == "true", nil
		}
		return nil, fmt.Errorf("line %d: %s is not an accepted value", n.Line, n.Value)
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := nodeValue(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.MappingNode:
		out := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			v, err := nodeValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			out[n.Content[i].Value] = v
		}
		return out, nil
	}
	return nil, fmt.Errorf("line %d: unsupported value", n.Line)
}
