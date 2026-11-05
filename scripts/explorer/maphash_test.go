package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testdata/maphash/vision-py.json holds what the vision skill's own script
// (scripts/vision.py: canonical and map_hash) produced for each input: the
// canonical form of the hashed content and the hash. The check must agree
// with it exactly, or every page the skill builds would look stale. The last
// case is the first station of Evie's real map; the whole map agrees too,
// since check passes on the page vision.py built from it.
func TestMapHashAgreesWithTheVisionSkill(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "maphash", "vision-py.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Name, Input, Canonical, Hash string }
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 9 {
		t.Fatalf("want at least 9 cases, got %d", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			value, err := decodeJSONValue([]byte(c.Input))
			if err != nil {
				t.Fatal(err)
			}
			stripUnhashed(value)
			canonical, err := canonicalJSON(value)
			if err != nil {
				t.Fatal(err)
			}
			if string(canonical) != c.Canonical {
				t.Errorf("canonical form\n got %q\nwant %q", canonical, c.Canonical)
			}
			got, err := mapHash([]byte(c.Input))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.Hash {
				t.Errorf("mapHash = %s, want %s", got, c.Hash)
			}
		})
	}
}

func TestESNumberWritesNumbersAsECMAScriptDoes(t *testing.T) {
	// Expected values are what Number.prototype.toString returns.
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"}, {math.Copysign(0, -1), "0"}, {1, "1"}, {-1, "-1"}, {0.1, "0.1"}, {0.30000000000000004, "0.30000000000000004"},
		{1e21, "1e+21"}, {1e20, "100000000000000000000"}, {123e18, "123000000000000000000"}, {1.5e300, "1.5e+300"},
		{1e-6, "0.000001"}, {1e-7, "1e-7"}, {1.5e-7, "1.5e-7"}, {-2.5e-9, "-2.5e-9"}, {123.456, "123.456"},
		{5e-324, "5e-324"}, {math.MaxFloat64, "1.7976931348623157e+308"}, {9007199254740993, "9007199254740992"},
		{4.35, "4.35"}, {100, "100"}, {1234567.0, "1234567"}, {0.000123, "0.000123"},
	}
	for _, c := range cases {
		if got := esNumber(c.in); got != c.want {
			t.Errorf("esNumber(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMapHashRefusesWhatJSONCannotCarry(t *testing.T) {
	for _, c := range []struct{ name, input, want string }{
		{"a number past the double range", `{"n": 1e400}`, "outside what an IEEE 754 double holds"},
		{"two values", `{"a": 1} {"b": 2}`, "more than one JSON value"},
		{"not JSON", `{"a": }`, "invalid character"},
	} {
		if _, err := mapHash([]byte(c.input)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: mapHash error = %v, want one containing %q", c.name, err, c.want)
		}
	}
}

func TestMapHashIgnoresLayoutAndStampFields(t *testing.T) {
	a, err := mapHash([]byte(`{"meta": {"title": "T"}, "code": {"A": {"file": "a.go", "symbol": "A"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := mapHash([]byte("{\"code\":{\"A\":{\"symbol\":\"A\",\"hash\":\"abc\",\"file\":\"a.go\",\"src\":\"func A() {}\"}},\n\"meta\":{\"stamped\":\"2026-11-04\",\"title\":\"T\",\"commit\":\"abc\"}}"))
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("stamping and reformatting changed the hash: %s, then %s", a, b)
	}
	c, err := mapHash([]byte(`{"meta": {"title": "T2"}, "code": {"A": {"file": "a.go", "symbol": "A"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c == a {
		t.Error("changing the title left the hash unchanged")
	}
}
