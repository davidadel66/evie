package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind is a value type's kind.
type Kind string

const (
	KindAny     Kind = "any"
	KindString  Kind = "string"
	KindInteger Kind = "integer"
	KindNumber  Kind = "number"
	KindBoolean Kind = "boolean"
	KindObject  Kind = "object"
	KindArray   Kind = "array"
)

// Type is the closed type system shared by JSON schemas, function
// signatures and capability contracts. Values are canonical: string, int64,
// float64, bool, []any and map[string]any.
type Type struct {
	Kind       Kind             `json:"kind"`
	Properties map[string]*Type `json:"properties,omitempty"`
	Required   []string         `json:"required,omitempty"`
	// Open objects accept fields they do not declare. Schemas, results and
	// contracts are closed; function parameters are open so a richer value
	// can be passed.
	Open      bool     `json:"open,omitempty"`
	Items     *Type    `json:"items,omitempty"`
	Enum      []any    `json:"enum,omitempty"`
	Minimum   *float64 `json:"minimum,omitempty"`
	Maximum   *float64 `json:"maximum,omitempty"`
	MinLength *int64   `json:"min_length,omitempty"`
	MaxLength *int64   `json:"max_length,omitempty"`
	MinItems  *int64   `json:"min_items,omitempty"`
	MaxItems  *int64   `json:"max_items,omitempty"`
}

// Field is one object property for ObjectType.
type Field struct {
	Name     string
	Type     *Type
	Optional bool
}

func AnyType() *Type     { return &Type{Kind: KindAny} }
func StringType() *Type  { return &Type{Kind: KindString} }
func IntegerType() *Type { return &Type{Kind: KindInteger} }
func NumberType() *Type  { return &Type{Kind: KindNumber} }
func BooleanType() *Type { return &Type{Kind: KindBoolean} }

// ArrayType is a list of items.
func ArrayType(items *Type) *Type { return &Type{Kind: KindArray, Items: items} }

// ObjectType is a closed object; fields are required unless Optional.
func ObjectType(fields ...Field) *Type {
	t := &Type{Kind: KindObject, Properties: map[string]*Type{}}
	for _, f := range fields {
		t.Properties[f.Name] = f.Type
		if !f.Optional {
			t.Required = append(t.Required, f.Name)
		}
	}
	sort.Strings(t.Required)
	return t
}

// openObject is an open object, for function parameters.
func openObject(fields ...Field) *Type {
	t := ObjectType(fields...)
	t.Open = true
	return t
}

func (t *Type) isAny() bool { return t == nil || t.Kind == KindAny }

func (t *Type) requires(name string) bool { return slices.Contains(t.Required, name) }

func (t *Type) clone() *Type {
	if t == nil {
		return nil
	}
	c := *t
	if t.Properties != nil {
		c.Properties = map[string]*Type{}
		for k, v := range t.Properties {
			c.Properties[k] = v.clone()
		}
	}
	c.Required = slices.Clone(t.Required)
	c.Items = t.Items.clone()
	return &c
}

// describe names a type briefly for messages and reports.
func (t *Type) describe() string {
	switch {
	case t.isAny():
		return "any value"
	case t.Kind == KindArray:
		return "list of " + t.Items.describe()
	case t.Kind == KindObject:
		names := sortedKeys(t.Properties)
		if len(names) > 4 {
			names = append(names[:4], "…")
		}
		return "object{" + strings.Join(names, ", ") + "}"
	}
	return string(t.Kind)
}

// field returns the type of one named field.
func (t *Type) field(name string) (*Type, bool) {
	if t.isAny() {
		return AnyType(), true
	}
	if t.Kind != KindObject {
		return nil, false
	}
	f, ok := t.Properties[name]
	if !ok && t.Open {
		return AnyType(), true
	}
	return f, ok
}

// assignable reports why a value of type src cannot be used where dst is
// expected, or nil. A field dst requires must also be required by src.
func assignable(src, dst *Type) error {
	if dst.isAny() || src.isAny() {
		return nil
	}
	if dst.Kind == KindNumber && src.Kind == KindInteger {
		return nil
	}
	if src.Kind != dst.Kind {
		return fmt.Errorf("%s is not %s", src.describe(), dst.describe())
	}
	switch dst.Kind {
	case KindArray:
		if err := assignable(src.Items, dst.Items); err != nil {
			return fmt.Errorf("items: %w", err)
		}
	case KindObject:
		for _, name := range sortedKeys(dst.Properties) {
			s, ok := src.Properties[name]
			if !ok {
				if dst.requires(name) {
					return fmt.Errorf("missing field %s", name)
				}
				continue
			}
			if dst.requires(name) && !src.requires(name) {
				return fmt.Errorf("field %s may be absent", name)
			}
			if err := assignable(s, dst.Properties[name]); err != nil {
				return fmt.Errorf("field %s: %w", name, err)
			}
		}
	}
	return nil
}

// checkValue validates a canonical value against a type.
func checkValue(t *Type, v any) error { return checkAt(t, v, "") }

func checkAt(t *Type, v any, path string) error {
	at := func(format string, args ...any) error {
		where := path
		if where == "" {
			where = "value"
		}
		return fmt.Errorf("%s: %s", where, fmt.Sprintf(format, args...))
	}
	if v == nil {
		if t.isAny() {
			return nil
		}
		return at("expected %s, got nothing", t.describe())
	}
	if t.isAny() {
		return nil
	}
	switch t.Kind {
	case KindString:
		s, ok := v.(string)
		if !ok {
			return at("expected string, got %s", valueKind(v))
		}
		n := int64(utf8.RuneCountInString(s))
		if t.MinLength != nil && n < *t.MinLength {
			return at("shorter than %d characters", *t.MinLength)
		}
		if t.MaxLength != nil && n > *t.MaxLength {
			return at("longer than %d characters", *t.MaxLength)
		}
	case KindInteger:
		if _, ok := v.(int64); !ok {
			return at("expected integer, got %s", valueKind(v))
		}
	case KindNumber:
		switch v.(type) {
		case int64, float64:
		default:
			return at("expected number, got %s", valueKind(v))
		}
	case KindBoolean:
		if _, ok := v.(bool); !ok {
			return at("expected boolean, got %s", valueKind(v))
		}
	case KindArray:
		list, ok := v.([]any)
		if !ok {
			return at("expected list, got %s", valueKind(v))
		}
		n := int64(len(list))
		if t.MinItems != nil && n < *t.MinItems {
			return at("has %d items; at least %d", n, *t.MinItems)
		}
		if t.MaxItems != nil && n > *t.MaxItems {
			return at("has %d items; at most %d", n, *t.MaxItems)
		}
		for i, item := range list {
			if err := checkAt(t.Items, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case KindObject:
		m, ok := v.(map[string]any)
		if !ok {
			return at("expected object, got %s", valueKind(v))
		}
		for _, name := range t.Required {
			if _, ok := m[name]; !ok {
				return at("missing field %s", name)
			}
		}
		for _, name := range sortedKeys(m) {
			ft, declared := t.Properties[name]
			if !declared {
				if t.Open {
					continue
				}
				return at("unexpected field %s", name)
			}
			if err := checkAt(ft, m[name], joinPath(path, name)); err != nil {
				return err
			}
		}
	}
	if n, ok := number(v); ok && (t.Kind == KindInteger || t.Kind == KindNumber) {
		if t.Minimum != nil && n < *t.Minimum {
			return at("below the minimum %s", formatNumber(*t.Minimum))
		}
		if t.Maximum != nil && n > *t.Maximum {
			return at("above the maximum %s", formatNumber(*t.Maximum))
		}
	}
	if len(t.Enum) > 0 && !slices.ContainsFunc(t.Enum, func(e any) bool { return equalValues(e, v) }) {
		return at("%s is not one of %s", formatValue(v), formatEnum(t.Enum))
	}
	return nil
}

func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func valueKind(v any) string {
	switch v.(type) {
	case nil:
		return "nothing"
	case string:
		return "string"
	case int64:
		return "integer"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "list"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

// typeOfValue is the static type of a literal.
func typeOfValue(v any) *Type {
	switch x := v.(type) {
	case string:
		return StringType()
	case int64:
		return IntegerType()
	case float64:
		return NumberType()
	case bool:
		return BooleanType()
	case []any:
		if len(x) == 0 {
			return ArrayType(AnyType())
		}
		return ArrayType(typeOfValue(x[0]))
	case map[string]any:
		fields := []Field{}
		for _, k := range sortedKeys(x) {
			fields = append(fields, Field{Name: k, Type: typeOfValue(x[k])})
		}
		return ObjectType(fields...)
	}
	return AnyType()
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func equalValues(a, b any) bool {
	if x, ok := number(a); ok {
		y, ok := number(b)
		return ok && x == y
	}
	ca, err1 := canonicalBytes(a)
	cb, err2 := canonicalBytes(b)
	return err1 == nil && err2 == nil && bytes.Equal(ca, cb)
}

func formatNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func formatValue(v any) string {
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	data, err := canonicalBytes(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

func formatEnum(values []any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = formatValue(v)
	}
	return strings.Join(parts, ", ")
}

// canonicalBytes is the canonical JSON encoding of a canonical value:
// object keys sorted, no HTML escaping, no trailing newline.
func canonicalBytes(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

var integerText = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// parseCanonicalJSON decodes one JSON document into canonical values:
// integers without a fraction or exponent become int64, other numbers
// float64.
func parseCanonicalJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("more than one JSON value")
	}
	return canonicalJSON(v)
}

func canonicalJSON(v any) (any, error) {
	switch x := v.(type) {
	case json.Number:
		s := x.String()
		if integerText.MatchString(s) {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("integer %s is out of range", s)
			}
			return n, nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("number %s is out of range", s)
		}
		return f, nil
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			c, err := canonicalJSON(item)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			c, err := canonicalJSON(item)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	}
	return v, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStrings(s []string) []string {
	out := slices.Clone(s)
	sort.Strings(out)
	return out
}

func ptr[T any](v T) *T { return &v }
