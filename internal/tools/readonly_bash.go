package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

// The explore worker's bash (Stage 7, 2026-10-03) runs only commands that are
// provably read-only, in the spirit of Claude Code's read-only validation:
// anything not provably read-only is refused. Unlike the owner's bash it
// never starts a shell. The command line is parsed here into argument
// vectors, every command, flag and path is checked against an allowlist, and
// only then are the programs executed directly, connected by pipes in Go.
// Without a shell there are no aliases, functions, profiles, expansions or
// redirections to reason about: what was checked is exactly what runs.

const (
	// maxWorkerBashOutput caps what comes back inline. There is no spill
	// file: it would sit outside the worker's root, where its tools cannot
	// read it, so the worker narrows the command instead.
	maxWorkerBashOutput = 30_000
	// maxWorkerArgs bounds one command's arguments after glob expansion.
	maxWorkerArgs = 4096
	// maxWorkerCommandBytes bounds the command line itself.
	maxWorkerCommandBytes = 16 << 10
)

var readOnlyBashTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "bash",
		Description: `Run a read-only command inside this assignment's root folder and get its output. Only vetted read-only commands run: ls, cat, head, tail, wc, find, rg, grep, git (log, diff, show, status, blame, ls-files), sqlite3 -readonly with one SELECT or .tables/.schema/.indexes, pwd, and a leading cd inside the root.

There is no shell. Pipes between these commands, &&, || and ; work, quotes work, and globs (*, ?, [...]) expand inside the root. Redirections, $ expansion, backticks, subshells, environment assignments, paths outside the root (including through .. or symlinks), secret files, and flags that could write, follow links out or run other programs (for example find -exec, git -c, rg --pre) are refused before anything runs.

Output is stdout and stderr followed by the exit status; a non-zero exit is information, not an error. Output past 30000 bytes is cut: narrow the command (head, rg -m, a more specific path). Commands are killed after 2 minutes by default; raise timeout_seconds (up to 600) for a slow search.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"command"},
			Properties: map[string]openrouter.Property{
				"command": {
					Type:        "string",
					Description: "The read-only command line to run.",
				},
				"cwd": {
					Type:        "string",
					Description: "Folder to start in, inside the root: relative to the root, or absolute. Defaults to the root.",
				},
				"timeout_seconds": {
					Type:        "integer",
					Description: "Seconds before the command is killed. Defaults to 120, maximum 600.",
				},
			},
		},
	},
}

// runReadOnlyBash vets the whole command line, then runs it. A refused line
// runs nothing, not even an allowed first command.
func runReadOnlyBash(parent context.Context, args string) (string, error) {
	root, err := workerRootFrom(parent)
	if err != nil {
		return "", err
	}
	var params struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
		Timeout int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	cwd := root
	if strings.TrimSpace(params.Cwd) != "" {
		if cwd, err = workerDirectory(root, root, strings.TrimSpace(params.Cwd)); err != nil {
			return "", fmt.Errorf("cwd: %w", err)
		}
	}
	plan, err := planReadOnly(root, cwd, params.Command)
	if err != nil {
		return "", fmt.Errorf("refused: explore workers run only vetted read-only commands, so nothing ran: %w", err)
	}
	timeout := defaultBashTimeout
	if params.Timeout > 0 {
		timeout = min(time.Duration(params.Timeout)*time.Second, maxBashTimeout)
	}
	if err := parent.Err(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	out := &cappedOutput{}
	status, err := plan.run(ctx, out)
	if parent.Err() != nil {
		return "", parent.Err()
	}
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %s and was killed. Partial output:\n%s", timeout, out.render())
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\nexit status: %d\n", out.render(), status), nil
}

// workerDirectory resolves a cd or cwd target: an existing directory inside
// root.
func workerDirectory(root, cwd, target string) (string, error) {
	dir, err := confinePath(root, cwd, target)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", target)
	}
	return dir, nil
}

// readOnlyPlan is a vetted command line: pipelines joined by &&, || or ;,
// each stage a complete argument vector, all run in dir.
type readOnlyPlan struct {
	root, dir string
	pipelines [][]vettedCommand
	ops       []string
}

// vettedCommand is one stage. pwd is answered by the harness; everything
// else is a program found on PATH.
type vettedCommand struct {
	argv []string
	pwd  bool
}

// planReadOnly parses and vets command for a worker rooted at root starting
// in cwd. It touches the filesystem only to resolve paths and globs.
func planReadOnly(root, cwd, command string) (readOnlyPlan, error) {
	if len(command) > maxWorkerCommandBytes {
		return readOnlyPlan{}, fmt.Errorf("command is %d bytes; limit %d", len(command), maxWorkerCommandBytes)
	}
	list, err := parseShellLine(command)
	if err != nil {
		return readOnlyPlan{}, err
	}
	plan := readOnlyPlan{root: root, dir: cwd, ops: list.ops}
	for i, pipeline := range list.pipelines {
		if first := pipeline[0]; first[0].value == "cd" {
			if i != 0 || len(pipeline) != 1 || (len(list.ops) > 0 && list.ops[0] == "||") {
				return readOnlyPlan{}, errors.New("cd may only start the line, alone, followed by && or ; (or use the cwd argument)")
			}
			if len(first) > 2 {
				return readOnlyPlan{}, errors.New("cd takes one folder")
			}
			target := root
			if len(first) == 2 {
				if target, err = workerDirectory(root, cwd, first[1].value); err != nil {
					return readOnlyPlan{}, fmt.Errorf("cd: %w", err)
				}
			}
			plan.dir = target
			plan.pipelines = append(plan.pipelines, nil)
			continue
		}
		var stages []vettedCommand
		for _, words := range pipeline {
			stage, err := vetCommand(root, plan.dir, words, len(pipeline) == 1)
			if err != nil {
				return readOnlyPlan{}, err
			}
			stages = append(stages, stage)
		}
		plan.pipelines = append(plan.pipelines, stages)
	}
	return plan, nil
}

// envAssignment matches a leading NAME=value word, which a shell would treat
// as an environment assignment for the command after it.
var envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// vetCommand checks one command and returns its final argument vector, with
// globs expanded and the harness's own safety flags added.
func vetCommand(root, cwd string, words []shellWord, alone bool) (vettedCommand, error) {
	name := words[0]
	if envAssignment.MatchString(name.value) {
		return vettedCommand{}, fmt.Errorf("environment assignment %q is not allowed", name.value)
	}
	if strings.ContainsRune(name.value, '/') {
		return vettedCommand{}, fmt.Errorf("run commands by name, not by path (%q)", name.value)
	}
	var args []string
	for _, word := range words[1:] {
		expanded, err := expandWord(root, cwd, word)
		if err != nil {
			return vettedCommand{}, err
		}
		args = append(args, expanded...)
		if len(args) > maxWorkerArgs {
			return vettedCommand{}, fmt.Errorf("%s has more than %d arguments after glob expansion; narrow the pattern", name.value, maxWorkerArgs)
		}
	}
	switch name.value {
	case "pwd":
		if len(args) > 0 || !alone {
			return vettedCommand{}, errors.New("pwd takes no arguments and runs alone")
		}
		return vettedCommand{pwd: true}, nil
	case "cd":
		return vettedCommand{}, errors.New("cd may only start the line, alone, followed by && or ; (or use the cwd argument)")
	case "ls", "cat", "head", "tail", "wc":
		paths, err := scanFlags(name.value, simpleCommands[name.value], args)
		if err != nil {
			return vettedCommand{}, err
		}
		if err := confineAll(root, cwd, paths.paths, paths.positionals); err != nil {
			return vettedCommand{}, err
		}
		return vettedCommand{argv: append([]string{name.value}, args...)}, nil
	case "grep", "rg":
		return vetSearch(root, cwd, name.value, args)
	case "find":
		return vetFind(root, cwd, args)
	case "git":
		return vetGit(root, cwd, args)
	case "sqlite3":
		return vetSQLite(root, cwd, args)
	}
	return vettedCommand{}, fmt.Errorf("%q is not an allowed read-only command (allowed: ls, cat, head, tail, wc, find, rg, grep, git log/diff/show/status/blame/ls-files, sqlite3 -readonly, pwd, cd)", name.value)
}

func confineAll(root, cwd string, groups ...[]string) error {
	for _, group := range groups {
		for _, arg := range group {
			if _, err := confinePath(root, cwd, arg); err != nil {
				return err
			}
		}
	}
	return nil
}

// Flag argument kinds. A value is checked as a path unless the flag is
// known to take a pattern, so a flag this table misjudges can still never
// carry a path outside the root.
type flagKind int

const (
	flagBool     flagKind = iota + 1
	flagValue             // takes a value; checked as a path
	flagPattern           // takes a regex, glob, format or text; not a path
	flagPath              // takes a path
	flagOptional          // takes an optional value attached with = (or, for a short flag, directly)
)

type flagTable struct {
	short   map[byte]flagKind
	long    map[string]flagKind
	numeric bool // -NUM, as in head -20 or git log -5
}

func shortFlags(kind flagKind, letters string) map[byte]flagKind {
	m := make(map[byte]flagKind, len(letters))
	for i := 0; i < len(letters); i++ {
		m[letters[i]] = kind
	}
	return m
}

func mergeShort(maps ...map[byte]flagKind) map[byte]flagKind {
	out := map[byte]flagKind{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func longFlags(kind flagKind, names ...string) map[string]flagKind {
	m := make(map[string]flagKind, len(names))
	for _, name := range names {
		m[name] = kind
	}
	return m
}

func mergeLong(maps ...map[string]flagKind) map[string]flagKind {
	out := map[string]flagKind{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// simpleCommands read only the files they are given. ls -L, tail -f/-F and
// the like are absent: they follow links out of the root or never end.
var simpleCommands = map[string]flagTable{
	"ls": {
		short: shortFlags(flagBool, "1@ABCFGORSTUWabcdefghiklmnopqrstuvwx"),
		long: mergeLong(longFlags(flagBool, "all", "almost-all", "human-readable", "recursive", "reverse", "size", "classify",
			"directory", "inode", "numeric-uid-gid", "no-group", "full-time", "group-directories-first", "literal", "si"),
			longFlags(flagOptional, "color", "colour"),
			longFlags(flagPattern, "sort", "time", "format", "time-style", "quoting-style", "indicator-style", "ignore", "hide", "width", "block-size")),
	},
	"cat": {
		short: shortFlags(flagBool, "AbeEnstTuv"),
		long:  longFlags(flagBool, "number", "number-nonblank", "squeeze-blank", "show-all", "show-ends", "show-tabs", "show-nonprinting"),
	},
	"head": {
		short:   mergeShort(shortFlags(flagBool, "qvz"), shortFlags(flagValue, "nc")),
		long:    mergeLong(longFlags(flagBool, "quiet", "silent", "verbose", "zero-terminated"), longFlags(flagValue, "lines", "bytes")),
		numeric: true,
	},
	"tail": {
		short:   mergeShort(shortFlags(flagBool, "qrvz"), shortFlags(flagValue, "nc")),
		long:    mergeLong(longFlags(flagBool, "quiet", "silent", "verbose", "zero-terminated"), longFlags(flagValue, "lines", "bytes")),
		numeric: true,
	},
	"wc": {
		short: shortFlags(flagBool, "clmwL"),
		long:  longFlags(flagBool, "bytes", "chars", "lines", "words", "max-line-length"),
	},
}

// scannedArgs are a command's positional arguments and the values of its
// path flags, plus which flags it used.
type scannedArgs struct {
	positionals []string
	paths       []string
	seen        map[string]bool
	dashdash    int // index in args of "--", or -1
}

// scanFlags checks every flag in args against t. Any argument that starts
// with - is a flag wherever it appears, until --; an unknown flag, including
// an abbreviation of a known one, is refused.
func scanFlags(command string, t flagTable, args []string) (scannedArgs, error) {
	scanned := scannedArgs{seen: map[string]bool{}, dashdash: -1}
	addValue := func(flag string, kind flagKind, value string) {
		switch kind {
		case flagValue, flagPath:
			scanned.paths = append(scanned.paths, value)
		}
		scanned.seen[flag] = true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			scanned.dashdash = i
			scanned.positionals = append(scanned.positionals, args[i+1:]...)
			return scanned, nil
		case arg == "-" || !strings.HasPrefix(arg, "-"):
			scanned.positionals = append(scanned.positionals, arg)
		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(arg[2:], "=")
			kind, ok := t.long[name]
			if !ok {
				return scanned, fmt.Errorf("%s flag %q is not allowed", command, "--"+name)
			}
			switch kind {
			case flagBool:
				if hasValue {
					return scanned, fmt.Errorf("%s flag %q takes no value", command, "--"+name)
				}
				scanned.seen["--"+name] = true
			case flagOptional:
				scanned.seen["--"+name] = true
			default:
				if !hasValue {
					if i+1 >= len(args) {
						return scanned, fmt.Errorf("%s flag %q needs a value", command, "--"+name)
					}
					i++
					value = args[i]
				}
				addValue("--"+name, kind, value)
			}
		default:
			if t.numeric && isDigits(arg[1:]) {
				continue
			}
			cluster := arg[1:]
			for j := 0; j < len(cluster); j++ {
				letter := cluster[j]
				kind, ok := t.short[letter]
				if !ok {
					return scanned, fmt.Errorf("%s flag %q is not allowed", command, "-"+string(letter))
				}
				flag := "-" + string(letter)
				switch kind {
				case flagBool:
					scanned.seen[flag] = true
					continue
				case flagOptional:
					scanned.seen[flag] = true
				default:
					value := cluster[j+1:]
					if value == "" {
						if i+1 >= len(args) {
							return scanned, fmt.Errorf("%s flag %q needs a value", command, flag)
						}
						i++
						value = args[i]
					}
					addValue(flag, kind, value)
				}
				break
			}
		}
	}
	return scanned, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Search tools. rg's --pre/--pre-glob/-z run other programs and -L/--follow
// and grep's -R/-S/-O follow links out of the root, so none is listed.
var searchCommands = map[string]flagTable{
	"grep": {
		short: mergeShort(shortFlags(flagBool, "abcEFGhHiIlLnoqrsTUvVwxyzp"), shortFlags(flagValue, "ABCmdD"),
			shortFlags(flagPattern, "e"), shortFlags(flagPath, "f")),
		long: mergeLong(longFlags(flagBool, "recursive", "ignore-case", "no-ignore-case", "line-number", "count", "files-with-matches",
			"files-without-match", "only-matching", "quiet", "silent", "no-messages", "invert-match", "word-regexp", "line-regexp",
			"extended-regexp", "fixed-strings", "basic-regexp", "perl-regexp", "with-filename", "no-filename", "text", "null",
			"line-buffered", "byte-offset", "initial-tab"),
			longFlags(flagOptional, "color", "colour"),
			longFlags(flagValue, "max-count", "after-context", "before-context", "context", "binary-files", "directories", "devices"),
			longFlags(flagPattern, "regexp", "include", "exclude", "exclude-dir", "label"),
			longFlags(flagPath, "file", "exclude-from")),
	},
	"rg": {
		short: mergeShort(shortFlags(flagBool, "0abcFhHiIlnNopqsSUuvVwx."), shortFlags(flagValue, "ABCjmM"),
			shortFlags(flagPattern, "egrtTE"), shortFlags(flagPath, "f")),
		long: mergeLong(longFlags(flagBool, "hidden", "no-hidden", "no-ignore", "no-ignore-vcs", "no-ignore-parent", "no-ignore-dot",
			"no-ignore-global", "no-ignore-exclude", "no-ignore-files", "ignore-case", "smart-case", "case-sensitive", "fixed-strings",
			"word-regexp", "line-regexp", "invert-match", "count", "count-matches", "files", "files-with-matches", "files-without-match",
			"line-number", "no-line-number", "with-filename", "no-filename", "heading", "no-heading", "only-matching", "multiline",
			"multiline-dotall", "pcre2", "json", "vimgrep", "stats", "null", "null-data", "text", "binary", "no-messages", "quiet",
			"trim", "column", "no-column", "byte-offset", "no-unicode", "crlf", "passthru", "type-list", "one-file-system",
			"no-config", "unrestricted", "max-columns-preview", "pretty", "include-zero", "glob-case-insensitive", "sort-files"),
			longFlags(flagValue, "max-count", "max-depth", "max-filesize", "max-columns", "context", "after-context", "before-context",
				"threads", "sort", "sortr", "color", "encoding", "engine", "dfa-size-limit", "regex-size-limit"),
			longFlags(flagPattern, "regexp", "glob", "iglob", "type", "type-not", "type-add", "type-clear", "replace", "colors",
				"context-separator", "field-match-separator", "field-context-separator", "path-separator"),
			longFlags(flagPath, "file", "ignore-file")),
	},
}

// secretExcludes keep the read_file denylist's secret file names out of a
// recursive search. Explicitly named secret paths are refused outright.
func secretExcludes(command string) []string {
	names := append([]string{".env*"}, deniedNames...)
	out := make([]string, len(names))
	for i, name := range names {
		if command == "rg" {
			out[i] = "--glob=!" + name
		} else {
			out[i] = "--exclude=" + name
		}
	}
	return out
}

func vetSearch(root, cwd, command string, args []string) (vettedCommand, error) {
	scanned, err := scanFlags(command, searchCommands[command], args)
	if err != nil {
		return vettedCommand{}, err
	}
	positionals := scanned.positionals
	explicitPattern := scanned.seen["-e"] || scanned.seen["--regexp"] || scanned.seen["-f"] || scanned.seen["--file"]
	listing := scanned.seen["--files"] || scanned.seen["--type-list"]
	if !explicitPattern && !listing && len(positionals) > 0 {
		positionals = positionals[1:] // the pattern
	}
	if err := confineAll(root, cwd, scanned.paths, positionals); err != nil {
		return vettedCommand{}, err
	}
	// Harness flags go before any -- so they stay flags. rg reads no user
	// config (which could name a --pre program), and its later globs win,
	// so the secret exclusions hold whatever the worker asked for.
	before, after := args, []string(nil)
	if scanned.dashdash >= 0 {
		before, after = args[:scanned.dashdash], args[scanned.dashdash:]
	}
	argv := []string{command}
	if command == "rg" {
		argv = append(argv, "--no-config")
	}
	argv = append(argv, before...)
	argv = append(argv, secretExcludes(command)...)
	argv = append(argv, after...)
	return vettedCommand{argv: argv}, nil
}

// find: leading options, starting points, then an expression. Only these
// primaries exist here; -exec, -execdir, -ok, -okdir, -delete, -fprint*,
// -fls, -follow and the -H/-L options are absent.
var (
	findLeading   = map[string]bool{"-E": true, "-s": true, "-x": true, "-d": true, "-P": true, "-X": true}
	findOperators = map[string]bool{"!": true, "(": true, ")": true, ",": true, "-not": true, "-and": true, "-or": true, "-a": true, "-o": true}
	findPrimaries = mergeLong(
		longFlags(flagBool, "-print", "-print0", "-prune", "-depth", "-d", "-empty", "-true", "-false", "-xdev", "-mount", "-ls",
			"-readable", "-writable", "-executable", "-nouser", "-nogroup", "-quit"),
		longFlags(flagPattern, "-name", "-iname", "-path", "-ipath", "-wholename", "-iwholename", "-regex", "-iregex", "-lname", "-ilname", "-printf"),
		longFlags(flagValue, "-type", "-xtype", "-maxdepth", "-mindepth", "-size", "-mtime", "-mmin", "-atime", "-amin", "-ctime", "-cmin",
			"-perm", "-user", "-group", "-uid", "-gid", "-links", "-inum", "-fstype", "-regextype", "-newermt", "-newerct", "-newerat"),
		longFlags(flagPath, "-newer", "-anewer", "-cnewer", "-samefile"),
	)
)

func vetFind(root, cwd string, args []string) (vettedCommand, error) {
	i := 0
	for i < len(args) && findLeading[args[i]] {
		i++
	}
	if i < len(args) && strings.HasPrefix(args[i], "-") && !findOperators[args[i]] {
		if _, primary := findPrimaries[args[i]]; !primary {
			return vettedCommand{}, fmt.Errorf("find option %q is not allowed", args[i])
		}
	}
	var starts []string
	for ; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") || findOperators[args[i]] {
			break
		}
		starts = append(starts, args[i])
	}
	var paths []string
	for ; i < len(args); i++ {
		arg := args[i]
		if findOperators[arg] {
			continue
		}
		kind, ok := findPrimaries[arg]
		if !ok {
			return vettedCommand{}, fmt.Errorf("find primary %q is not allowed", arg)
		}
		if kind == flagBool {
			continue
		}
		if i+1 >= len(args) {
			return vettedCommand{}, fmt.Errorf("find primary %q needs a value", arg)
		}
		i++
		if kind == flagValue || kind == flagPath {
			paths = append(paths, args[i])
		}
	}
	if err := confineAll(root, cwd, starts, paths); err != nil {
		return vettedCommand{}, err
	}
	return vettedCommand{argv: append([]string{"find"}, args...)}, nil
}

// git: only these subcommands and flags. Every global option (-c, -C,
// --exec-path, --git-dir, --work-tree, --config-env, -p ...) is refused, as
// are --output, --ext-diff, --textconv, --no-index, -O, --show-signature,
// blame --contents/-S/--ignore-revs-file, and every write subcommand.
var (
	gitDiffFlags = flagTable{
		short: mergeShort(shortFlags(flagBool, "puszRwbWaDB"), shortFlags(flagOptional, "MCl"),
			shortFlags(flagValue, "U"), shortFlags(flagPattern, "SG")),
		long: mergeLong(longFlags(flagBool, "patch", "no-patch", "raw", "patch-with-raw", "patch-with-stat", "numstat", "shortstat", "summary",
			"name-only", "name-status", "compact-summary", "full-index", "binary", "no-renames", "no-color", "color-words",
			"ignore-all-space", "ignore-space-change", "ignore-blank-lines", "ignore-space-at-eol", "ignore-cr-at-eol", "minimal",
			"patience", "histogram", "text", "function-context", "cached", "staged", "exit-code", "quiet", "irreversible-delete",
			"no-prefix", "pickaxe-all", "pickaxe-regex", "no-ext-diff", "no-textconv", "merge-base", "find-copies-harder", "check", "ita-invisible-in-index"),
			longFlags(flagOptional, "stat", "dirstat", "color", "word-diff", "submodule", "ignore-submodules", "relative", "find-renames",
				"find-copies", "abbrev", "color-moved", "break-rewrites"),
			longFlags(flagValue, "unified", "inter-hunk-context", "diff-algorithm", "stat-width", "stat-name-width", "stat-count", "word-diff-regex",
				"diff-filter", "src-prefix", "dst-prefix", "line-prefix", "anchored", "find-object", "ws-error-highlight"),
		),
		numeric: true,
	}
	gitLogFlags = flagTable{
		short: mergeShort(gitDiffFlags.short, shortFlags(flagBool, "giEFPm"), shortFlags(flagValue, "n"), shortFlags(flagPattern, "L")),
		long: mergeLong(gitDiffFlags.long,
			longFlags(flagBool, "oneline", "graph", "no-decorate", "all", "first-parent", "no-merges", "merges", "reverse", "date-order",
				"author-date-order", "topo-order", "follow", "abbrev-commit", "no-abbrev-commit", "left-right", "boundary", "cherry-pick",
				"cherry-mark", "cherry", "source", "full-history", "simplify-merges", "simplify-by-decoration", "ancestry-path", "log-size",
				"relative-date", "parents", "children", "walk-reflogs", "no-notes", "regexp-ignore-case", "all-match", "invert-grep",
				"extended-regexp", "fixed-strings", "perl-regexp", "basic-regexp", "no-walk", "do-walk", "expand-tabs", "no-expand-tabs",
				"no-mailmap", "use-mailmap", "mailmap", "dense", "sparse", "full-diff", "remove-empty", "show-pulls", "count"),
			longFlags(flagOptional, "decorate", "branches", "tags", "remotes", "notes", "pretty", "glob"),
			longFlags(flagValue, "max-count", "skip", "since", "after", "until", "before", "date", "min-parents", "max-parents", "encoding", "exclude"),
			longFlags(flagPattern, "author", "committer", "grep", "grep-reflog", "format", "decorate-refs", "decorate-refs-exclude"),
		),
		numeric: true,
	}
	gitCommands = map[string]flagTable{
		"diff": gitDiffFlags,
		"log":  gitLogFlags,
		"show": gitLogFlags,
		"status": {
			short: mergeShort(shortFlags(flagBool, "sbvz"), shortFlags(flagOptional, "u")),
			long: mergeLong(longFlags(flagBool, "short", "branch", "long", "verbose", "no-column", "ahead-behind", "no-ahead-behind",
				"renames", "no-renames", "show-stash", "null"),
				longFlags(flagOptional, "porcelain", "untracked-files", "ignored", "ignore-submodules", "find-renames", "column")),
		},
		"blame": {
			short: mergeShort(shortFlags(flagBool, "lstewpfncb"), shortFlags(flagOptional, "MC"), shortFlags(flagPattern, "L")),
			long: mergeLong(longFlags(flagBool, "porcelain", "line-porcelain", "incremental", "show-name", "show-number", "root",
				"show-stats", "show-email", "color-lines", "color-by-age", "minimal", "first-parent", "no-progress", "progress", "score-debug"),
				longFlags(flagOptional, "abbrev"),
				longFlags(flagValue, "date", "reverse", "ignore-rev", "encoding")),
		},
		"ls-files": {
			short: mergeShort(shortFlags(flagBool, "cdmoiskutvfz"), shortFlags(flagPattern, "x"), shortFlags(flagPath, "X")),
			long: mergeLong(longFlags(flagBool, "cached", "deleted", "modified", "others", "ignored", "stage", "unmerged", "killed",
				"directory", "no-empty-directory", "eol", "full-name", "recurse-submodules", "debug", "error-unmatch", "exclude-standard",
				"deduplicate", "sparse", "resolve-undo"),
				longFlags(flagOptional, "abbrev"),
				longFlags(flagPattern, "exclude", "exclude-per-directory", "with-tree", "format"),
				longFlags(flagPath, "exclude-from")),
		},
	}
)

func vetGit(root, cwd string, args []string) (vettedCommand, error) {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if args[i] != "--no-pager" && args[i] != "-P" {
			name, _, _ := strings.Cut(args[i], "=")
			return vettedCommand{}, fmt.Errorf("git global option %q is not allowed", name)
		}
		i++
	}
	if i >= len(args) {
		return vettedCommand{}, errors.New("git needs a subcommand: log, diff, show, status, blame or ls-files")
	}
	sub := args[i]
	table, ok := gitCommands[sub]
	if !ok {
		return vettedCommand{}, fmt.Errorf("git %q is not allowed; read-only git is log, diff, show, status, blame and ls-files", sub)
	}
	rest := args[i+1:]
	scanned, err := scanFlags("git "+sub, table, rest)
	if err != nil {
		return vettedCommand{}, err
	}
	// Revisions and pathspecs alike are judged as paths: a revision such as
	// HEAD~2 resolves inside the root, and git refuses pathspecs outside its
	// repository, which GIT_CEILING_DIRECTORIES keeps inside the root.
	if err := confineAll(root, cwd, scanned.paths, scanned.positionals); err != nil {
		return vettedCommand{}, err
	}
	// The repository's own configuration could still name a pager, a
	// fsmonitor hook or a signature program; the harness turns each off.
	argv := []string{"git", "--no-pager", "-c", "core.fsmonitor=false", "-c", "core.pager=cat", "-c", "log.showSignature=false", sub}
	switch sub {
	case "diff", "log", "show":
		argv = append(argv, "--no-ext-diff", "--no-textconv")
	}
	return vettedCommand{argv: append(argv, rest...)}, nil
}

// sqlite3 opens one database read-only in safe mode, which also refuses
// writefile, readfile, load_extension, ATTACH and dangerous dot commands.
var (
	sqliteFlags = map[string]flagKind{
		"readonly": flagBool, "header": flagBool, "noheader": flagBool, "csv": flagBool, "json": flagBool, "line": flagBool,
		"list": flagBool, "column": flagBool, "box": flagBool, "markdown": flagBool, "table": flagBool, "tabs": flagBool,
		"quote": flagBool, "ascii": flagBool, "html": flagBool, "bail": flagBool, "batch": flagBool, "nofollow": flagBool, "safe": flagBool,
		"separator": flagPattern, "nullvalue": flagPattern, "newline": flagPattern,
	}
	sqliteDotCommand = regexp.MustCompile(`^\.(tables|schema|indexes|indices)(\s+[A-Za-z0-9_%]+)?$`)
)

func vetSQLite(root, cwd string, args []string) (vettedCommand, error) {
	var options, positionals []string
	readonly := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
		kind, ok := sqliteFlags[name]
		if !ok {
			return vettedCommand{}, fmt.Errorf("sqlite3 option %q is not allowed", arg)
		}
		readonly = readonly || name == "readonly"
		options = append(options, "-"+name)
		if kind == flagPattern {
			if i+1 >= len(args) {
				return vettedCommand{}, fmt.Errorf("sqlite3 option %q needs a value", arg)
			}
			i++
			options = append(options, args[i])
		}
	}
	if !readonly {
		return vettedCommand{}, errors.New("sqlite3 must be run with -readonly")
	}
	if len(positionals) != 2 {
		return vettedCommand{}, errors.New("sqlite3 needs exactly a database file and one SQL statement")
	}
	database, query := positionals[0], strings.TrimSpace(positionals[1])
	if strings.HasPrefix(database, "file:") || strings.HasPrefix(database, ":") {
		return vettedCommand{}, fmt.Errorf("sqlite3 database %q must be a plain file path, not a file: URI or special name", database)
	}
	path, err := confinePath(root, cwd, database)
	if err != nil {
		return vettedCommand{}, err
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return vettedCommand{}, fmt.Errorf("sqlite3 database %q is not an existing file", database)
	}
	if !sqliteDotCommand.MatchString(query) {
		if err := validateSingleSelect(query, "sqlite3"); err != nil {
			return vettedCommand{}, err
		}
	}
	argv := append([]string{"sqlite3", "-readonly", "-safe", "-nofollow", "-batch", "-bail", "-init", os.DevNull}, options...)
	return vettedCommand{argv: append(argv, "--", database, query)}, nil
}

// run executes the plan's pipelines in order with the shell's &&, || and ;
// semantics and returns the exit status of the last pipeline that ran.
func (p readOnlyPlan) run(ctx context.Context, out *cappedOutput) (int, error) {
	env := workerEnvironment(p.root)
	status := 0
	for i, pipeline := range p.pipelines {
		if i > 0 {
			switch p.ops[i-1] {
			case "&&":
				if status != 0 {
					continue
				}
			case "||":
				if status == 0 {
					continue
				}
			}
		}
		if pipeline == nil { // the leading cd, already applied to p.dir
			status = 0
			continue
		}
		var err error
		if status, err = runPipeline(ctx, p.dir, env, pipeline, out); err != nil {
			return status, err
		}
	}
	return status, nil
}

// workerEnvironment is the whole environment of a worker command: nothing
// else is inherited, so GIT_DIR, GIT_EXTERNAL_DIFF, RIPGREP_CONFIG_PATH and
// the like cannot redirect or extend a command. Git never searches for a
// repository above the root and never takes optional locks, so status does
// not rewrite the index.
func workerEnvironment(root string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"TERM=dumb", "NO_COLOR=1", "PAGER=cat", "GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CEILING_DIRECTORIES=" + filepath.Dir(root),
	}
	for _, name := range []string{"HOME", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// runPipeline starts every stage connected by pipes, with stderr from all of
// them and the last stage's stdout going to out. The first stage reads
// nothing. The status is the last stage's, as in a shell without pipefail.
func runPipeline(ctx context.Context, dir string, env []string, stages []vettedCommand, out *cappedOutput) (int, error) {
	if len(stages) == 1 && stages[0].pwd {
		_, _ = out.Write([]byte(dir + "\n"))
		return 0, nil
	}
	commands := make([]*exec.Cmd, len(stages))
	for i, stage := range stages {
		path, err := exec.LookPath(stage.argv[0])
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s: command not found on PATH\n", stage.argv[0])
			return 127, nil
		}
		cmd := exec.CommandContext(ctx, path, stage.argv[1:]...)
		cmd.Dir, cmd.Env, cmd.Stderr = dir, env, out
		cmd.WaitDelay = 2 * time.Second
		commands[i] = cmd
	}
	var parentEnds []io.Closer
	closeAll := func() {
		for _, c := range parentEnds {
			c.Close()
		}
		parentEnds = nil
	}
	for i := 0; i+1 < len(commands); i++ {
		r, w, err := os.Pipe()
		if err != nil {
			closeAll()
			return 0, fmt.Errorf("connect pipeline: %w", err)
		}
		commands[i].Stdout, commands[i+1].Stdin = w, r
		parentEnds = append(parentEnds, r, w)
	}
	commands[len(commands)-1].Stdout = out
	started := 0
	var startErr error
	for _, cmd := range commands {
		if startErr = cmd.Start(); startErr != nil {
			break
		}
		started++
	}
	// The children hold their own copies; closing ours lets a reader see
	// end of input and a writer see a closed pipe when its reader exits.
	closeAll()
	status := 0
	for i := 0; i < started; i++ {
		err := commands[i].Wait()
		if i == len(commands)-1 {
			var exitErr *exec.ExitError
			switch {
			case err == nil:
			case errors.As(err, &exitErr):
				status = exitErr.ExitCode()
			default:
				return 0, fmt.Errorf("run %s: %w", commands[i].Args[0], err)
			}
		}
	}
	if startErr != nil {
		return 0, fmt.Errorf("start %s: %w", commands[started].Args[0], startErr)
	}
	return status, nil
}

// cappedOutput keeps the first maxWorkerBashOutput bytes of a command's
// combined output and counts the rest. It is shared by every stage's
// stderr, so writes are serialized.
type cappedOutput struct {
	mu    sync.Mutex
	head  []byte
	total int64
}

func (o *cappedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.total += int64(len(p))
	if room := maxWorkerBashOutput - len(o.head); room > 0 {
		o.head = append(o.head, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

func (o *cappedOutput) render() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.total <= maxWorkerBashOutput {
		return string(o.head)
	}
	cut := completeRunePrefix(o.head)
	return string(o.head[:cut]) + fmt.Sprintf("\n\n[output trimmed: %d of %d bytes shown; narrow the command (head, rg -m, a more specific path) to see the rest]", cut, o.total)
}
