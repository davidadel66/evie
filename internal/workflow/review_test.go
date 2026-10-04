package workflow

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The reviewed files are exactly workflow.yaml, README.md and the prompts
// and schemas the definition references: nothing else in its directory is
// proposed, committed or approved with it.
func TestWorkflowFilesAreExactlyTheReviewedDefinition(t *testing.T) {
	dir := copyDefinition(t, "notes-digest")
	writeFile(t, dir, "notes.txt", "scratch notes beside the definition\n")
	writeFile(t, dir, "prompts/unused.md", "not referenced\n")
	v := mustValidate(t, dir, testEnvironment())
	files := v.Files()
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	want := []string{"README.md", "prompts/summarize.md", "schemas/ask.schema.json", "schemas/digest.schema.json", "workflow.yaml"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("files = %v, want %v", names, want)
	}
	if !strings.Contains(string(files["README.md"]), "# Notes digest") || !strings.Contains(string(files["workflow.yaml"]), "name: notes-digest") {
		t.Fatalf("file content was not the definition's: %q", files["README.md"])
	}
	if v.Name() != "notes-digest" {
		t.Fatalf("name = %q", v.Name())
	}
	// The map is a copy: changing it cannot change the validated definition.
	files["workflow.yaml"] = []byte("changed")
	if string(v.Files()["workflow.yaml"]) == "changed" {
		t.Fatal("Files returned the validated definition's own map")
	}
}

// Requirements are what one approval binds besides the executable hash: the
// requested Standing Authority with its own hash, the worker presets agent
// steps run and the schedule the same approval activates.
func TestWorkflowRequirementsBindAuthorityWorkersAndSchedule(t *testing.T) {
	notes := RequirementsOf(mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment()).Report())
	if len(notes.Authority) != 2 || notes.Authority[0].Capability != "notes.append" || notes.Authority[1].Capability != "notes.list" {
		t.Fatalf("authority = %+v", notes.Authority)
	}
	if !strings.HasPrefix(notes.AuthorityHash, "sha256:") || len(notes.AuthorityHash) != len("sha256:")+64 {
		t.Fatalf("authority hash = %q", notes.AuthorityHash)
	}
	if len(notes.Workers) != 0 || notes.Schedule == nil || notes.Schedule.Timezone != "America/New_York" {
		t.Fatalf("workers %v schedule %+v", notes.Workers, notes.Schedule)
	}
	research := RequirementsOf(mustValidate(t, filepath.Join("testdata", "deep-research"), testEnvironment()).Report())
	if !reflect.DeepEqual(research.Workers, []string{"research"}) || research.Schedule != nil || len(research.Authority) != 0 {
		t.Fatalf("deep-research requirements = %+v", research)
	}
	if research.AuthorityHash == notes.AuthorityHash || research.AuthorityHash != AuthorityHash(nil) {
		t.Fatalf("empty authority hash = %s", research.AuthorityHash)
	}

	// Listing the operations in another order is the same authority.
	dir := copyDefinition(t, "notes-digest")
	list := "    - capability: notes.list\n      connection: conn-notes\n      resources: [\"notebook:inbox\"]\n      max_calls_per_run: 1\n"
	edit(t, dir, "workflow.yaml", list, "")
	edit(t, dir, "workflow.yaml", "      max_calls_per_run: 1\n\nschedule:", "      max_calls_per_run: 1\n"+list+"\nschedule:")
	if got := RequirementsOf(mustValidate(t, dir, testEnvironment()).Report()).AuthorityHash; got != notes.AuthorityHash {
		t.Fatalf("reordered operations hashed %s, want %s", got, notes.AuthorityHash)
	}
	// A larger call bound is different authority.
	edit(t, dir, "workflow.yaml", "      resources: [\"notebook:inbox\"]\n      max_calls_per_run: 1\n", "      resources: [\"notebook:inbox\"]\n      max_calls_per_run: 2\n")
	if got := RequirementsOf(mustValidate(t, dir, testEnvironment()).Report()).AuthorityHash; got == notes.AuthorityHash {
		t.Fatal("a larger call bound kept the authority hash")
	}
}

// Requested Standing Authority must stay within Workspace Access: every
// operation's connection and each of its resources.
func TestWorkflowAuthorityOutsideAccess(t *testing.T) {
	ops := RequirementsOf(mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment()).Report()).Authority
	full := WorkspaceAccess{Connections: []ConnectionAccess{{ID: "conn-notes", Resources: []string{"notebook:inbox", "notebook:digest"}}}}
	if outside := AuthorityOutside(ops, full); len(outside) != 0 {
		t.Fatalf("outside full access: %v", outside)
	}
	narrowed := WorkspaceAccess{Connections: []ConnectionAccess{{ID: "conn-notes", Resources: []string{"notebook:inbox"}}}}
	outside := AuthorityOutside(ops, narrowed)
	if len(outside) != 1 || !strings.Contains(outside[0], "notebook:digest") || !strings.Contains(outside[0], "notes.append") {
		t.Fatalf("narrowed: %v", outside)
	}
	if outside := AuthorityOutside(ops, WorkspaceAccess{}); len(outside) != 2 || !strings.Contains(outside[0], "conn-notes") {
		t.Fatalf("empty access: %v", outside)
	}
	if outside := AuthorityOutside(nil, WorkspaceAccess{}); len(outside) != 0 {
		t.Fatalf("no authority: %v", outside)
	}
}

// Workspace Access is stored canonically: connections and resources sorted,
// with no duplicates, empty entries or malformed selectors.
func TestWorkflowNormalizeAccess(t *testing.T) {
	got, err := NormalizeAccess(WorkspaceAccess{Connections: []ConnectionAccess{
		{ID: "conn-sheets", Resources: []string{"sheet:tips", "sheet:hours"}},
		{ID: "conn-notes", Resources: []string{"notebook:inbox"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := WorkspaceAccess{Connections: []ConnectionAccess{
		{ID: "conn-notes", Resources: []string{"notebook:inbox"}},
		{ID: "conn-sheets", Resources: []string{"sheet:hours", "sheet:tips"}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized = %+v", got)
	}
	if empty, err := NormalizeAccess(WorkspaceAccess{}); err != nil || len(empty.Connections) != 0 {
		t.Fatalf("empty access = %+v, %v", empty, err)
	}
	for name, bad := range map[string]WorkspaceAccess{
		"duplicate connection": {Connections: []ConnectionAccess{{ID: "c", Resources: []string{"r"}}, {ID: "c", Resources: []string{"s"}}}},
		"duplicate resource":   {Connections: []ConnectionAccess{{ID: "c", Resources: []string{"r", "r"}}}},
		"no resources":         {Connections: []ConnectionAccess{{ID: "c"}}},
		"blank connection":     {Connections: []ConnectionAccess{{ID: "", Resources: []string{"r"}}}},
		"spaced selector":      {Connections: []ConnectionAccess{{ID: "c", Resources: []string{"two words"}}}},
		"control character":    {Connections: []ConnectionAccess{{ID: "c\x01", Resources: []string{"r"}}}},
	} {
		if _, err := NormalizeAccess(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func reportOf(t *testing.T, dir string) Report {
	t.Helper()
	return mustValidate(t, dir, testEnvironment()).Report()
}

// The review diff separates executable changes, by report section, from
// authority changes and documentation; a README-only change has no
// executable or authority change.
func TestWorkflowReviewDiff(t *testing.T) {
	base := reportOf(t, filepath.Join("testdata", "notes-digest"))

	created := DiffReports(nil, base)
	if !created.ExecutableChanged || created.FromHash != "" || created.ToHash != base.ExecutableHash {
		t.Fatalf("new definition diff = %+v", created)
	}
	if len(created.Authority) != 2 || !strings.HasPrefix(created.Authority[0], "+ notes.append on conn-notes") {
		t.Fatalf("new definition authority = %v", created.Authority)
	}

	same := DiffReports(&base, base)
	if same.ExecutableChanged || len(same.Executable) != 0 || len(same.Authority) != 0 || same.DocumentationChanged {
		t.Fatalf("identical diff = %+v", same)
	}

	readme := copyDefinition(t, "notes-digest")
	writeFile(t, readme, "README.md", "# Notes digest\n\nNow better explained.\n")
	docs := DiffReports(&base, reportOf(t, readme))
	if docs.ExecutableChanged || len(docs.Executable) != 0 || len(docs.Authority) != 0 || !docs.DocumentationChanged {
		t.Fatalf("README-only diff = %+v", docs)
	}
	if !strings.Contains(docs.Text(), "No executable change") {
		t.Fatalf("README-only text:\n%s", docs.Text())
	}

	changed := copyDefinition(t, "notes-digest")
	writeFile(t, changed, "prompts/summarize.md", "Summarize the notes in three bullets.\n")
	edit(t, changed, "workflow.yaml", "  max_tokens: 200000\n", "  max_tokens: 300000\n")
	edit(t, changed, "workflow.yaml", "      resources: [\"notebook:digest\"]\n      max_calls_per_run: 1\n", "      resources: [\"notebook:digest\"]\n      max_calls_per_run: 2\n")
	edit(t, changed, "workflow.yaml", "  cron: \"0 7 * * 1-5\"\n", "  cron: \"30 6 * * 1-5\"\n")
	d := DiffReports(&base, reportOf(t, changed))
	if !d.ExecutableChanged || d.FromHash != base.ExecutableHash || d.ToHash == base.ExecutableHash {
		t.Fatalf("changed diff hashes = %+v", d)
	}
	sections := map[string]SectionDiff{}
	for _, s := range d.Executable {
		sections[s.Section] = s
	}
	for _, want := range []string{"Limits", "Prompts and schemas", "Schedule"} {
		if _, ok := sections[want]; !ok {
			t.Fatalf("diff lacks section %s: %+v", want, d.Executable)
		}
	}
	if s := sections["Limits"]; len(s.Removed) != 1 || len(s.Added) != 1 || !strings.Contains(s.Added[0], "300,000") {
		t.Fatalf("limits diff = %+v", s)
	}
	if len(d.Authority) != 1 || !strings.HasPrefix(d.Authority[0], "~ notes.append on conn-notes") || !strings.Contains(d.Authority[0], "1 -> 2") {
		t.Fatalf("authority diff = %v", d.Authority)
	}
	text := d.Text()
	for _, want := range []string{"Executable changes", "Authority changes", "~ notes.append on conn-notes", "+ ", "- "} {
		if !strings.Contains(text, want) {
			t.Fatalf("diff text lacks %q:\n%s", want, text)
		}
	}

	// Reading a second notebook widens the requested authority.
	widened := copyDefinition(t, "notes-digest")
	edit(t, widened, "workflow.yaml", "      resources: [\"notebook:inbox\"]\n      max_calls_per_run: 1\n", "      resources: [\"notebook:inbox\", \"notebook:digest\"]\n      max_calls_per_run: 2\n")
	edit(t, widened, "workflow.yaml", "    next: check\n\n  - id: check", "    next: read2\n\n  - id: read2\n    kind: capability\n    capability: notes.list\n    contract: 1.0.0\n    connection: conn-notes\n    resource: \"notebook:digest\"\n    input:\n      day: $inputs.day\n    next: check\n\n  - id: check")
	wider := DiffReports(&base, reportOf(t, widened))
	found := false
	for _, line := range wider.Authority {
		if strings.HasPrefix(line, "~ notes.list on conn-notes") && strings.Contains(line, "+notebook:digest") {
			found = true
		}
	}
	if !found {
		t.Fatalf("widened resources diff = %v", wider.Authority)
	}
}
