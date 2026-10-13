// Package workspacetabs reads a Workspace's own tabs from its folder
// (workspace-tabs.spec.md). Each tab is a folder .evie/tabs/<tab-id>/ that
// holds tab.json and index.html. Every read goes through an os.Root opened
// on the tab folder, so no path or ".." reaches outside it, and only regular
// files reached without a link are read: a link could otherwise serve a
// dotfile, and a pipe would hang the reader.
package workspacetabs

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
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
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	// Dir is where a Workspace folder keeps its tabs.
	Dir = ".evie/tabs"
	// MaxTabs is how many tabs one Workspace shows; later ones are problems.
	MaxTabs = 12
	// MaxTabBytes and MaxTabFiles bound one tab folder; MaxTabFiles counts
	// folders inside it too.
	MaxTabBytes = 5 << 20
	MaxTabFiles = 200
	// maxFolders bounds how many tab folders are examined at all, and
	// maxEntries how many entries .evie/tabs may hold.
	maxFolders = 32
	maxEntries = 256
	// maxManifestBytes bounds tab.json.
	maxManifestBytes = 16 << 10
	maxTitleRunes    = 24
	// maxSources bounds a tab's data list, and MaxFileBytes one file read.
	maxSources   = 8
	MaxFileBytes = 1 << 20
	// Reserved is the path inside a tab where Evie serves its own helper;
	// a tab folder's own files there are never served.
	Reserved = "_evie"
)

var (
	ErrNotFound = errors.New("workspace tab not found")
	ErrInvalid  = errors.New("workspace tab has a problem")
	ErrPath     = errors.New("workspace tab path is not allowed")
	// ErrTooMany is a .evie/tabs folder with more than maxEntries entries.
	ErrTooMany = fmt.Errorf(".evie/tabs holds over %d entries", maxEntries)
)

var tabIDPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// ValidID is whether id can name a tab folder.
func ValidID(id string) bool { return tabIDPattern.MatchString(id) }

// Tab is one tab as the header and Settings → Tabs show it. A tab with a
// Problem is listed but never shown or served. Data is what it may read once
// the owner approves; DataKey pins an approval to that exact list.
type Tab struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	Order   int          `json:"order"`
	Data    []DataSource `json:"data,omitempty"`
	DataKey string       `json:"dataKey,omitempty"`
	Problem string       `json:"problem,omitempty"`
}

// Problem is one thing that went wrong in a tab (story 3): a script error,
// something its page policy blocked, or a refused data request, kept so
// Evie can fix what it wrote.
type Problem struct {
	TabID      string    `json:"tabId"`
	Kind       string    `json:"kind"`
	Message    string    `json:"message"`
	RecordedAt time.Time `json:"recordedAt"`
}

// DataSource is one thing a tab may read (story 2): exactly one of a
// registered database, files in the Workspace folder matching a glob, or
// this Workspace's workflow runs.
type DataSource struct {
	DB    string `json:"db,omitempty"`
	Files string `json:"files,omitempty"`
	Runs  bool   `json:"runs,omitempty"`
}

// manifest is tab.json. Unknown fields are a problem, so a newer manifest
// on an older Evie says so instead of being half read.
type manifest struct {
	Title *string      `json:"title"`
	Order int          `json:"order"`
	Data  []DataSource `json:"data"`
}

var databaseNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// dataKey is the hex SHA-256 of a data list, "" for none.
func dataKey(data []DataSource) string {
	if len(data) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(data)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
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
	if !allowedPath(rel) || rel == Reserved || strings.HasPrefix(rel, Reserved+"/") {
		return nil, nil, ErrPath
	}
	// Open lists every tab so a 13th or broken tab is refused exactly as
	// the header leaves it out. Folder and entry limits bound that work.
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
	return openRegular(root, rel)
}

// openRegular opens rel only if it and every folder on the way are reached
// without a link and it is a regular file. The open does not block, so a
// pipe swapped in after the checks cannot hang the reader.
func openRegular(root *os.Root, rel string) (*os.File, fs.FileInfo, error) {
	segments := strings.Split(rel, "/")
	for i := range segments {
		info, err := root.Lstat(strings.Join(segments[:i+1], "/"))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, ErrNotFound
		}
		last := i == len(segments)-1
		if err != nil || info.Mode()&fs.ModeSymlink != 0 || (last && !info.Mode().IsRegular()) || (!last && !info.IsDir()) {
			return nil, nil, ErrPath
		}
	}
	file, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
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

// readDir reads at most maxEntries entries, sorted by name so which tab
// folders are examined does not depend on the filesystem's order.
func readDir(root *os.Root) ([]fs.DirEntry, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(maxEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > maxEntries {
		return nil, ErrTooMany
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
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
	tab.Title, tab.Order, tab.Data, tab.DataKey = strings.TrimSpace(*m.Title), m.Order, m.Data, dataKey(m.Data)
	if info, err := root.Lstat("index.html"); err != nil || !info.Mode().IsRegular() {
		tab.Problem = "index.html is missing or not a regular file"
		return tab
	}
	tab.Problem = checkSize(root)
	return tab
}

func readManifest(root *os.Root) (manifest, string) {
	var m manifest
	file, _, err := openRegular(root, "tab.json")
	if errors.Is(err, ErrNotFound) {
		return m, "tab.json is missing"
	}
	if err != nil {
		return m, "tab.json must be a regular file"
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
	if len(m.Data) > maxSources {
		return m, fmt.Sprintf("data lists at most %d sources", maxSources)
	}
	for _, source := range m.Data {
		if problem := source.problem(); problem != "" {
			return m, "data: " + problem
		}
	}
	return m, ""
}

func (source DataSource) problem() string {
	set := 0
	for _, present := range []bool{source.DB != "", source.Files != "", source.Runs} {
		if present {
			set++
		}
	}
	switch {
	case set != 1:
		return `each source names exactly one of "db", "files" or "runs": true`
	case source.DB != "" && !databaseNamePattern.MatchString(source.DB):
		return fmt.Sprintf("%q is not a database name", source.DB)
	case source.Files != "" && !validGlob(source.Files):
		return fmt.Sprintf("%q must be a relative glob with no dot folders or ..", source.Files)
	}
	return ""
}

// validGlob accepts a relative slash glob of path.Match segments and "**",
// with no dot segment.
func validGlob(pattern string) bool {
	if len(pattern) > 200 || strings.ContainsAny(pattern, "\\\x00") || path.IsAbs(pattern) {
		return false
	}
	for _, segment := range strings.Split(pattern, "/") {
		if segment == "" || strings.HasPrefix(segment, ".") {
			return false
		}
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, ""); err != nil {
			return false
		}
	}
	return true
}

// matchGlob matches a slash path against a validGlob pattern, where "**"
// matches zero or more whole segments.
func matchGlob(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(name); i++ {
			if matchSegments(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	ok, err := path.Match(pattern[0], name[0])
	return err == nil && ok && matchSegments(pattern[1:], name[1:])
}

// secretName is a file name that looks like it holds a secret; tabs never
// read one, whatever the glob says.
func secretName(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	for _, word := range []string{"secret", "credential", "password", "passwd", "token", "id_rsa", "id_ed25519", "id_ecdsa"} {
		if strings.Contains(base, word) {
			return true
		}
	}
	for _, ext := range []string{".pem", ".key", ".p12", ".pfx", ".keychain", ".kdbx"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	return false
}

// ReadFile reads one file of the Workspace folder for a tab's declared
// globs: a regular file reached without a link, matching a glob, with no
// dot segment or secret-looking name, at most MaxFileBytes.
func ReadFile(folder, rel string, globs []string) ([]byte, error) {
	if folder == "" || !allowedPath(rel) || secretName(rel) || !slices.ContainsFunc(globs, func(glob string) bool { return matchGlob(glob, rel) }) {
		return nil, ErrPath
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return nil, ErrNotFound
	}
	defer root.Close()
	file, info, err := openRegular(root, rel)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if info.Size() > MaxFileBytes {
		return nil, fmt.Errorf("%w: the file is over 1 MB", ErrPath)
	}
	return io.ReadAll(io.LimitReader(file, MaxFileBytes))
}

// checkSize walks the tab folder and stops as soon as it is over a limit.
func checkSize(root *os.Root) string {
	files, size := 0, int64(0)
	over := errors.New("over")
	problem := ""
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if files++; files > MaxTabFiles {
			problem = fmt.Sprintf("the tab folder has over %d files and folders", MaxTabFiles)
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
