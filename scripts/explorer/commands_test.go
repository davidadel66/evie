package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// realRoot is the repository the tool lives in.
var realRoot = filepath.Join("..", "..")

// goldenCommands pins the word paths extracted from Evie's four binaries.
// A change to a binary's subcommands changes this list and data/facts.js
// together (go run ./scripts/explorer extract).
var goldenCommands = []string{
	"evie changes",
	"evie changes close",
	"evie changes disarm",
	"evie changes list",
	"evie changes merge",
	"evie changes push",
	"evie connections",
	"evie connections add",
	"evie connections connect",
	"evie connections disconnect",
	"evie connections list",
	"evie cron-exec",
	"evie memory-backfill",
	"evie memory-backfill cancel",
	"evie memory-backfill resume",
	"evie memory-backfill select",
	"evie memory-backfill status",
	"evie memory-background",
	"evie memory-background status",
	"evie memory-candidates",
	"evie memory-candidates cancel",
	"evie memory-candidates inspect",
	"evie memory-candidates resume",
	"evie memory-candidates status",
	"evie memory-compile",
	"evie memory-compiler",
	"evie memory-compiler activate",
	"evie memory-compiler disable",
	"evie memory-compiler resume",
	"evie memory-compiler status",
	"evie memory-health",
	"evie memory-review",
	"evie memory-review alternatives",
	"evie memory-review batch-inspect",
	"evie memory-review batch-prepare",
	"evie memory-review batch-resolve",
	"evie memory-review choose",
	"evie memory-review edit",
	"evie memory-review edit-revision",
	"evie memory-review identity-revision",
	"evie memory-review inbox",
	"evie memory-review inspect",
	"evie memory-review lineage",
	"evie memory-review operation",
	"evie memory-review prepare",
	"evie memory-review resolve",
	"evie memory-review temporal-choose",
	"evie memory-review temporal-options",
	"evie memory-review temporal-revision",
	"evie plugins",
	"evie plugins disable",
	"evie plugins enable",
	"evie plugins list",
	"evie presets",
	"evie presets list",
	"evie presets validate",
	"evie projects",
	"evie projects list",
	"evie projects verify",
	"evie serve",
	"evie sessions",
	"evie sessions inspect",
	"evie skills",
	"evie skills approve",
	"evie skills deactivate",
	"evie skills list",
	"evie skills propose",
	"evie skills show",
	"evie workflows",
	"evie workflows answer",
	"evie workflows approve",
	"evie workflows builtin",
	"evie workflows builtin disable",
	"evie workflows builtin enable",
	"evie workflows builtin list",
	"evie workflows cancel",
	"evie workflows customize",
	"evie workflows deactivate",
	"evie workflows diff",
	"evie workflows global",
	"evie workflows global deactivate",
	"evie workflows global list",
	"evie workflows global show",
	"evie workflows inspect",
	"evie workflows interruptions",
	"evie workflows list",
	"evie workflows migrate",
	"evie workflows notifications",
	"evie workflows propose",
	"evie workflows reconcile",
	"evie workflows report",
	"evie workflows retention",
	"evie workflows retention status",
	"evie workflows retention sweep",
	"evie workflows run",
	"evie workflows runs",
	"evie workflows schedule",
	"evie workflows schedule pause",
	"evie workflows schedule resume",
	"evie workflows schedules",
	"evie workflows show",
	"evie workflows simulate",
	"evie workflows status",
	"evie workflows substitute",
	"evie workflows templates",
	"evie workflows validate",
	"evie workspaces",
	"evie workspaces access",
	"evie workspaces build",
	"evie workspaces explore",
	"evie workspaces list",
	"evie workspaces mcp-tools",
	"evie workspaces verify",
	"evie worktrees",
	"evie worktrees clean",
	"evie worktrees list",
	"finance categorize",
	"finance db",
	"finance help",
	"finance link",
	"finance rules",
	"finance sync",
	"todo add",
	"todo block",
	"todo cancel",
	"todo delete",
	"todo done",
	"todo get",
	"todo help",
	"todo list",
	"todo release-claim",
	"todo reopen",
	"todo show",
	"todo start",
	"todo update",
	"ytscribe fetch",
	"ytscribe help",
	"ytscribe import",
	"ytscribe scrape",
}

// notInUsage are extracted paths that no usage string mentions, each with
// the reason. Every other extracted path is reconciled against its
// binary's usage strings.
var notInUsage = map[string]string{
	"evie memory-compile":              "runCompilerManagement prints no usage string for compile",
	"evie memory-compiler activate":    "retired: runCompilerActivationManagement answers errCompilerActivationRetired",
	"evie memory-compiler resume":      "retired: runCompilerActivationManagement answers errCompilerActivationRetired",
	"evie memory-health":               "runCompilerDiagnostics prints no usage string",
	"evie memory-review batch-inspect": "runOwnerBatchReviewManagement prints no usage string",
	"evie memory-review batch-prepare": "runOwnerBatchReviewManagement prints no usage string",
	"evie memory-review batch-resolve": "runOwnerBatchReviewManagement prints no usage string",
	"evie memory-review edit":          "runOwnerBatchReviewManagement prints no usage string",
	"evie memory-review edit-revision": "runOwnerBatchReviewManagement prints no usage string",
}

// missedByTheRule are paths a usage string documents that the extraction
// rule does not find, each with the reason. They stay absent from the
// facts until the code compares them with the argument slice itself.
var missedByTheRule = map[string]string{
	"evie workflows notifications read":   "compared with a filtered copy of the arguments (rest), not the argument slice",
	"evie workflows notifications unread": "compared with a filtered copy of the arguments (rest), not the argument slice",
}

func TestExtractCommandsOfTheRealBinaries(t *testing.T) {
	facts, err := extractFacts(realRoot, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	var got []string
	for _, c := range facts.Commands {
		got = append(got, strings.Join(append([]string{c.Binary}, c.Words...), " "))
	}
	if !reflect.DeepEqual(got, goldenCommands) {
		t.Fatalf("commands =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(goldenCommands, "\n"))
	}
}

// TestRealCommandsAreReconciledWithTheirUsageStrings checks each golden path
// against the usage strings of its binary: every word of the path appears,
// in order, in one string the binary's package prints as usage.
func TestRealCommandsAreReconciledWithTheirUsageStrings(t *testing.T) {
	usage := map[string][][]string{}
	for _, binary := range []string{"evie", "finance", "todo", "ytscribe"} {
		usage[binary] = usageTokens(t, filepath.Join(realRoot, "cmd", binary))
	}
	golden := map[string]bool{}
	for _, path := range goldenCommands {
		golden[path] = true
		words := strings.Fields(path)
		documented := mentions(usage[words[0]], words[1:])
		reason, excepted := notInUsage[path]
		switch {
		case !documented && !excepted:
			t.Errorf("%q is in no usage string of %s; document it, or add it to notInUsage with the reason", path, words[0])
		case documented && excepted:
			t.Errorf("%q is in a usage string now; remove it from notInUsage (%s)", path, reason)
		}
	}
	for path := range notInUsage {
		if !golden[path] {
			t.Errorf("notInUsage names %q, which is not extracted", path)
		}
	}
	for path, reason := range missedByTheRule {
		words := strings.Fields(path)
		if golden[path] {
			t.Errorf("%q is extracted now; remove it from missedByTheRule (%s)", path, reason)
		}
		if !mentions(usage[words[0]], words[1:]) {
			t.Errorf("missedByTheRule names %q, which no usage string of %s documents", path, words[0])
		}
	}
}

// mentions reports whether one usage string holds every word, in order.
func mentions(strings [][]string, words []string) bool {
	for _, tokens := range strings {
		i := 0
		for _, token := range tokens {
			if i < len(words) && token == words[i] {
				i++
			}
		}
		if i == len(words) {
			return true
		}
	}
	return false
}

// usageTokens returns the words of every usage string a command package
// prints: each string literal, or constant built from literals, whose text
// contains "usage".
func usageTokens(t *testing.T, dir string) [][]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	consts := map[string]ast.Expr{}
	var files []*ast.File
	for _, entry := range entries {
		if !isSourceFile(entry.Name()) {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
		for _, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range gen.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) == len(vs.Values) {
						for i, name := range vs.Names {
							consts[name.Name] = vs.Values[i]
						}
					}
				}
			}
		}
	}
	var eval func(ast.Expr) (string, bool)
	eval = func(expr ast.Expr) (string, bool) {
		switch e := expr.(type) {
		case *ast.BasicLit:
			if e.Kind == token.STRING {
				s, err := strconv.Unquote(e.Value)
				return s, err == nil
			}
		case *ast.ParenExpr:
			return eval(e.X)
		case *ast.Ident:
			if v, ok := consts[e.Name]; ok {
				return eval(v)
			}
		case *ast.BinaryExpr:
			if e.Op == token.ADD {
				x, okX := eval(e.X)
				y, okY := eval(e.Y)
				return x + y, okX && okY
			}
		}
		return "", false
	}
	var texts []string
	for _, v := range consts {
		if s, ok := eval(v); ok {
			texts = append(texts, s)
		}
	}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok {
				if s, ok := eval(lit); ok {
					texts = append(texts, s)
				}
			}
			return true
		})
	}
	var out [][]string
	for _, text := range texts {
		if !strings.Contains(strings.ToLower(text), "usage") {
			continue
		}
		out = append(out, strings.FieldsFunc(text, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-')
		}))
	}
	return out
}
