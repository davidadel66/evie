package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Reference kinds. Every reference is "<kind>:<target>".
var refFixes = map[string]string{
	"code":    "point it at an existing repository file or directory, with #Name or #Type.Method naming a declaration in that Go file",
	"doc":     "point it at an existing repository file, with #anchor naming one of its Markdown headings (GitHub slug)",
	"adr":     "use the four-digit number of an ADR in docs/adr",
	"term":    "use a term exactly as GLOSSARY.md spells it",
	"table":   "use database.table from the facts databases",
	"route":   "use a route from the facts as [METHOD ]pattern",
	"command": "use a binary and word path from the facts commands",
	"pkg":     "use a package path from the facts",
	"node":    "use the id of an existing explorer node",
}

const refKindsFix = "use one of code:, doc:, adr:, term:, table:, route:, command:, pkg:, node:"

// statement checks a statement's text, that it cites at least one source,
// and that every reference resolves.
func (c *checker) statement(file, item string, s *Statement, extra map[string]bool) {
	if s == nil {
		return
	}
	if strings.TrimSpace(s.Text) == "" {
		c.missing(file, item, "text")
	}
	if len(s.Refs) == 0 {
		c.add(file, item, "the statement has no source", "cite at least one reference in refs")
	}
	c.refs(file, item, s.Refs, extra)
}

func (c *checker) refs(file, item string, refs []string, extra map[string]bool) {
	for _, ref := range refs {
		c.ref(file, item, ref, extra)
	}
}

// ref reports ref when it does not resolve. extra holds node ids that exist
// only inside the design proposal being checked.
func (c *checker) ref(file, item, ref string, extra map[string]bool) {
	kind, _, _ := strings.Cut(ref, ":")
	why := c.resolve(ref, extra)
	if why == "" {
		return
	}
	fix, ok := refFixes[kind]
	if !ok {
		fix = refKindsFix
	}
	c.add(file, item, fmt.Sprintf("dead reference %q: %s", ref, why), fix)
}

// resolve returns "" when ref resolves, or why it does not.
func (c *checker) resolve(ref string, extra map[string]bool) string {
	kind, target, ok := strings.Cut(ref, ":")
	if !ok || target == "" {
		return "unknown reference kind; want kind:target"
	}
	switch kind {
	case "code":
		return c.resolveCode(target)
	case "doc":
		return c.resolveDoc(target)
	case "adr":
		for _, adr := range c.facts.ADRs {
			if adr.Number == target {
				return ""
			}
		}
		return "there is no ADR " + target
	case "term":
		for _, term := range c.facts.Glossary {
			if term.Term == target {
				return ""
			}
		}
		return "there is no glossary term " + target
	case "table":
		db, table, _ := strings.Cut(target, ".")
		for _, d := range c.facts.Databases {
			for _, t := range d.Tables {
				if d.ID == db && t.Name == table {
					return ""
				}
			}
		}
		return "there is no table " + target + " in the facts"
	case "route":
		method, pattern := splitRoute(target)
		for _, r := range c.facts.Routes {
			if r.Pattern == pattern && (method == "" || r.Method == method) {
				return ""
			}
		}
		return "there is no route " + target + " in the facts"
	case "command":
		words := strings.Fields(target)
		if len(words) == 0 {
			return "the reference names no binary"
		}
		for _, cmd := range c.facts.Commands {
			if cmd.Binary == words[0] && (len(words) == 1 || slices.Equal(cmd.Words, words[1:])) {
				return ""
			}
		}
		return "there is no command " + target + " in the facts"
	case "pkg":
		if c.packages[target] {
			return ""
		}
		return "there is no package " + target
	case "node":
		if _, ok := c.nodes[target]; ok || extra[target] {
			return ""
		}
		return "there is no node " + target
	}
	return "unknown reference kind " + kind
}

// splitRoute splits "GET /x" into its method and pattern; "/x" has none.
func splitRoute(target string) (string, string) {
	method, pattern, ok := strings.Cut(target, " ")
	if !ok || method == "" || strings.ToUpper(method) != method || strings.ContainsAny(method, "/{") {
		return "", target
	}
	return method, strings.TrimSpace(pattern)
}

// repoPath reports why p is not a clean relative path inside the repository.
func repoPath(p string) string {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, `\`) ||
		path.Clean(p) != p || p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "the path must be a clean relative path inside the repository"
	}
	return ""
}

func (c *checker) resolveCode(target string) string {
	file, symbol, hasSymbol := strings.Cut(target, "#")
	if why := repoPath(file); why != "" {
		return why
	}
	info, err := os.Stat(c.path(file))
	if err != nil {
		return file + " does not exist"
	}
	if !hasSymbol {
		return ""
	}
	if info.IsDir() || !strings.HasSuffix(file, ".go") {
		return "#Name and #Type.Method name declarations in a Go file"
	}
	decls, err := c.goDecls(file)
	if err != nil {
		return err.Error()
	}
	if !decls[symbol] {
		return file + " declares no " + symbol
	}
	return ""
}

func (c *checker) resolveDoc(target string) string {
	file, anchor, hasAnchor := strings.Cut(target, "#")
	if why := repoPath(file); why != "" {
		return why
	}
	info, err := os.Stat(c.path(file))
	if err != nil {
		return file + " does not exist"
	}
	if info.IsDir() {
		return file + " is not a file"
	}
	if !hasAnchor {
		return ""
	}
	if !c.headings(file)[anchor] {
		return file + " has no heading #" + anchor
	}
	return ""
}

// goDecls returns the names a Go file declares: top-level functions, types,
// variables, and constants as Name; methods, struct fields, and interface
// methods as Type.Name.
func (c *checker) goDecls(file string) (map[string]bool, error) {
	if decls, ok := c.declCache[file]; ok {
		return decls, nil
	}
	src, err := os.ReadFile(c.path(file))
	if err != nil {
		return nil, err
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("%s does not parse: %v", file, err)
	}
	decls := map[string]bool{}
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil || len(d.Recv.List) == 0 {
				decls[d.Name.Name] = true
			} else {
				decls[receiverName(d.Recv.List[0].Type)+"."+d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					decls[s.Name.Name] = true
					for _, member := range typeMembers(s.Type) {
						decls[s.Name.Name+"."+member] = true
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						decls[name.Name] = true
					}
				}
			}
		}
	}
	if c.declCache == nil {
		c.declCache = map[string]map[string]bool{}
	}
	c.declCache[file] = decls
	return decls, nil
}

func receiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return receiverName(e.X)
	case *ast.IndexExpr:
		return receiverName(e.X)
	case *ast.IndexListExpr:
		return receiverName(e.X)
	case *ast.Ident:
		return e.Name
	}
	return ""
}

// typeMembers lists the field names of a struct or the method names of an
// interface; an embedded member is named by its type.
func typeMembers(expr ast.Expr) []string {
	var list *ast.FieldList
	switch t := expr.(type) {
	case *ast.StructType:
		list = t.Fields
	case *ast.InterfaceType:
		list = t.Methods
	default:
		return nil
	}
	var names []string
	for _, field := range list.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
		if len(field.Names) == 0 {
			if name := receiverName(field.Type); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

var (
	atxHeading      = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	setextUnderline = regexp.MustCompile(`^ {0,3}(?:=+|-+)[ \t]*$`)
	listItem        = regexp.MustCompile(`^ {0,3}(?:[-*+]|[0-9]{1,9}[.)])(?:[ \t]|$)`)
	mdImage         = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	mdLink          = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	htmlTag         = regexp.MustCompile(`<[^>]+>`)
)

// codeFence is an open fenced code block: its character and length.
type codeFence struct {
	char byte
	size int
}

// fenceLine reads a line as a code fence: a run of three or more backticks
// or tildes and the info string after it. A backtick fence's info string
// holds no backtick.
func fenceLine(line string) (fence codeFence, info string, ok bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return codeFence{}, "", false
	}
	size := len(trimmed) - len(strings.TrimLeft(trimmed, trimmed[:1]))
	info = strings.TrimSpace(trimmed[size:])
	if size < 3 || (trimmed[0] == '`' && strings.Contains(info, "`")) {
		return codeFence{}, "", false
	}
	return codeFence{char: trimmed[0], size: size}, info, true
}

// headings returns the GitHub anchors of a Markdown file's ATX and setext
// headings, ignoring fenced code blocks; a repeated heading gets -1, -2, and
// so on. A fence closes only on a run of its own character at least as long
// as the one that opened it. A setext heading is a paragraph followed by a
// line of = or -; a list item, table row, quote, or indented code above such
// a line is not a paragraph, so the line is a thematic break.
func (c *checker) headings(file string) map[string]bool {
	if anchors, ok := c.headingCache[file]; ok {
		return anchors
	}
	anchors := map[string]bool{}
	counts := map[string]int{}
	addHeading := func(text string) {
		slug := githubSlug(text)
		if n := counts[slug]; n > 0 {
			anchors[fmt.Sprintf("%s-%d", slug, n)] = true
		} else {
			anchors[slug] = true
		}
		counts[slug]++
	}
	data, _ := os.ReadFile(c.path(file))
	var open codeFence
	var paragraph []string // the open paragraph's lines, if it can become a heading
	inBlock := false       // inside a list, table, quote, or indented code until a blank line
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	for _, line := range skipFrontMatter(lines) {
		if open.size > 0 {
			if f, info, ok := fenceLine(line); ok && info == "" && f.char == open.char && f.size >= open.size {
				open = codeFence{}
			}
			continue
		}
		if f, _, ok := fenceLine(line); ok {
			open, paragraph = f, nil
			continue
		}
		if m := atxHeading.FindStringSubmatch(line); m != nil {
			addHeading(m[1])
			paragraph = nil
			continue
		}
		if setextUnderline.MatchString(line) && len(paragraph) > 0 {
			addHeading(strings.Join(paragraph, "\n"))
			paragraph = nil
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			paragraph, inBlock = nil, false
		case thematicBreak(trimmed):
			paragraph = nil
		case inBlock:
		case listItem.MatchString(line) || strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, ">") ||
			(len(paragraph) == 0 && (strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t"))):
			paragraph, inBlock = nil, true
		default:
			paragraph = append(paragraph, trimmed)
		}
	}
	if c.headingCache == nil {
		c.headingCache = map[string]map[string]bool{}
	}
	c.headingCache[file] = anchors
	return anchors
}

// skipFrontMatter drops a leading YAML front matter block, which GitHub
// renders as a table rather than as headings.
func skipFrontMatter(lines []string) []string {
	if len(lines) == 0 || lines[0] != "---" {
		return lines
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" || lines[i] == "..." {
			return lines[i+1:]
		}
	}
	return lines
}

// thematicBreak reports whether a trimmed line is three or more of the same
// -, *, or _ with optional spaces between them.
func thematicBreak(trimmed string) bool {
	run := strings.ReplaceAll(strings.ReplaceAll(trimmed, " ", ""), "\t", "")
	return len(run) >= 3 && strings.Trim(run, run[:1]) == "" && strings.ContainsAny(run[:1], "-*_")
}

// githubSlug turns heading text into its GitHub anchor: the rendered text,
// lower-cased, keeping letters, digits, hyphens, and underscores, with each
// space becoming a hyphen.
func githubSlug(text string) string {
	text = mdImage.ReplaceAllString(text, "$1")
	text = mdLink.ReplaceAllString(text, "$1")
	text = htmlTag.ReplaceAllString(text, "")
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsMark(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}
