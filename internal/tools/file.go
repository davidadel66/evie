package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/davidadel66/moussa/internal/openrouter"
)

// maxReadBytes caps a single read. The constraint isn't disk, it's the
// context window: 100KB is roughly 25–30k tokens arriving in one tool
// result. Over the cap read_file errors rather than truncating — silent
// truncation would have the model reasoning about a file it only half saw.
const maxReadBytes = 100 * 1024

// Locations that hold secrets and are off-limits to every file tool, for reads
// as well as writes. Reading ~/.zshrc leaks API keys into the conversation and
// thus to the model provider; writing it is a foot-gun with no upside.
// Entries are "~"-relative and expanded at check time.
var deniedDirs = []string{"~/.ssh", "~/.aws", "~/.gnupg"}

// File names that carry credentials wherever they happen to live.
var deniedNames = []string{".zshrc", ".bashrc", ".zprofile", ".bash_profile", ".netrc"}

var lineNumRe = regexp.MustCompile(`^[ \t]*\d+\t`)

func applyEdit(content, oldString, newString string) (string, int, error) {
	if oldString == "" {
		return "", 0, errors.New("oldString_string must not be empty")
	}
	if oldString == newString {
		return "", 0, errors.New("oldString_string matches new_string exactly - nothing to do")
	}

	count := strings.Count(content, oldString)
	if count == 0 {
		return "", 0, errors.New("oldString_string does not exist in content")
	}
	if count > 1 {
		return "", 0, fmt.Errorf("oldString_string appears %v times. Include more surrounding lines so it matches exactly once", count)
	}

	idx := strings.Index(content, oldString)
	line := 1 + (strings.Count(content[:idx], "\n"))
	out := strings.Replace(content, oldString, newString, 1)

	return out, line, nil
}

func stripLineNumbers(s string) string {
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if !lineNumRe.MatchString(line) {
			return s
		}
	}
	var newString []string
	for _, line := range lines {
		_, afterLine, _ := strings.Cut(line, "\t")
		newString = append(newString, afterLine)

	}

	return strings.Join(newString, "\n")
}

// numbered prefixes every line with its 1-based number and a tab, the same
// shape `cat -n` produces — familiar enough to the model that it reads the
// prefixes as chrome rather than content. The tab matters: it is one
// unambiguous byte, so stripLineNumbers can find the boundary without
// guessing how much padding was used.
func numbered(content string) string {
	var b strings.Builder
	for i, line := range strings.Split(content, "\n") {
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, line)
	}
	return b.String()
}

// expandHome replaces a leading "~" with the current user's home directory.
// Go does not do this for you: os.Open("~/x") looks for a directory literally
// named "~". A "~user" path is rejected rather than guessed at — resolving
// another user's home needs the user database, and inventing /Users/alice is
// wrong on other platforms and a security surprise everywhere.
func expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		if strings.HasPrefix(p, "~") {
			return "", errors.New("~user paths are not supported; use an absolute path")
		}
		return p, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}

// resolvePath turns a model-supplied path into an absolute, cleaned path, and
// rejects anything on the secrets denylist. Every file tool calls this before
// touching disk: the fence lives inside path resolution so that no tool can
// obtain a usable path without having been checked.
func resolvePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("path must not be empty")
	}

	expanded, err := expandHome(p)
	if err != nil {
		return "", err
	}

	// Abs resolves a relative path against the working directory and cleans
	// away any "..", which is what makes the denylist check below meaningful.
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", p, err)
	}

	if denied(abs) {
		return "", fmt.Errorf("%s is off-limits: that location may hold secrets", abs)
	}

	// A symlink pointing into a fenced directory sails past a prefix check on
	// the unresolved path. EvalSymlinks only works on paths that already
	// exist, so an error here means "nothing to follow" rather than a problem.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && denied(resolved) {
		return "", fmt.Errorf("%s resolves to %s, which is off-limits", abs, resolved)
	}

	return abs, nil
}

// denied reports whether abs sits on the secrets denylist. It must only ever
// be called with an already-absolute, already-cleaned path: matching patterns
// against a raw "../../.env" is how denylists get walked around.
func denied(abs string) bool {
	base := filepath.Base(abs)

	// Covers .env, .env.local, .env.production, and friends.
	if strings.HasPrefix(base, ".env") {
		return true
	}
	if slices.Contains(deniedNames, base) {
		return true
	}

	for _, dir := range deniedDirs {
		expanded, err := expandHome(dir)
		if err != nil {
			continue
		}
		// The separator matters: comparing against the bare prefix would let
		// ~/.financeXYZ match ~/.finance.
		if abs == expanded || strings.HasPrefix(abs, expanded+string(os.PathSeparator)) {
			return true
		}
	}

	return false
}

// readFileTool describes read_file to the model: the ungated half of the
// file pair. Reading is safe for the filesystem, so there is no approval
// gate — but it is the leaky half, since everything it returns flows into
// the conversation and on to the model provider. The denylist inside
// resolvePath, not an approval prompt, is what defends that.
var readFileTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "read_file",
		Description: `Read a text file from the local filesystem. Returns the entire file with every line prefixed by its line number and a tab, like "    42<tab>the actual line".

The line numbers are display only — they are NOT part of the file. Never include them in edit_file's old_string or new_string.

Paths may be absolute ("/Users/david/notes.md") or home-relative ("~/notes.md"); a bare relative path resolves against the working directory. Files larger than 100KB are refused rather than truncated — there is no pagination, so narrow down with a different tool if a file is too big. A few locations holding credentials (~/.ssh, ~/.aws, ~/.gnupg, .env files, shell rc files) are off-limits and will return an error.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]openrouter.Property{
				"path": {
					Type:        "string",
					Description: "Path to the file. Absolute, or home-relative starting with ~/. Must be an existing regular file, not a directory.",
				},
			},
		},
	},
}

// readFile resolves the model's path through the secrets fence, checks the
// file is readable and under the cap, and returns it line-numbered. Stat
// comes before ReadFile deliberately: checking the size after loading the
// bytes would defeat the entire point of a cap.
func readFile(args string) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	abs, err := resolvePath(params.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", abs, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not a file", abs)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", abs)
	}
	if info.Size() > maxReadBytes {
		return "", fmt.Errorf("%s is %dKB; the limit is %dKB. There is no pagination — narrow down with a more specific file", abs, info.Size()/1024, maxReadBytes/1024)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", abs, err)
	}

	return numbered(string(data)), nil
}

// writeFileAtomic replaces the contents of abs without ever leaving it in a
// half-written state. os.WriteFile truncates first and writes second, so a
// crash between the two loses the original outright; writing a complete temp
// file and renaming it into place has no such window — os.Rename is atomic,
// and a concurrent reader sees either the whole old file or the whole new one.
//
// The temp file must sit in the same directory as abs: renaming across
// filesystems is not atomic and may fail outright, so a temp file in /tmp
// would defeat the entire point.
func writeFileAtomic(abs string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(abs)

	tmp, err := os.CreateTemp(dir, ".moussa-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	// Every failure between here and the rename must take the temp file with
	// it; cleared once the rename succeeds and there is nothing left to clean.
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// CreateTemp always makes 0600; restore whatever the real file carried.
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, abs); err != nil {
		return fmt.Errorf("rename temp file over %s: %w", abs, err)
	}

	tmpName = ""
	return nil
}

// editFileTool describes edit_file to the model: the destructive half of
// the pair, gated behind David's approval (NeedsApproval in the registry).
// Exact string replacement rather than line numbers or whole-file rewrites
// — models quote text reliably, count lines unreliably, and a whole-file
// rewrite risks silent truncation of everything it didn't echo back.
var editFileTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "edit_file",
		Description: `Replace an exact string in an existing file. Every call is shown to David for approval before it runs — explain what you're about to change before calling this, and keep each edit small and targeted.

old_string must appear EXACTLY ONCE in the file. If it appears zero times or more than once the edit is refused rather than guessed at; the fix for an ambiguous match is to include more surrounding lines until the quote is unique.

Whitespace and indentation are part of the match and are preserved verbatim — quote the file exactly as it is on disk. If you got the text from read_file, strip the "    42<tab>" line-number prefixes first; they are display only and are not in the file.

This tool cannot create files: the file must already exist, and an empty old_string is an error.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"path", "old_string", "new_string"},
			Properties: map[string]openrouter.Property{
				"path": {
					Type:        "string",
					Description: "Path to an existing file. Absolute, or home-relative starting with ~/.",
				},
				"old_string": {
					Type:        "string",
					Description: "The exact text to replace, including indentation. Must match exactly once in the file.",
				},
				"new_string": {
					Type:        "string",
					Description: "The text to put in its place. Must differ from old_string.",
				},
			},
		},
	},
}

// editFile applies one approved string replacement. Every failure returns
// an error rather than a partial write, and those error strings are the
// model's only feedback — they are written to be acted on, not just read.
func editFile(args string) (string, error) {
	var params struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	abs, err := resolvePath(params.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", abs, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not a file", abs)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", abs)
	}
	if info.Size() > maxReadBytes {
		return "", fmt.Errorf("%s is %dKB; the limit is %dKB", abs, info.Size()/1024, maxReadBytes/1024)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", abs, err)
	}

	// Be liberal in what we accept: a model that quoted read_file's output
	// verbatim carried the display-only line numbers along with it.
	oldString := stripLineNumbers(params.OldString)
	newString := stripLineNumbers(params.NewString)

	out, line, err := applyEdit(string(data), oldString, newString)
	if err != nil {
		return "", err
	}

	if err := writeFileAtomic(abs, []byte(out), info.Mode().Perm()); err != nil {
		return "", err
	}

	return fmt.Sprintf("OK — replaced 1 occurrence in %s at line %d\n", abs, line), nil
}
