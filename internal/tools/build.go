package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/davidadel66/evie/internal/openrouter"
)

// Build worker tools (Stage 11 of docs/memory-architecture.md, decided by
// the owner on 2026-10-03: free edits inside the worktree, approval once at
// merge). A delegated build worker changes code in its own git worktree on
// its own branch, which Evie's harness creates, commits and cleans up. Its
// file tools write without approval, but only inside that worktree: every
// path is resolved with symlinks evaluated and must stay inside it, and file
// access goes through os.Root, so a link swapped in later cannot lead out
// either. The worktree's .git and any nested .git are the harness's alone.
// Its bash runs only under an OS sandbox (sandbox.go); without one there is
// no bash.

// maxBuildWriteBytes bounds one write_file call.
const maxBuildWriteBytes = 1 << 20

// BuildFence is one build attempt's worktree and private temp folder, both
// canonical absolute paths. The harness binds it with WithBuildFence.
type BuildFence struct {
	Worktree string
	Temp     string
}

// BuildTools returns fresh definitions of the build worker's Kernel tools,
// in schema order. withBash adds the sandboxed bash; a host without a usable
// sandbox composes build workers without it.
func BuildTools(withBash bool) []Tool {
	definitions := []Tool{
		{Schema: cloneSchema(buildReadFileTool), Execute: buildReadFile},
		{Schema: cloneSchema(buildEditFileTool), Execute: buildEditFile},
		{Schema: cloneSchema(buildWriteFileTool), Execute: buildWriteFile},
	}
	if withBash {
		definitions = append(definitions, Tool{Schema: cloneSchema(sandboxedBashTool), Execute: runSandboxedBash})
	}
	return definitions
}

// BuildToolset is the build preset's base toolset.
func BuildToolset(withBash bool) Toolset { return NewToolset(BuildTools(withBash)) }

type buildFenceKey struct{}

func withBuildFence(ctx context.Context, fence BuildFence) context.Context {
	return context.WithValue(ctx, buildFenceKey{}, fence)
}

// WithBuildFence binds every tool of t to one build attempt's fence. Schemas
// are unchanged, so the toolset still matches its receipt.
func (t Toolset) WithBuildFence(fence BuildFence) Toolset {
	definitions := make([]Tool, len(t.tools))
	for i, definition := range t.tools {
		if execute := definition.Execute; execute != nil {
			definition.Execute = func(ctx context.Context, args string) (string, error) {
				return execute(withBuildFence(ctx, fence), args)
			}
		}
		if prepare := definition.Prepare; prepare != nil {
			definition.Prepare = func(ctx context.Context, args string) (PreparedTool, error) {
				return prepare(withBuildFence(ctx, fence), args)
			}
		}
		definitions[i] = definition
	}
	return NewToolset(definitions)
}

// buildFenceFrom is the bound fence, whose worktree is still a canonical
// existing directory.
func buildFenceFrom(ctx context.Context) (BuildFence, error) {
	fence, _ := ctx.Value(buildFenceKey{}).(BuildFence)
	if fence.Worktree == "" {
		return fence, errors.New("this tool runs only inside a delegated build worker with a pinned worktree")
	}
	canonical, err := filepath.EvalSymlinks(fence.Worktree)
	if err != nil || canonical != fence.Worktree {
		return fence, fmt.Errorf("the worker's worktree %q is unavailable or has moved", fence.Worktree)
	}
	if info, err := os.Stat(fence.Worktree); err != nil || !info.IsDir() {
		return fence, fmt.Errorf("the worker's worktree %q is not a directory", fence.Worktree)
	}
	return fence, nil
}

// GitComponent reports whether one path component names a git directory:
// .git in any case, ignoring invisible format characters and trailing dots
// or spaces that some filesystems drop. .gitignore, .github and the like do
// not count.
func GitComponent(name string) bool {
	name = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) || unicode.Is(unicode.Variation_Selector, r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	return strings.EqualFold(name, ".git")
}

// hasGitComponent reports whether rel, a slash- or separator-joined relative
// path, has a git-directory component.
func hasGitComponent(rel string) bool {
	for _, component := range strings.Split(filepath.ToSlash(rel), "/") {
		if GitComponent(component) {
			return true
		}
	}
	return false
}

// fencedPath is a checked path relative to the worktree: read is fully
// resolved through symlinks; write resolves only the parent folder, so a
// write replaces the named entry itself and never follows a final link.
type fencedPath struct{ read, write string }

// buildPath resolves a model-supplied path against the worktree. The path,
// and what it resolves to through symlinks, must stay inside the worktree,
// have no .git component and be off the secrets denylist. Access then goes
// through os.Root, which refuses any link that leads out at access time.
func buildPath(fence BuildFence, arg string) (fencedPath, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return fencedPath{}, errors.New("path must not be empty")
	}
	if strings.HasPrefix(arg, "~") {
		return fencedPath{}, fmt.Errorf("%q uses ~, which is outside the worktree %s; use a path inside it", arg, fence.Worktree)
	}
	p := arg
	if !filepath.IsAbs(p) {
		p = filepath.Join(fence.Worktree, p)
	}
	p = filepath.Clean(p)
	if !pathWithin(fence.Worktree, p) {
		return fencedPath{}, fmt.Errorf("%q is outside the worktree %s", arg, fence.Worktree)
	}
	rel, err := filepath.Rel(fence.Worktree, p)
	if err != nil || rel == "." {
		return fencedPath{}, fmt.Errorf("%q is the worktree itself, a directory", arg)
	}
	if hasGitComponent(rel) {
		return fencedPath{}, fmt.Errorf("%q is inside a .git, which Evie's harness manages; the harness commits your changes", arg)
	}
	if denied(p) {
		return fencedPath{}, fmt.Errorf("%q is off-limits: that location may hold secrets", arg)
	}
	var out fencedPath
	for _, c := range []struct {
		resolved string
		into     *string
	}{
		{resolveExisting(p), &out.read},
		{filepath.Join(resolveExisting(filepath.Dir(p)), filepath.Base(p)), &out.write},
	} {
		if !pathWithin(fence.Worktree, c.resolved) {
			return fencedPath{}, fmt.Errorf("%q resolves to %s, outside the worktree %s", arg, c.resolved, fence.Worktree)
		}
		resolvedRel, err := filepath.Rel(fence.Worktree, c.resolved)
		if err != nil || resolvedRel == "." || hasGitComponent(resolvedRel) {
			return fencedPath{}, fmt.Errorf("%q resolves to the worktree's .git or root, which Evie's harness manages", arg)
		}
		if denied(c.resolved) {
			return fencedPath{}, fmt.Errorf("%q is off-limits: that location may hold secrets", arg)
		}
		*c.into = resolvedRel
	}
	return out, nil
}

// openBuildRoot opens the fenced worktree as an os.Root, so no file access
// below it can follow a link out, whenever that link appeared.
func openBuildRoot(ctx context.Context, args string, params any) (BuildFence, *os.Root, error) {
	fence, err := buildFenceFrom(ctx)
	if err != nil {
		return fence, nil, err
	}
	if err := json.Unmarshal([]byte(args), params); err != nil {
		return fence, nil, fmt.Errorf("parse arguments: %w", err)
	}
	root, err := os.OpenRoot(fence.Worktree)
	if err != nil {
		return fence, nil, fmt.Errorf("open the worktree: %w", err)
	}
	return fence, root, nil
}

// fenceError words an os.Root refusal for the model.
func fenceError(arg string, err error) error {
	if strings.Contains(err.Error(), "path escapes from parent") {
		return fmt.Errorf("%q leads outside the worktree", arg)
	}
	return err
}

// readBuildFile reads one fenced regular file within the read cap.
func readBuildFile(root *os.Root, rel, arg string) (string, fs.FileMode, error) {
	file, err := root.Open(rel)
	if err != nil {
		return "", 0, fenceError(arg, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if info.IsDir() {
		return "", 0, fmt.Errorf("%s is a directory, not a file", arg)
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%s is not a regular file", arg)
	}
	if info.Size() > maxReadBytes {
		return "", 0, fmt.Errorf("%s is %dKB; the limit is %dKB. Narrow down with bash (rg, head, tail) instead", arg, info.Size()/1024, maxReadBytes/1024)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxReadBytes+1))
	if err != nil {
		return "", 0, fmt.Errorf("read %s: %w", arg, err)
	}
	return string(data), info.Mode().Perm(), nil
}

// writableTarget refuses to replace anything but a regular file: renaming
// over a symlink would replace the link, not what it points to.
func writableTarget(root *os.Root, rel, arg string) (fs.FileMode, bool, error) {
	info, err := root.Lstat(rel)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return 0o644, false, nil
	case err != nil:
		return 0, false, fenceError(arg, err)
	case info.Mode()&fs.ModeSymlink != 0:
		return 0, false, fmt.Errorf("%s is a symbolic link; write the file it points to by its own path", arg)
	case info.IsDir():
		return 0, false, fmt.Errorf("%s is a directory, not a file", arg)
	case !info.Mode().IsRegular():
		return 0, false, fmt.Errorf("%s is not a regular file", arg)
	}
	return info.Mode().Perm(), true, nil
}

// writeBuildFile replaces rel atomically through root: a complete temp file
// in the same folder, renamed into place. The rename replaces the directory
// entry itself, so it never writes through a link or into a hard-linked file.
func writeBuildFile(ctx context.Context, root *os.Root, rel string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(rel)
	if dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fenceError(rel, err)
		}
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".evie-write-"+hex.EncodeToString(suffix))
	file, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fenceError(rel, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = root.Remove(tmp)
		}
	}()
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write %s: %w", rel, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("write %s: %w", rel, err)
	}
	if err := root.Chmod(tmp, perm); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.Rename(tmp, rel); err != nil {
		return fenceError(rel, err)
	}
	committed = true
	return nil
}

var buildReadFileTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "read_file",
		Description: `Read a text file in your worktree. Returns the entire file with every line prefixed by its line number and a tab, like "    42<tab>the actual line"; the numbers are display only and are not in the file.

A relative path resolves against the worktree root. Paths outside the worktree, including through .. or symlinks, the worktree's .git, and files that may hold credentials (.env files and similar) are refused. Files larger than 100KB are refused rather than truncated; narrow down with bash (rg, head, tail) when you have it.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]openrouter.Property{
				"path": {Type: "string", Description: "Path to an existing regular file in the worktree: relative to the worktree root, or absolute."},
			},
		},
	},
}

var buildEditFileTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "edit_file",
		Description: `Replace an exact string in an existing file in your worktree. No approval is needed inside the worktree; the owner reviews your branch before it is merged.

old_string must appear EXACTLY ONCE in the file, with its exact whitespace and indentation; include more surrounding lines when it is ambiguous. Strip read_file's line-number prefixes first. To create a file or replace it whole, use write_file. Paths outside the worktree (including through .. or symlinks), its .git and secret files are refused.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"path", "old_string", "new_string"},
			Properties: map[string]openrouter.Property{
				"path":       {Type: "string", Description: "Path to an existing file in the worktree: relative to the worktree root, or absolute."},
				"old_string": {Type: "string", Description: "The exact text to replace. Must match exactly once in the file."},
				"new_string": {Type: "string", Description: "The text to put in its place. Must differ from old_string."},
			},
		},
	},
}

var buildWriteFileTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "write_file",
		Description: `Create a file in your worktree, or replace an existing file's whole content. Missing parent folders are created. No approval is needed inside the worktree; the owner reviews your branch before it is merged.

Content is at most 1 MiB. Paths outside the worktree (including through .. or symlinks), symlinks themselves, the worktree's .git or any .git folder, and secret files are refused. Prefer edit_file for small changes to an existing file.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"path", "content"},
			Properties: map[string]openrouter.Property{
				"path":    {Type: "string", Description: "Path of the file in the worktree: relative to the worktree root, or absolute."},
				"content": {Type: "string", Description: "The file's complete new content."},
			},
		},
	},
}

func buildReadFile(ctx context.Context, args string) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	fence, root, err := openBuildRoot(ctx, args, &params)
	if err != nil {
		return "", err
	}
	defer root.Close()
	path, err := buildPath(fence, params.Path)
	if err != nil {
		return "", err
	}
	content, _, err := readBuildFile(root, path.read, params.Path)
	if err != nil {
		return "", err
	}
	return numbered(content), ctx.Err()
}

func buildWriteFile(ctx context.Context, args string) (string, error) {
	var params struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
	}
	fence, root, err := openBuildRoot(ctx, args, &params)
	if err != nil {
		return "", err
	}
	defer root.Close()
	path, err := buildPath(fence, params.Path)
	if err != nil {
		return "", err
	}
	if params.Content == nil {
		return "", errors.New("content is required (it may be empty)")
	}
	if len(*params.Content) > maxBuildWriteBytes {
		return "", fmt.Errorf("content is %d bytes; the limit is %d", len(*params.Content), maxBuildWriteBytes)
	}
	perm, existed, err := writableTarget(root, path.write, params.Path)
	if err != nil {
		return "", err
	}
	if err := writeBuildFile(ctx, root, path.write, []byte(*params.Content), perm); err != nil {
		return "", err
	}
	verb := "created"
	if existed {
		verb = "replaced"
	}
	return fmt.Sprintf("OK — %s %s (%d bytes)\n", verb, path.write, len(*params.Content)), nil
}

func buildEditFile(ctx context.Context, args string) (string, error) {
	var params struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	fence, root, err := openBuildRoot(ctx, args, &params)
	if err != nil {
		return "", err
	}
	defer root.Close()
	path, err := buildPath(fence, params.Path)
	if err != nil {
		return "", err
	}
	perm, existed, err := writableTarget(root, path.write, params.Path)
	if err != nil {
		return "", err
	}
	if !existed {
		return "", fmt.Errorf("%s does not exist; create it with write_file", params.Path)
	}
	content, _, err := readBuildFile(root, path.write, params.Path)
	if err != nil {
		return "", err
	}
	out, line, err := applyEdit(content, stripLineNumbers(params.OldString), stripLineNumbers(params.NewString))
	if err != nil {
		return "", err
	}
	if err := writeBuildFile(ctx, root, path.write, []byte(out), perm); err != nil {
		return "", err
	}
	return fmt.Sprintf("OK — replaced 1 occurrence in %s at line %d\n", path.write, line), nil
}
