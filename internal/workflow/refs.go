package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

// Reference roots. A reference is the only way a value flows between steps;
// there is no expression language, indexing or templating.
const (
	refInputs   = "inputs"
	refSteps    = "steps"
	refItem     = "item"
	refSchedule = "schedule"
)

var refSegment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Ref is a parsed reference: $inputs.<name>…, $steps.<id>.output…, $item…
// (inside a map item) or $schedule.date / $schedule.time (in schedule inputs).
type Ref struct {
	Raw  string
	Root string
	// Name is the input name or step ID.
	Name string
	// Path is the field path after the input, step output or item.
	Path []string
}

const refHelp = "use $inputs.<name>, $steps.<id>.output or $item, with dotted field names"

func isRefText(s string) bool { return strings.HasPrefix(s, "$") }

func parseRef(s string) (Ref, error) {
	bad := func() (Ref, error) {
		return Ref{}, fmt.Errorf("%s is not a reference; %s", s, refHelp)
	}
	if !strings.HasPrefix(s, "$") {
		return bad()
	}
	parts := strings.Split(s[1:], ".")
	for _, p := range parts {
		if !refSegment.MatchString(p) {
			return bad()
		}
	}
	r := Ref{Raw: s, Root: parts[0]}
	switch r.Root {
	case refInputs:
		if len(parts) < 2 {
			return bad()
		}
		r.Name, r.Path = parts[1], parts[2:]
	case refSteps:
		if len(parts) < 3 || parts[2] != "output" {
			return bad()
		}
		r.Name, r.Path = parts[1], parts[3:]
	case refItem:
		r.Path = parts[1:]
	case refSchedule:
		if len(parts) != 2 || (parts[1] != "date" && parts[1] != "time") {
			return Ref{}, fmt.Errorf("%s is not a schedule value; use $schedule.date or $schedule.time", s)
		}
		r.Name = parts[1]
	default:
		return bad()
	}
	return r, nil
}

// walkType follows a field path through a type.
func walkType(t *Type, path []string) (*Type, error) {
	for i, name := range path {
		next, ok := t.field(name)
		if !ok {
			where := "the value"
			if i > 0 {
				where = strings.Join(path[:i], ".")
			}
			return nil, fmt.Errorf("%s has no field %s (it is %s)", where, name, t.describe())
		}
		if next.Withheld != "" {
			return nil, fmt.Errorf("%s cannot be read: %s", strings.Join(path[:i+1], "."), next.Withheld)
		}
		t = next
	}
	return t, nil
}

// walkValue follows a field path through a canonical value; a missing field
// is nil.
func walkValue(v any, path []string) any {
	for _, name := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[name]
	}
	return v
}
