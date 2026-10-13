// Package workspacetabs reads a Workspace's own tabs from its folder
// (workspace-tabs.spec.md). Each tab is a folder .evie/tabs/<tab-id>/ that
// holds tab.json and index.html. Every read goes through an os.Root opened
// on the tab folder, so no path, ".." or symlink can reach outside it.
package workspacetabs

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	// Dir is where a Workspace folder keeps its tabs.
	Dir = ".evie/tabs"
	// MaxTabs is how many tabs one Workspace shows; later ones are problems.
	MaxTabs = 12
	// MaxTabBytes and MaxTabFiles bound one tab folder.
	MaxTabBytes = 5 << 20
	MaxTabFiles = 200
	// maxFolders bounds how many tab folders are examined at all.
	maxFolders = 32
	// maxManifestBytes bounds tab.json.
	maxManifestBytes = 16 << 10
	maxTitleRunes    = 24
)

var (
	ErrNotFound = errors.New("workspace tab not found")
	ErrInvalid  = errors.New("workspace tab has a problem")
	ErrPath     = errors.New("workspace tab path is not allowed")
)

var tabIDPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// Tab is one tab as the header and Settings → Tabs show it. A tab with a
// Problem is listed but never shown or served.
type Tab struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Order   int    `json:"order"`
	Problem string `json:"problem,omitempty"`
}

// manifest is tab.json, version 1. Unknown fields are a problem, so a newer
// manifest on an older Evie says so instead of being half read.
type manifest struct {
	Title *string `json:"title"`
	Order int     `json:"order"`
}

// List returns the tabs in folder's .evie/tabs, sorted by order then id.
// A folder of "" (a Workspace without a folder) has no tabs.
func List(folder string) ([]Tab, error) {
	if folder == "" {
		return []Tab{}, nil
	}
	tabs, err := openTabs(folder)
	if errors.Is(err, fs.ErrNotExist) {
		return []Tab{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer tabs.Close()
	entries, err := readDir(tabs)
	if err != nil {
		return nil, err
	}
	list := []Tab{}
	examined := 0
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			list = append(list, Tab{ID: name, Title: name, Problem: "a tab folder cannot be a link"})
		case !entry.IsDir():
			continue
		case examined >= maxFolders:
			list = append(list, Tab{ID: name, Title: name, Problem: fmt.Sprintf("Evie examines at most %d tab folders", maxFolders)})
		default:
			examined++
			list = append(list, inspect(tabs, name))
		}
	}
	slices.SortFunc(list, func(a, b Tab) int {
		return cmp.Or(cmp.Compare(a.Order, b.Order), strings.Compare(a.ID, b.ID))
	})
	shown := 0
	for i := range list {
		if list[i].Problem != "" {
			continue
		}
		if shown++; shown > MaxTabs {
			list[i].Problem = fmt.Sprintf("a Workspace shows at most %d tabs", MaxTabs)
		}
	}
	return list, nil
}

// Open opens one file of a valid tab for serving. rel is relative to the
// tab folder; "" means index.html. It refuses absolute paths, "..", dot
// segments, dotfiles, directories and anything a symlink would take outside
// the tab folder.
func Open(folder, id, rel string) (*os.File, fs.FileInfo, error) {
	if folder == "" || !tabIDPattern.MatchString(id) {
		return nil, nil, ErrNotFound
	}
	if rel == "" {
		rel = "index.html"
	}
	if !allowedPath(rel) {
		return nil, nil, ErrPath
	}
	tabs, err := List(folder)
	if err != nil {
		return nil, nil, err
	}
	i := slices.IndexFunc(tabs, func(tab Tab) bool { return tab.ID == id })
	if i < 0 {
		return nil, nil, ErrNotFound
	}
	if tabs[i].Problem != "" {
		return nil, nil, fmt.Errorf("%w: %s", ErrInvalid, tabs[i].Problem)
	}
	root, err := openTab(folder, id)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	defer root.Close()
	file, err := root.Open(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		// os.Root refuses a path that escapes the tab folder.
		return nil, nil, ErrPath
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, ErrPath
	}
	return file, info, nil
}

// allowedPath accepts a clean, relative, slash-separated path with no dot
// segment and no dotfile.
func allowedPath(rel string) bool {
	if rel == "" || strings.ContainsAny(rel, "\\\x00") || path.IsAbs(rel) || path.Clean(rel) != rel {
		return false
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" || strings.HasPrefix(segment, ".") {
			return false
		}
	}
	return true
}

func openTabs(folder string) (*os.Root, error) {
	workspace, err := os.OpenRoot(folder)
	if err != nil {
		return nil, err
	}
	defer workspace.Close()
	return workspace.OpenRoot(Dir)
}

func openTab(folder, id string) (*os.Root, error) {
	tabs, err := openTabs(folder)
	if err != nil {
		return nil, err
	}
	defer tabs.Close()
	if info, err := tabs.Lstat(id); err != nil || !info.IsDir() {
		return nil, ErrNotFound
	}
	return tabs.OpenRoot(id)
}

func readDir(root *os.Root) ([]fs.DirEntry, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.ReadDir(-1)
}

// inspect validates one tab folder: its name, its manifest, its index.html
// and its size.
func inspect(tabs *os.Root, name string) Tab {
	tab := Tab{ID: name, Title: name}
	if !tabIDPattern.MatchString(name) {
		tab.Problem = "the folder name must be 1–32 lowercase letters, digits or hyphens"
		return tab
	}
	root, err := tabs.OpenRoot(name)
	if err != nil {
		tab.Problem = "the tab folder cannot be opened"
		return tab
	}
	defer root.Close()
	m, problem := readManifest(root)
	if problem != "" {
		tab.Problem = problem
		return tab
	}
	tab.Title, tab.Order = strings.TrimSpace(*m.Title), m.Order
	if info, err := root.Stat("index.html"); err != nil || !info.Mode().IsRegular() {
		tab.Problem = "index.html is missing"
		return tab
	}
	tab.Problem = checkSize(root)
	return tab
}

func readManifest(root *os.Root) (manifest, string) {
	var m manifest
	file, err := root.Open("tab.json")
	if err != nil {
		return m, "tab.json is missing"
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return m, "tab.json cannot be read"
	}
	if len(data) > maxManifestBytes {
		return m, "tab.json is over 16 KB"
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return m, "tab.json is not valid: " + err.Error()
	}
	if decoder.More() {
		return m, "tab.json must hold one JSON object"
	}
	if m.Title == nil {
		return m, "tab.json needs a title"
	}
	if title := strings.TrimSpace(*m.Title); title == "" || utf8.RuneCountInString(title) > maxTitleRunes {
		return m, fmt.Sprintf("the title must be 1–%d characters", maxTitleRunes)
	}
	return m, ""
}

// checkSize walks the tab folder and stops as soon as it is over a limit.
func checkSize(root *os.Root) string {
	files, size := 0, int64(0)
	over := errors.New("over")
	problem := ""
	err := fs.WalkDir(root.FS(), ".", func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if files++; files > MaxTabFiles {
			problem = fmt.Sprintf("the tab folder has over %d files", MaxTabFiles)
			return over
		}
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
			if size += info.Size(); size > MaxTabBytes {
				problem = "the tab folder is over 5 MB"
				return over
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, over) {
		return "the tab folder cannot be read"
	}
	return problem
}
