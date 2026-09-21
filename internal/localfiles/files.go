// Package localfiles reads current disk contents beneath one attached folder.
// It never substitutes those contents for recorded conversation evidence.
package localfiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

const MaxFileBytes = 512 * 1024

type Entry struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
}
type Listing struct {
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}
type File struct {
	Path string `json:"path"`
	Text string `json:"text"`
	Size int64  `json:"size"`
}

func openRoot(path string) (*os.Root, error) {
	canonical, err := memory.CanonicalProjectRoot(path)
	if err != nil || canonical != path {
		return nil, errors.New("The attached folder is unavailable or has moved. Update it in the Workspace.")
	}
	return os.OpenRoot(path)
}

func relative(path string) (string, error) {
	if path == "" {
		path = "."
	}
	if !fs.ValidPath(path) || strings.Contains(path, "\\") {
		return "", errors.New("Choose a path inside the attached folder")
	}
	return path, nil
}

func allowed(root, path string) bool {
	return tools.ValidateFileAccess(filepath.Join(root, filepath.FromSlash(path))) == nil
}

func List(ctx context.Context, folder, path, query string) (Listing, error) {
	result := Listing{Entries: []Entry{}}
	path, err := relative(path)
	if err != nil {
		return result, err
	}
	root, err := openRoot(folder)
	if err != nil {
		return result, err
	}
	defer root.Close()
	if !allowed(folder, path) {
		return result, errors.New("This location is excluded from file browsing")
	}
	query = strings.ToLower(strings.TrimSpace(query))
	add := func(path, name string, dir bool) { result.Entries = append(result.Entries, Entry{path, name, dir}) }
	if query == "" {
		f, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return result, errors.New("Folder could not be opened")
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.IsDir() {
			return result, errors.New("Choose a directory to browse")
		}
		entries, err := f.ReadDir(2001)
		if err != nil && err != io.EOF {
			return result, err
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			child := e.Name()
			if path != "." {
				child = path + "/" + child
			}
			if e.Name() == ".git" || !allowed(folder, child) || e.Type()&os.ModeSymlink != 0 {
				continue
			}
			add(child, e.Name(), e.IsDir())
		}
		result.Truncated = len(entries) > 2000
	} else {
		seen := 0
		err = fs.WalkDir(root.FS(), ".", func(path string, e fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return nil
			}
			seen++
			if seen > 20000 || len(result.Entries) >= 300 {
				result.Truncated = true
				return fs.SkipAll
			}
			if path == "." {
				return nil
			}
			if !allowed(folder, path) || e.Name() == ".git" || e.Name() == "node_modules" || e.Name() == ".venv" {
				if e.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if e.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if strings.Contains(strings.ToLower(path), query) {
				add(path, e.Name(), e.IsDir())
			}
			return nil
		})
		if err != nil {
			return result, err
		}
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		a, b := result.Entries[i], result.Entries[j]
		if a.Directory != b.Directory {
			return a.Directory
		}
		return a.Path < b.Path
	})
	return result, nil
}

func Read(folder, path string) (File, error) {
	result := File{Path: path}
	path, err := relative(path)
	if err != nil {
		return result, err
	}
	root, err := openRoot(folder)
	if err != nil {
		return result, err
	}
	defer root.Close()
	if !allowed(folder, path) {
		return result, errors.New("This file is excluded from file browsing")
	}
	info, err := root.Stat(path)
	if err != nil {
		return result, errors.New("File is unavailable")
	}
	if !info.Mode().IsRegular() {
		return result, errors.New("Only regular text files can be opened")
	}
	if info.Size() > MaxFileBytes {
		return result, fmt.Errorf("File is too large to preview (limit %d KiB)", MaxFileBytes/1024)
	}
	f, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return result, errors.New("File is outside the attached folder or unavailable")
	}
	defer f.Close()
	// Inspect the opened file too, so a replacement cannot bypass the size/type check.
	info, err = f.Stat()
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, errors.New("Only regular text files can be opened")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return result, err
	}
	if len(data) > MaxFileBytes {
		return result, errors.New("File is too large to preview")
	}
	if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return result, errors.New("Binary files do not have a text preview")
	}
	result.Path = path
	result.Text = string(data)
	result.Size = int64(len(data))
	return result, nil
}
