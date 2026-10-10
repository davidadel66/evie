package delegation

import (
	"strings"
	"testing"
)

// changes.spec.md story 4a (2026-10-10): the parent's title and description
// of a build's Change become its pull request's, so they are bounded,
// printable and free of the owner's local paths.
func TestChangeTextIsBoundedAndNamesNoLocalPath(t *testing.T) {
	good := ChangeText{Title: "Add a footer to the README", Description: "Adds one line to README.md.\n\n- Checked with `test -s README.md`."}
	if err := good.Check(); err != nil {
		t.Fatal(err)
	}
	if err := (ChangeText{}).Check(); err != nil {
		t.Fatalf("no text (an older chat's proposal) = %v", err)
	}
	for name, text := range map[string]ChangeText{
		"no title":               {Description: "Adds a footer."},
		"no description":         {Title: "Add a footer"},
		"two-line title":         {Title: "Add a footer\nand more", Description: "Adds a footer."},
		"long title":             {Title: strings.Repeat("a", MaxChangeTitleRunes+1), Description: "Adds a footer."},
		"long description":       {Title: "Add a footer", Description: strings.Repeat("a", MaxChangeDescriptionBytes+1)},
		"control in title":       {Title: "Add a \x1b[31mfooter", Description: "Adds a footer."},
		"control in description": {Title: "Add a footer", Description: "Adds a \x00footer."},
	} {
		if err := text.Check(); err == nil {
			t.Fatalf("%s: %+v was accepted", name, text)
		}
	}
	if long := (ChangeText{Title: strings.Repeat("é", MaxChangeTitleRunes), Description: "Adds a footer."}); long.Check() != nil {
		t.Fatal("the title bound counts bytes, not characters")
	}

	worktree := "/Users/me/code/notes/.evie/worktrees/1a2b3c4d"
	if got := good.LocalPath("/Users/me", worktree); got != "" {
		t.Fatalf("good text names %q", got)
	}
	named := ChangeText{Title: "Add a footer", Description: "Edited " + worktree + "/README.md."}
	if got := named.LocalPath("/Users/me", worktree); got != "/Users/me" {
		t.Fatalf("LocalPath = %q", got)
	}
	if got := named.LocalPath("", worktree); got != worktree {
		t.Fatalf("LocalPath without a home = %q", got)
	}
	if got := (ChangeText{Title: "Fix /", Description: "Root."}).LocalPath("/", ""); got != "" {
		t.Fatalf("the root folder counts as a local path: %q", got)
	}
}
