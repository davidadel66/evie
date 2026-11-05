package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// testdata/inputshash/vision-py.json holds, for each case, the files of a
// repository, the map's meta, and the inputs hash the vision skill's own
// script (scripts/vision.py: inputs_hash over read_proposals) computed for
// them. The check must agree with it exactly, or every page the skill
// builds would look stale. Evie's own inputs agree too, since check passes
// on the page vision.py built from them. CRLF cases are written by the test,
// because the repository checks testdata out with LF.
func TestInputsHashAgreesWithTheVisionSkill(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "inputshash", "vision-py.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name  string
		Meta  struct{ Facts, Changelog string }
		Files map[string]string
		Dirs  []string
		Hash  string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("want at least 10 cases, got %d", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range c.Dirs {
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for name, text := range c.Files {
				writeFile(t, root, name, text)
			}
			proposals, err := readProposals(root)
			if err != nil {
				t.Fatal(err)
			}
			got, err := inputsHash(root, c.Meta.Facts, c.Meta.Changelog, proposals)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.Hash {
				t.Errorf("inputsHash = %s, want %s", got, c.Hash)
			}
		})
	}
}
