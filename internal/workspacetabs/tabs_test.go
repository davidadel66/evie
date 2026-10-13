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
	writeTab(t, folder, "unknown-field", map[string]string{"tab.json": `{"title":"X","data":[]}`, "index.html": "x"})
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
		"unknown-field": `unknown field "data"`, "no-title": "needs a title", "long-title": "1–24 characters",
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
