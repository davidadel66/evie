package workspacetabs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// writeTab writes a tab folder with the given files under folder.
func writeTab(t *testing.T, folder, id string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(folder, Dir, id)
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func valid(title string, order int) map[string]string {
	return map[string]string{"tab.json": fmt.Sprintf(`{"title":%q,"order":%d}`, title, order), "index.html": "<p>" + title + "</p>"}
}

func problems(tabs []Tab) map[string]string {
	out := map[string]string{}
	for _, tab := range tabs {
		out[tab.ID] = tab.Problem
	}
	return out
}

func TestListShowsValidTabsInOrderAndExplainsInvalidOnes(t *testing.T) {
	folder := t.TempDir()
	writeTab(t, folder, "reports", valid("Reports", 20))
	writeTab(t, folder, "budget", valid("Budget", 10))
	writeTab(t, folder, "same-order", valid("Same order", 10))
	writeTab(t, folder, "unknown-field", map[string]string{"tab.json": `{"title":"X","layout":"wide"}`, "index.html": "x"})
	writeTab(t, folder, "no-title", map[string]string{"tab.json": `{"order":1}`, "index.html": "x"})
	writeTab(t, folder, "long-title", map[string]string{"tab.json": `{"title":"` + strings.Repeat("t", 25) + `"}`, "index.html": "x"})
	writeTab(t, folder, "two-objects", map[string]string{"tab.json": `{"title":"A"} {}`, "index.html": "x"})
	writeTab(t, folder, "no-index", map[string]string{"tab.json": `{"title":"No index"}`})
	writeTab(t, folder, "Bad_Name", valid("Bad", 0))
	writeTab(t, folder, ".hidden", valid("Hidden", 0))
	if err := os.WriteFile(filepath.Join(folder, Dir, "stray.txt"), []byte("not a tab"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(folder, Dir, "reports"), filepath.Join(folder, Dir, "linked")); err != nil {
		t.Fatal(err)
	}
	tabs, err := List(folder)
	if err != nil {
		t.Fatal(err)
	}
	var shown []string
	for _, tab := range tabs {
		if tab.Problem == "" {
			shown = append(shown, tab.Title)
		}
	}
	if strings.Join(shown, ",") != "Budget,Same order,Reports" {
		t.Fatalf("shown tabs = %v", shown)
	}
	got := problems(tabs)
	for id, want := range map[string]string{
		"unknown-field": `unknown field "layout"`, "no-title": "needs a title", "long-title": "1–24 characters",
		"two-objects": "one JSON object", "no-index": "index.html is missing", "Bad_Name": "lowercase letters",
		"linked": "cannot be a link",
	} {
		if !strings.Contains(got[id], want) {
			t.Errorf("%s problem = %q, want it to mention %q", id, got[id], want)
		}
	}
	if _, listed := got[".hidden"]; listed {
		t.Error("a dot folder was listed")
	}
	if _, listed := got["stray.txt"]; listed {
		t.Error("a file was listed as a tab")
	}
}

func TestListWithoutAFolderOrTabsIsEmpty(t *testing.T) {
	for _, folder := range []string{"", t.TempDir()} {
		tabs, err := List(folder)
		if err != nil || tabs == nil || len(tabs) != 0 {
			t.Fatalf("List(%q) = %v, %v", folder, tabs, err)
		}
	}
}

func TestListLimitsTabCountAndFolderSize(t *testing.T) {
	folder := t.TempDir()
	for i := range MaxTabs + 1 {
		writeTab(t, folder, fmt.Sprintf("tab-%02d", i), valid(fmt.Sprintf("Tab %d", i), i))
	}
	many := valid("Many", 100)
	for i := range MaxTabFiles {
		many[fmt.Sprintf("f/%d.txt", i)] = "x"
	}
	writeTab(t, folder, "many-files", many)
	big := valid("Big", 101)
	big["big.bin"] = strings.Repeat("x", MaxTabBytes)
	writeTab(t, folder, "big", big)
	tabs, err := List(folder)
	if err != nil {
		t.Fatal(err)
	}
	got := problems(tabs)
	if got["tab-11"] != "" || !strings.Contains(got["tab-12"], "at most 12 tabs") {
		t.Fatalf("tab-11 = %q, tab-12 = %q", got["tab-11"], got["tab-12"])
	}
	if !strings.Contains(got["many-files"], "over 200 files") || !strings.Contains(got["big"], "over 5 MB") {
		t.Fatalf("many-files = %q, big = %q", got["many-files"], got["big"])
	}
	if _, _, err := Open(folder, "tab-12", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("open the 13th tab: %v", err)
	}
}

func TestOpenServesOnlyFilesInsideAValidTab(t *testing.T) {
	folder := t.TempDir()
	files := valid("Reports", 1)
	files["app.js"] = "console.log(1)"
	files["lib/chart.js"] = "chart"
	files[".env"] = "SECRET=1"
	dir := writeTab(t, folder, "reports", files)
	writeTab(t, folder, "broken", map[string]string{"index.html": "no manifest"})
	if err := os.WriteFile(filepath.Join(folder, "secret.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(folder, "secret.txt"), filepath.Join(dir, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("app.js", filepath.Join(dir, "alias.js")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".env", filepath.Join(dir, "public.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("lib", filepath.Join(dir, "linked-lib")); err != nil {
		t.Fatal(err)
	}
	read := func(rel string) (string, error) {
		file, _, err := Open(folder, "reports", rel)
		if err != nil {
			return "", err
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		return string(data), err
	}
	for rel, want := range map[string]string{"": "<p>Reports</p>", "index.html": "<p>Reports</p>", "lib/chart.js": "chart"} {
		if got, err := read(rel); err != nil || got != want {
			t.Errorf("read %q = %q, %v", rel, got, err)
		}
	}
	// Only regular files reached without a link: a link inside the tab could
	// serve a dotfile (public.txt -> .env).
	for _, rel := range []string{"../../secret.txt", "/etc/passwd", "lib/../app.js", "./app.js", ".env", "lib/", "a\\b", "escape.txt", "alias.js", "public.txt", "linked-lib/chart.js"} {
		if _, err := read(rel); !errors.Is(err, ErrPath) {
			t.Errorf("read %q: err = %v, want ErrPath", rel, err)
		}
	}
	if _, err := read("lib"); !errors.Is(err, ErrPath) {
		t.Errorf("read a directory: %v", err)
	}
	if _, err := read("missing.js"); !errors.Is(err, ErrNotFound) {
		t.Errorf("read a missing file: %v", err)
	}
	for _, id := range []string{"missing", "../reports", "Reports"} {
		if _, _, err := Open(folder, id, ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("open tab %q: %v", id, err)
		}
	}
	if _, _, err := Open(folder, "broken", ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("open an invalid tab: %v", err)
	}
	if _, _, err := Open("", "reports", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("open without a folder: %v", err)
	}
}

func TestATabsFolderLinkedOutsideTheWorkspaceIsRefused(t *testing.T) {
	folder, outside := t.TempDir(), t.TempDir()
	writeTab(t, outside, "reports", valid("Reports", 1))
	if err := os.Symlink(filepath.Join(outside, ".evie"), filepath.Join(folder, ".evie")); err != nil {
		t.Fatal(err)
	}
	if _, err := List(folder); err == nil {
		t.Fatal("listed tabs through a link that leaves the Workspace folder")
	}
	if _, _, err := Open(folder, "reports", ""); err == nil {
		t.Fatal("opened a tab through a link that leaves the Workspace folder")
	}
}

// A pipe where a file should be must not hang the list or a request.
func TestPipesInATabAreRefusedWithoutBlocking(t *testing.T) {
	folder := t.TempDir()
	dir := writeTab(t, folder, "reports", valid("Reports", 1))
	if err := syscall.Mkfifo(filepath.Join(dir, "data.json"), 0o644); err != nil {
		t.Fatal(err)
	}
	piped := filepath.Join(folder, Dir, "piped")
	if err := os.MkdirAll(piped, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(piped, "tab.json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(piped, "index.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() {
		tabs, err := List(folder)
		_, _, openErr := Open(folder, "reports", "data.json")
		done <- fmt.Sprintf("%v|%v|%v", problems(tabs)["piped"], err, errors.Is(openErr, ErrPath))
	}()
	select {
	case got := <-done:
		if got != "tab.json must be a regular file|<nil>|true" {
			t.Fatalf("pipes = %s", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a pipe in a tab folder blocked the reader")
	}
}

func TestFoldersCountTowardATabsLimitAndTabsHoldsABoundedNumberOfEntries(t *testing.T) {
	folder := t.TempDir()
	dir := writeTab(t, folder, "nested", valid("Nested", 1))
	for i := range MaxTabFiles {
		if err := os.Mkdir(filepath.Join(dir, fmt.Sprintf("d%03d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tabs, err := List(folder)
	if err != nil || !strings.Contains(problems(tabs)["nested"], "over 200 files and folders") {
		t.Fatalf("nested = %v, %v", tabs, err)
	}
	crowded := t.TempDir()
	for i := range maxEntries + 1 {
		if err := os.MkdirAll(filepath.Join(crowded, Dir, fmt.Sprintf("t%03d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := List(crowded); !errors.Is(err, ErrTooMany) {
		t.Fatalf("crowded .evie/tabs: %v", err)
	}
}

func TestATabDeclaresTheDataItMayReadAndItsKeyPinsTheList(t *testing.T) {
	folder := t.TempDir()
	declared := `{"title":"Reports","data":[{"db":"finance"},{"files":"eval/**/*.json"},{"runs":true}]}`
	writeTab(t, folder, "reports", map[string]string{"tab.json": declared, "index.html": "x"})
	for id, manifest := range map[string]string{
		"two-kinds":   `{"title":"A","data":[{"db":"finance","runs":true}]}`,
		"no-kind":     `{"title":"A","data":[{}]}`,
		"bad-db":      `{"title":"A","data":[{"db":"Finance DB"}]}`,
		"escape-glob": `{"title":"A","data":[{"files":"../secrets/*"}]}`,
		"dot-glob":    `{"title":"A","data":[{"files":".evie/**"}]}`,
		"abs-glob":    `{"title":"A","data":[{"files":"/etc/*"}]}`,
		"extra-field": `{"title":"A","data":[{"db":"finance","write":true}]}`,
	} {
		writeTab(t, folder, id, map[string]string{"tab.json": manifest, "index.html": "x"})
	}
	tabs, err := List(folder)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Tab{}
	for _, tab := range tabs {
		byID[tab.ID] = tab
	}
	reports := byID["reports"]
	if reports.Problem != "" || len(reports.Data) != 3 || reports.Data[0].DB != "finance" || reports.Data[1].Files != "eval/**/*.json" || !reports.Data[2].Runs || len(reports.DataKey) != 64 {
		t.Fatalf("reports = %+v", reports)
	}
	for _, id := range []string{"two-kinds", "no-kind", "bad-db", "escape-glob", "dot-glob", "abs-glob", "extra-field"} {
		if !strings.HasPrefix(byID[id].Problem, "data: ") && !strings.Contains(byID[id].Problem, "unknown field") {
			t.Errorf("%s problem = %q", id, byID[id].Problem)
		}
	}
	writeTab(t, folder, "reports", map[string]string{"tab.json": `{"title":"Reports","data":[{"db":"finance"}]}`})
	again, _ := List(folder)
	for _, tab := range again {
		if tab.ID == "reports" && (tab.DataKey == reports.DataKey || tab.DataKey == "") {
			t.Fatalf("a changed data list kept its key: %q", tab.DataKey)
		}
	}
}

func TestReadFileReadsOnlyDeclaredFilesInTheWorkspace(t *testing.T) {
	folder := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(folder, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("eval/run-1/metrics.json", `{"loss":0.4}`)
	write("eval/summary.json", `{"best":1}`)
	write("eval/notes.txt", "not json")
	write("eval/api_token.json", `{"token":"x"}`)
	write("eval/.hidden.json", `{}`)
	write("train/data.json", `{}`)
	write("eval/big.json", strings.Repeat("x", MaxFileBytes+1))
	if err := os.Symlink(filepath.Join(folder, "train/data.json"), filepath.Join(folder, "eval/linked.json")); err != nil {
		t.Fatal(err)
	}
	globs := []string{"eval/**/*.json"}
	for rel, want := range map[string]string{"eval/run-1/metrics.json": `{"loss":0.4}`, "eval/summary.json": `{"best":1}`} {
		if got, err := ReadFile(folder, rel, globs); err != nil || string(got) != want {
			t.Errorf("read %s = %q, %v", rel, got, err)
		}
	}
	for rel, why := range map[string]string{
		"eval/notes.txt":          "declares no files matching",
		"eval/api_token.json":     "look like secrets",
		"eval/.hidden.json":       "no dot folders",
		"train/data.json":         "declares no files matching",
		"eval/linked.json":        "reached without a link",
		"eval/big.json":           "over 1 MB",
		"eval/../train/data.json": "no dot folders",
		"/etc/passwd":             "relative to the Workspace folder",
	} {
		if _, err := ReadFile(folder, rel, globs); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), why) {
			t.Errorf("read %s: err = %v, want a refusal saying %q", rel, err, why)
		}
	}
	if _, err := ReadFile(folder, "eval/summary.json", nil); !errors.Is(err, ErrRefused) {
		t.Errorf("read with no declared files: %v", err)
	}
}

// Secrets and raw databases stay out whatever the glob: by any folder or
// file name, by a SQLite header, or through a hard link.
func TestReadFileRefusesSecretsDatabasesAndHardLinks(t *testing.T) {
	folder, outside := t.TempDir(), t.TempDir()
	write := func(dir, rel, content string) string {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, rel := range []string{"secrets/prod.json", "credentials/gcp.json", "config/api_key.json", "prod.env", "finance.db", "finance.db-journal", "auth.json", "keys/a.json", "private/notes.json"} {
		write(folder, rel, "{}")
		if _, err := ReadFile(folder, rel, []string{"**"}); !errors.Is(err, ErrRefused) {
			t.Errorf("read %s: %v", rel, err)
		}
	}
	write(folder, "data/ledger.json", "SQLite format 3\x00rest")
	if _, err := ReadFile(folder, "data/ledger.json", []string{"**"}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "SQLite database") {
		t.Errorf("a SQLite file under another name: %v", err)
	}
	key := write(outside, "id", "PRIVATE KEY")
	if err := os.Link(key, filepath.Join(folder, "data", "notes.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(folder, "data/notes.json", []string{"**"}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "hard link") {
		t.Errorf("a hard link: %v", err)
	}
	write(folder, "eval/authors.json", `{"a":1}`)
	if got, err := ReadFile(folder, "eval/authors.json", []string{"**"}); err != nil || string(got) != `{"a":1}` {
		t.Errorf("an ordinary file = %q, %v", got, err)
	}
}

// Matching costs pattern × path, so a pattern of many ** cannot stall.
func TestGlobMatchingIsBoundedAndCapped(t *testing.T) {
	long := strings.Repeat("a/", 31) + "b"
	start := time.Now()
	if matchGlob("**/x/**/x/**/x/**/x/**/x/**/x/**/z", long) {
		t.Fatal("matched")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("matching took %s", elapsed)
	}
	for pattern, want := range map[string]bool{"eval/**/*.json": true, "**": true, "eval/*.json": false, "**/run-*/**": true} {
		if got := matchGlob(pattern, "eval/run-1/deep/metrics.json"); got != want {
			t.Errorf("matchGlob(%q) = %v", pattern, got)
		}
	}
	if matchGlob("**", strings.Repeat("a/", 40)+"b") {
		t.Error("a path of over 32 segments matched")
	}
	for _, pattern := range []string{"**/**/x", strings.Repeat("a/", 16) + "b"} {
		if validGlob(pattern) {
			t.Errorf("validGlob(%q) accepted", pattern)
		}
	}
}

func TestATabsOwnReservedFolderIsNeverServed(t *testing.T) {
	folder := t.TempDir()
	files := valid("Reports", 1)
	files["_evie/tab.js"] = "fake helper"
	writeTab(t, folder, "reports", files)
	for _, rel := range []string{"_evie/tab.js", "_evie"} {
		if _, _, err := Open(folder, "reports", rel); !errors.Is(err, ErrPath) {
			t.Errorf("open %s: %v", rel, err)
		}
	}
}

// A tab can show one of Evie's own views by name instead of a page.
func TestABuiltinTabNamesOneOfEviesViews(t *testing.T) {
	folder := t.TempDir()
	writeTab(t, folder, "spending", map[string]string{"tab.json": `{"title":"Spending","order":5,"builtin":"spending"}`})
	writeTab(t, folder, "unknown", map[string]string{"tab.json": `{"title":"X","builtin":"payroll"}`})
	writeTab(t, folder, "with-data", map[string]string{"tab.json": `{"title":"X","builtin":"spending","data":[{"db":"finance"}]}`})
	tabs, err := List(folder)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Tab{}
	for _, tab := range tabs {
		byID[tab.ID] = tab
	}
	if tab := byID["spending"]; tab.Problem != "" || tab.Builtin != "spending" || tab.Title != "Spending" || tab.Order != 5 {
		t.Fatalf("spending = %+v", tab)
	}
	if !strings.Contains(byID["unknown"].Problem, "not one of Evie's built-in views") || !strings.Contains(byID["with-data"].Problem, "leave out data") {
		t.Fatalf("unknown = %q, with-data = %q", byID["unknown"].Problem, byID["with-data"].Problem)
	}
	if _, _, err := Open(folder, "spending", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a built-in tab served a page: %v", err)
	}
}
