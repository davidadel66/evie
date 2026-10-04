package workflow

import (
	"errors"
	"fmt"
	"slices"
)

// The accepted JSON Schema subset (2026-10-04). Output, result and response
// schemas describe typed data only: no references, combinators, patterns or
// formats, and every object is closed.
var schemaKeywords = map[string]bool{
	"type": true, "properties": true, "required": true, "items": true, "enum": true,
	"minimum": true, "maximum": true, "minLength": true, "maxLength": true,
	"minItems": true, "maxItems": true, "additionalProperties": true,
	"title": true, "description": true,
}

// parseSchema reads one schema file into a Type, returning its canonical
// JSON for hashing.
func parseSchema(data []byte) (*Type, []byte, error) {
	v, err := parseCanonicalJSON(data)
	if err != nil {
		return nil, nil, fmt.Errorf("is not valid JSON: %v", err)
	}
	t, err := schemaType(v, "")
	if err != nil {
		return nil, nil, err
	}
	canonical, err := canonicalBytes(v)
	if err != nil {
		return nil, nil, err
	}
	return t, canonical, nil
}

func schemaType(v any, path string) (*Type, error) {
	where := path
	if where == "" {
		where = "root"
	}
	fail := func(format string, args ...any) (*Type, error) {
		return nil, fmt.Errorf("%s: %s", where, fmt.Sprintf(format, args...))
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fail("a schema must be an object")
	}
	for _, k := range sortedKeys(m) {
		if !schemaKeywords[k] {
			return fail("keyword %s is not supported", k)
		}
	}
	kind, _ := m["type"].(string)
	t := &Type{Kind: Kind(kind)}
	switch t.Kind {
	case KindString, KindInteger, KindNumber, KindBoolean, KindObject, KindArray:
	default:
		return fail("type must be one of string, integer, number, boolean, object or array")
	}
	only := func(kinds []Kind, keys ...string) error {
		for _, k := range keys {
			if _, present := m[k]; present && !slices.Contains(kinds, t.Kind) {
				return fmt.Errorf("%s: %s does not apply to %s", where, k, t.Kind)
			}
		}
		return nil
	}
	if err := errors.Join(
		only([]Kind{KindObject}, "properties", "required", "additionalProperties"),
		only([]Kind{KindArray}, "items", "minItems", "maxItems"),
		only([]Kind{KindString}, "minLength", "maxLength"),
		only([]Kind{KindInteger, KindNumber}, "minimum", "maximum"),
		only([]Kind{KindString, KindInteger, KindNumber, KindBoolean}, "enum"),
	); err != nil {
		return nil, err
	}
	for _, s := range []string{"title", "description"} {
		if x, present := m[s]; present {
			if _, ok := x.(string); !ok {
				return fail("%s must be text", s)
			}
		}
	}
	count := func(key string) (*int64, error) {
		x, present := m[key]
		if !present {
			return nil, nil
		}
		n, ok := x.(int64)
		if !ok || n < 0 {
			return nil, fmt.Errorf("%s: %s must be a non-negative integer", where, key)
		}
		return &n, nil
	}
	bound := func(key string) (*float64, error) {
		x, present := m[key]
		if !present {
			return nil, nil
		}
		n, ok := number(x)
		if !ok {
			return nil, fmt.Errorf("%s: %s must be a number", where, key)
		}
		return &n, nil
	}
	var err error
	if t.MinLength, err = count("minLength"); err != nil {
		return nil, err
	}
	if t.MaxLength, err = count("maxLength"); err != nil {
		return nil, err
	}
	if t.MinItems, err = count("minItems"); err != nil {
		return nil, err
	}
	if t.MaxItems, err = count("maxItems"); err != nil {
		return nil, err
	}
	if t.Minimum, err = bound("minimum"); err != nil {
		return nil, err
	}
	if t.Maximum, err = bound("maximum"); err != nil {
		return nil, err
	}
	switch t.Kind {
	case KindObject:
		if closed, ok := m["additionalProperties"].(bool); !ok || closed {
			return fail("every object must declare additionalProperties: false")
		}
		t.Properties = map[string]*Type{}
		if props, present := m["properties"]; present {
			pm, ok := props.(map[string]any)
			if !ok {
				return fail("properties must be an object")
			}
			for _, name := range sortedKeys(pm) {
				child, err := schemaType(pm[name], joinPath(path, "properties."+name))
				if err != nil {
					return nil, err
				}
				t.Properties[name] = child
			}
		}
		if req, present := m["required"]; present {
			list, ok := req.([]any)
			if !ok {
				return fail("required must be a list of property names")
			}
			for _, x := range list {
				name, ok := x.(string)
				if !ok || t.Properties[name] == nil {
					return fail("required names %v, which is not a property", x)
				}
				if slices.Contains(t.Required, name) {
					return fail("required lists %s twice", name)
				}
				t.Required = append(t.Required, name)
			}
			slices.Sort(t.Required)
		}
	case KindArray:
		items, present := m["items"]
		if !present {
			return fail("an array must declare items")
		}
		child, err := schemaType(items, joinPath(path, "items"))
		if err != nil {
			return nil, err
		}
		t.Items = child
	}
	if e, present := m["enum"]; present {
		list, ok := e.([]any)
		if !ok || len(list) == 0 {
			return fail("enum must be a non-empty list")
		}
		plain := &Type{Kind: t.Kind}
		for _, x := range list {
			if err := checkValue(plain, x); err != nil {
				return fail("enum value %s is not %s", formatValue(x), t.Kind)
			}
		}
		t.Enum = list
	}
	return t, nil
}
