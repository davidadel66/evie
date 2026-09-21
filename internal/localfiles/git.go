package localfiles

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Change struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}
type Changes struct {
	Available bool     `json:"available"`
	Branch    string   `json:"branch"`
	Files     []Change `json:"files"`
}
type Patch struct {
	Text string `json:"text"`
}

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2*1024*1024 {
		return 0, errors.New("Git output exceeds the 2 MiB preview limit")
	}
	return b.Buffer.Write(p)
}
func git(ctx context.Context, folder string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	options := []string{"--literal-pathspecs", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-C", folder}
	// Diff can execute clean/process filters while reading the worktree, even
	// with external diff and textconv disabled. Neutralize all configured filter
	// commands for this invocation, without changing the repository's config.
	config := exec.CommandContext(ctx, "git", append(append([]string{}, options...), "config", "--null", "--name-only", "--get-regexp", `^filter\..*\.(clean|smudge|process|required)$`)...)
	config.WaitDelay = time.Second
	var names cappedBuffer
	config.Stdout = &names
	config.Stderr = &cappedBuffer{}
	if err := config.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return "", errors.New("Git configuration could not be read")
		}
	}
	for _, name := range strings.Split(names.String(), "\x00") {
		if name == "" {
			continue
		}
		value := ""
		if strings.HasSuffix(name, ".required") {
			value = "false"
		}
		options = append(options, "-c", name+"="+value)
	}
	cmd := exec.CommandContext(ctx, "git", append(options, args...)...)
	cmd.WaitDelay = time.Second
	var out cappedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &cappedBuffer{}
	if err := cmd.Run(); err != nil {
		return "", errors.New("Git could not read this comparison; check the repository and base revision")
	}
	return out.String(), nil
}
func comparison(view, base string) ([]string, error) {
	switch view {
	case "", "working":
		return []string{"HEAD"}, nil
	case "staged":
		return []string{"--cached"}, nil
	case "branch":
		if base == "" {
			base = "HEAD"
		}
		if strings.HasPrefix(base, "-") || strings.ContainsAny(base, "\x00\r\n") || len(base) > 200 {
			return nil, errors.New("Invalid base revision")
		}
		return []string{base + "...HEAD"}, nil
	default:
		return nil, errors.New("Unknown comparison")
	}
}
func Review(ctx context.Context, folder, view, base string) (Changes, error) {
	result := Changes{Files: []Change{}}
	root, err := openRoot(folder)
	if err != nil {
		return result, err
	}
	root.Close()
	if _, err = git(ctx, folder, "rev-parse", "--show-toplevel"); err != nil {
		return result, nil
	}
	result.Available = true
	prefix, err := git(ctx, folder, "rev-parse", "--show-prefix")
	if err != nil {
		return result, err
	}
	prefix = strings.TrimSpace(prefix)
	branch, _ := git(ctx, folder, "branch", "--show-current")
	result.Branch = strings.TrimSpace(branch)
	if view == "" || view == "working" {
		out, err := git(ctx, folder, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".")
		if err != nil {
			return result, err
		}
		parts := strings.Split(out, "\x00")
		for i := 0; i < len(parts); i++ {
			p := parts[i]
			if len(p) < 4 {
				continue
			}
			path := p[3:]
			status := strings.TrimSpace(p[:2])
			if strings.ContainsAny(p[:2], "RC") {
				i++
			}
			if prefix != "" {
				if !strings.HasPrefix(path, prefix) {
					continue
				}
				path = strings.TrimPrefix(path, prefix)
			}
			if allowed(folder, path) {
				result.Files = append(result.Files, Change{path, status})
			}
		}
	} else {
		options, err := comparison(view, base)
		if err != nil {
			return result, err
		}
		args := append([]string{"diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--relative", "--name-status", "-z"}, options...)
		args = append(args, "--", ".")
		out, err := git(ctx, folder, args...)
		if err != nil {
			return result, err
		}
		parts := strings.Split(out, "\x00")
		for i := 0; i+1 < len(parts); i += 2 {
			if allowed(folder, parts[i+1]) {
				result.Files = append(result.Files, Change{parts[i+1], parts[i]})
			}
		}
	}
	return result, nil
}
func Diff(ctx context.Context, folder, path, view, base string) (Patch, error) {
	path, err := relative(path)
	if err != nil {
		return Patch{}, err
	}
	if !allowed(folder, path) {
		return Patch{}, errors.New("This file is excluded from review")
	}
	if path == "." {
		return Patch{}, errors.New("Choose one file to review")
	}
	root, err := openRoot(folder)
	if err != nil {
		return Patch{}, err
	}
	info, statErr := root.Stat(path)
	root.Close()
	if statErr == nil && info.IsDir() {
		return Patch{}, errors.New("Choose one file to review")
	}
	options, err := comparison(view, base)
	if err != nil {
		return Patch{}, err
	}
	// Untracked files are shown as additions, with the same text/size checks as Files.
	if view == "" || view == "working" {
		untracked, err := git(ctx, folder, "ls-files", "--others", "--exclude-standard", "-z", "--", path)
		if err != nil {
			return Patch{}, err
		}
		_, headErr := git(ctx, folder, "rev-parse", "--verify", "HEAD")
		if untracked != "" || headErr != nil {
			if untracked != "" && untracked != path+"\x00" {
				return Patch{}, errors.New("Choose one file to review")
			}
			if headErr != nil && errors.Is(statErr, os.ErrNotExist) {
				return Patch{}, nil
			}
			f, err := Read(folder, path)
			if err != nil {
				return Patch{}, err
			}
			return Patch{Text: "--- /dev/null\n+++ " + path + "\n+" + strings.ReplaceAll(f.Text, "\n", "\n+")}, nil
		}
	}
	args := append([]string{"diff", "--raw", "-z", "--patch", "--no-ext-diff", "--no-textconv", "--no-renames", "--no-color", "--relative", "--unified=4"}, options...)
	args = append(args, "--", path)
	text, err := git(ctx, folder, args...)
	if err != nil || text == "" {
		return Patch{}, err
	}
	// Validate Git's actual selection, including deleted historical paths. A
	// literal pathspec may still select a directory and all its descendants.
	raw, patch, ok := strings.Cut(text, "\x00\x00")
	parts := strings.Split(raw, "\x00")
	if !ok || len(parts) != 2 || parts[1] != path {
		return Patch{}, errors.New("Choose one file to review")
	}
	return Patch{Text: patch}, nil
}
