package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/davidadel66/evie/internal/openrouter"
)

// Explore worker tools (Stage 7 of docs/memory-architecture.md, decided by
// the owner on 2026-10-03). A delegated explore worker reads local files,
// databases and repositories for its orchestrator and can change nothing:
// read_file and bash are confined to the folder pinned for its assignment,
// bash runs only vetted read-only commands (readonly_bash.go), and query_db
// is the Kernel's existing read-only tool with its existing fences.

// ExploreToolNames are the explore worker's tools, in schema order.
var ExploreToolNames = []string{"read_file", "query_db", "bash"}

// ExploreTools returns fresh definitions of the explore worker's tools. Both
// confined tools refuse to run unless the supervisor bound a root with
// Toolset.WithWorkerRoot.
func ExploreTools() []Tool {
	return []Tool{
		{Schema: cloneSchema(workerReadFileTool), Execute: workerReadFile},
		{Schema: cloneSchema(queryDBTool), Execute: queryDB},
		{Schema: cloneSchema(readOnlyBashTool), Execute: runReadOnlyBash},
	}
}

// ExploreToolset is the explore preset's base toolset.
func ExploreToolset() Toolset { return NewToolset(ExploreTools()) }

type workerRootKey struct{}

func withWorkerRoot(ctx context.Context, root string) context.Context {
	return context.WithValue(ctx, workerRootKey{}, root)
}

// WithWorkerRoot binds every tool of t to one explore worker's pinned root.
// Schemas are unchanged, so the toolset still matches its receipt.
func (t Toolset) WithWorkerRoot(root string) Toolset {
	definitions := make([]Tool, len(t.tools))
	for i, definition := range t.tools {
		if execute := definition.Execute; execute != nil {
			definition.Execute = func(ctx context.Context, args string) (string, error) {
				return execute(withWorkerRoot(ctx, root), args)
			}
		}
		if prepare := definition.Prepare; prepare != nil {
			definition.Prepare = func(ctx context.Context, args string) (PreparedTool, error) {
				return prepare(withWorkerRoot(ctx, root), args)
			}
		}
		definitions[i] = definition
	}
	return NewToolset(definitions)
}

// workerRootFrom is the bound root, still a canonical existing directory. A
// root that was moved, deleted or replaced by a symlink since it was pinned
// is refused rather than followed.
func workerRootFrom(ctx context.Context) (string, error) {
	root, _ := ctx.Value(workerRootKey{}).(string)
	if root == "" {
		return "", errors.New("this tool runs only inside a delegated explore worker with a pinned root")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || canonical != root {
		return "", fmt.Errorf("the worker's root %q is unavailable or has moved", root)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return "", fmt.Errorf("the worker's root %q is not a directory", root)
	}
	return root, nil
}

// ResolveWorkerRoot chooses an explore worker's root: declared resolved
// against base when given, otherwise base itself. base is the parent's
// Workspace folder or project root, or empty when the parent has neither, in
// which case declared must be an absolute path. The result is canonical, an
// existing directory, off the secrets denylist and, when base is set, inside
// base after resolving symlinks.
func ResolveWorkerRoot(base, declared string) (string, error) {
	declared = strings.TrimSpace(declared)
	candidate := base
	if declared != "" {
		expanded, err := expandHome(declared)
		if err != nil {
			return "", err
		}
		switch {
		case filepath.IsAbs(expanded):
			candidate = expanded
		case base == "":
			return "", fmt.Errorf("path %q must be absolute: this conversation has no Workspace folder or project root", declared)
		default:
			candidate = filepath.Join(base, expanded)
		}
	}
	if candidate == "" {
		return "", errors.New("explore needs a folder: this conversation has no Workspace folder or project root, so declare an absolute path")
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("folder %q is unavailable: %w", candidate, err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", candidate)
	}
	// A worker may search its root recursively, so the root may neither be
	// nor contain a location that may hold secrets.
	if denied(abs) || denied(root) || holdsDenied(root) {
		return "", fmt.Errorf("%s is off-limits as a worker folder: it is or contains a location that may hold secrets (~/.ssh, ~/.aws, ~/.gnupg, ~/.evie)", root)
	}
	if base != "" {
		canonicalBase, err := filepath.EvalSymlinks(base)
		if err != nil {
			return "", fmt.Errorf("folder %q is unavailable: %w", base, err)
		}
		if !pathWithin(canonicalBase, root) {
			return "", fmt.Errorf("path %q is outside this conversation's folder %s", declared, canonicalBase)
		}
	}
	return root, nil
}

// holdsDenied reports whether the folder dir contains a denylisted directory
// or file, so that a recursive search below dir could reach it.
func holdsDenied(dir string) bool {
	for _, location := range append(append([]string(nil), deniedDirs...), deniedFiles...) {
		expanded, err := expandHome(location)
		if err != nil {
			continue
		}
		if pathWithin(dir, expanded) {
			return true
		}
		if resolved, err := filepath.EvalSymlinks(expanded); err == nil && pathWithin(dir, resolved) {
			return true
		}
	}
	return false
}

// pathWithin reports whether p is root or below it. Both must be clean
// absolute paths.
func pathWithin(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveExisting resolves symlinks in the longest existing prefix of p and
// keeps the rest, so a path that does not exist yet is judged by where its
// existing ancestors really are.
func resolveExisting(p string) string {
	rest := ""
	current := p
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return p
		}
		rest = filepath.Join(filepath.Base(current), rest)
		current = parent
	}
}

// confinePath resolves arg against cwd and requires it, and what it
// resolves to through symlinks, to stay inside root and off the secrets
// denylist. It returns the cleaned absolute path.
func confinePath(root, cwd, arg string) (string, error) {
	if strings.HasPrefix(arg, "~") {
		return "", fmt.Errorf("%q uses ~, which is outside the worker's root %s; use a path inside it", arg, root)
	}
	p := arg
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	p = filepath.Clean(p)
	if !pathWithin(root, p) {
		return "", fmt.Errorf("%q is outside the worker's root %s", arg, root)
	}
	if resolved := resolveExisting(p); !pathWithin(root, resolved) {
		return "", fmt.Errorf("%q resolves to %s, outside the worker's root %s", arg, resolved, root)
	} else if denied(resolved) {
		return "", fmt.Errorf("%q is off-limits: that location may hold secrets", arg)
	}
	if denied(p) {
		return "", fmt.Errorf("%q is off-limits: that location may hold secrets", arg)
	}
	return p, nil
}

// workerReadFileTool is read_file for an explore worker: the same output and
// limits, confined to the worker's root.
var workerReadFileTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "read_file",
		Description: `Read a text file inside this assignment's root folder. Returns the entire file with every line prefixed by its line number and a tab, like "    42<tab>the actual line"; the numbers are display only.

A relative path resolves against the root. Paths outside the root, including through .. or symlinks, and files that may hold credentials (.env files, shell rc files, ~/.ssh and similar) are refused. Files larger than 100KB are refused rather than truncated; narrow down with bash (rg, head, tail) instead.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]openrouter.Property{
				"path": {
					Type:        "string",
					Description: "Path to an existing regular file inside the root: relative to the root, or absolute.",
				},
			},
		},
	},
}

func workerReadFile(ctx context.Context, args string) (string, error) {
	root, err := workerRootFrom(ctx)
	if err != nil {
		return "", err
	}
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	path := strings.TrimSpace(params.Path)
	if path == "" {
		return "", errors.New("path must not be empty")
	}
	abs, err := confinePath(root, root, path)
	if err != nil {
		return "", err
	}
	return readFileAt(ctx, abs)
}
