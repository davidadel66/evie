package skill

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ReadFolder reads and checks the skill folder dir, named by its last path
// element. A dir that is itself a link is followed; inside the folder every
// entry is checked with Lstat before anything is read, and links, scripts/
// folders at any depth, executable and non-regular files are refused,
// because the procedural repository stores every file as plain,
// non-executable text. Operating-system and version-control clutter is
// skipped with a warning. Reads go through an os.Root of the folder, so
// nothing outside it is read.
func ReadFolder(dir string) (Version, error) {
	clean := filepath.Clean(dir)
	info, err := os.Stat(clean)
	if err != nil {
		return Version{}, fmt.Errorf("skill folder %s: %w", dir, err)
	}
	if !info.IsDir() {
		return Version{}, fmt.Errorf("skill folder %s is not a folder", dir)
	}
	root, err := os.OpenRoot(clean)
	if err != nil {
		return Version{}, fmt.Errorf("skill folder %s: %w", dir, err)
	}
	defer root.Close()
	w := &walker{root: root, files: map[string][]byte{}, refused: map[string]bool{}}
	if err := w.walk("."); err != nil {
		return Version{}, fmt.Errorf("skill folder %s: %w", dir, err)
	}
	// A walk stopped at a limit may not have reached SKILL.md; the limit is
	// the problem then, not a missing SKILL.md.
	if _, read := w.files[InstructionsFile]; w.stopped && !read {
		if _, err := root.Lstat(InstructionsFile); err == nil {
			w.refused[InstructionsFile] = true
		}
	}
	v, problems := check(filepath.Base(clean), w.files, w.refused)
	problems = append(w.problems, problems...)
	if len(problems) > 0 {
		return Version{}, &InvalidError{Problems: problems}
	}
	if len(w.skipped) > 0 {
		v.Warnings = append(v.Warnings, Warning{Kind: WarningSkipped,
			Message: "skipped " + listed(w.skipped) + ": operating-system and version-control files are not part of a skill"})
	}
	return v, nil
}

// clutter reports whether a folder entry is operating-system or
// version-control clutter, never part of a skill: the Finder's .DS_Store
// and Icon\r, AppleDouble ._ files, and a .git folder or file.
func clutter(name string) bool {
	return name == ".DS_Store" || name == "Icon\r" || strings.HasPrefix(name, "._") || strings.EqualFold(name, ".git")
}

// shownPath is p as the owner reads it in a warning: quoted when it holds a
// control character, such as Icon\r.
func shownPath(p string) string {
	if strings.ContainsFunc(p, unicode.IsControl) {
		return strconv.Quote(p)
	}
	return p
}

type walker struct {
	root    *os.Root
	files   map[string][]byte
	refused map[string]bool
	// skipped are the clutter paths left out, in walk order.
	skipped  []string
	problems []Problem
	count    int
	total    int64
	stopped  bool
}

func (w *walker) refuse(p, format string, args ...any) {
	w.refused[p] = true
	w.problems = append(w.problems, Problem{Path: p, Message: fmt.Sprintf(format, args...)})
}

func (w *walker) walk(dir string) error {
	f, err := w.root.Open(dir)
	if err != nil {
		return err
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if w.stopped {
			return nil
		}
		p := path.Join(dir, entry.Name())
		if clutter(entry.Name()) {
			w.skipped = append(w.skipped, shownPath(p))
			continue
		}
		info, err := w.root.Lstat(p)
		if err != nil {
			return err
		}
		mode := info.Mode()
		switch {
		case mode&fs.ModeSymlink != 0:
			w.refuse(p, "is a symbolic link; links inside a skill are refused")
		case mode.IsDir() && entry.Name() == "scripts":
			w.refuse(p, "%s", scriptsRefused)
		case mode.IsDir():
			if msg := pathProblem(p); msg != "" {
				w.refuse(p, "%s", msg)
				continue
			}
			if err := w.walk(p); err != nil {
				return err
			}
		case !mode.IsRegular():
			w.refuse(p, "is not a regular file")
		case mode.Perm()&0o111 != 0:
			w.refuse(p, "is executable; executable files are refused")
		case info.Size() > MaxFileBytes:
			w.refuse(p, "%s", fileTooLarge(info.Size()))
		default:
			if err := w.read(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// read reads one regular file, checking it again on the open file, and
// stops the walk past the file count or total size limit.
func (w *walker) read(p string) error {
	w.count++
	if w.count > MaxFiles {
		w.refuse(".", "%s", tooManyFiles())
		w.stopped = true
		return nil
	}
	f, err := w.root.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
		return errors.New(p + " changed while it was read")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return err
	}
	if len(data) > MaxFileBytes {
		w.refuse(p, "%s", fileTooLarge(int64(len(data))))
		return nil
	}
	w.total += int64(len(data))
	if w.total > MaxTotalBytes {
		w.refuse(".", "%s", tooLarge())
		w.stopped = true
		return nil
	}
	w.files[p] = data
	return nil
}
