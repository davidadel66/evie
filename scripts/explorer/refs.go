package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// refFixes are the reference kinds a map, a part, a sequence or a
// changelog source may use, as "<kind>:<target>", with how to fix each.
var refFixes = map[string]string{
	"code":    "point it at an existing repository file or folder, with #Name or #Type.Method naming a declaration in that Go, JavaScript or TypeScript file",
	"doc":     "point it at an existing repository file or folder, with #anchor naming one of a Markdown file's headings (GitHub slug)",
	"adr":     "use the four-digit number of an ADR in docs/adr",
	"term":    "use a term exactly as GLOSSARY.md spells it",
	"table":   "use database.table from the facts databases",
	"route":   "use a route from the facts as /pattern or METHOD /pattern",
	"command": "use a binary and word path from the facts commands",
	"pkg":     "use a package path from the facts",
	"part":    "use the id of a part of the map",
}

// refKinds lists the reference kinds for a fix hint.
func refKinds() string {
	return strings.Join(slices.Sorted(maps.Keys(refFixes)), ":, ") + ":"
}

// resolve returns "" when ref, "<kind>:<target>", resolves, or why it does
// not. A part: reference names one of parts, the map's part ids.
func (c *checker) resolve(ref string, parts map[string]bool) string {
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
		method, pattern, ok := splitRoute(target)
		if !ok {
			return "want route:/pattern or route:METHOD /pattern, with an upper-case method and one space"
		}
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
	case "part":
		if parts[target] {
			return ""
		}
		return "there is no part " + target
	}
	return "unknown reference kind " + kind
}

// routeRef is a route reference's one form, as the vision skill reads it: a
// pattern starting with "/", optionally after an upper-case method and one
// space.
var routeRef = regexp.MustCompile(`^(?:([A-Z]+) )?(/.*)$`)

// splitRoute splits "GET /x" into its method and pattern; "/x" has none.
// Anything else is not a route reference.
func splitRoute(target string) (method, pattern string, ok bool) {
	m := routeRef.FindStringSubmatch(target)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

const notInRepo = "the path must be a clean relative path inside the repository"

// cleanRelative reports whether p is a clean slash path that stays inside
// the directory it is relative to.
func cleanRelative(p string) bool {
	return p != "" && p != "." && p != ".." && !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "../") &&
		!strings.Contains(p, `\`) && path.Clean(p) == p
}

// stat returns a repository path's file information, or why it names
// nothing.
func (c *checker) stat(file string) (os.FileInfo, string) {
	if !cleanRelative(file) {
		return nil, notInRepo
	}
	info, err := os.Stat(c.path(file))
	if err != nil {
		return nil, file + " does not exist"
	}
	return info, ""
}

// resolveCode resolves code:path, a file or folder, and code:path#Name,
// a declaration in that file.
func (c *checker) resolveCode(target string) string {
	file, symbol, hasSymbol := strings.Cut(target, "#")
	info, why := c.stat(file)
	if why != "" || !hasSymbol {
		return why
	}
	if info.IsDir() {
		return file + " is a folder, so it declares nothing"
	}
	_, why = c.declaration(file, symbol)
	return why
}

// declaration returns the whole lines that declare symbol in file, or why
// there are none: Go files by their declarations (Name, or Type.Name for a
// method, field or interface method), and JavaScript and TypeScript files
// by a declaring line (Name, or Owner.member for a member declared
// directly in Owner's body).
func (c *checker) declaration(file, symbol string) (string, string) {
	var src string
	var found bool
	switch ext := path.Ext(file); {
	case ext == ".go":
		decls, err := c.goDecls(file)
		if err != nil {
			return "", err.Error()
		}
		at, ok := decls.spans[symbol]
		src, found = string(decls.src[at.from:at.to]), ok
	case scriptExts[ext]:
		decls, err := c.scriptDecls(file)
		if err != nil {
			return "", err.Error()
		}
		src, found = decls.find(symbol)
	default:
		return "", "#Name names a declaration in a Go, JavaScript or TypeScript file, not a " + ext + " file"
	}
	if !found {
		return "", file + " declares no " + symbol
	}
	return src, ""
}

// resolveDoc resolves doc:path, a file or folder, and doc:path#anchor, a
// heading of that Markdown file.
func (c *checker) resolveDoc(target string) string {
	file, anchor, hasAnchor := strings.Cut(target, "#")
	info, why := c.stat(file)
	switch {
	case why != "":
		return why
	case info.IsDir() && hasAnchor:
		return file + " is not a file"
	case !hasAnchor:
		return ""
	case !c.headings(file)[anchor]:
		return file + " has no heading #" + anchor
	}
	return ""
}

// fileText returns a repository file's whole text, read once, or false
// when it cannot be read.
func (c *checker) fileText(file string) (string, bool) {
	if text, ok := c.textCache[file]; ok {
		if text == nil {
			return "", false
		}
		return *text, true
	}
	if c.textCache == nil {
		c.textCache = map[string]*string{}
	}
	data, err := os.ReadFile(c.path(file))
	if err != nil {
		c.textCache[file] = nil
		return "", false
	}
	text := string(data)
	c.textCache[file] = &text
	return text, true
}

// goFile is a Go file's text and the whole lines of each name it declares.
type goFile struct {
	src   []byte
	spans map[string]span
}

// span is a run of whole lines, as byte offsets into its file.
type span struct{ from, to int }

// goDecls returns what a Go file declares: top-level functions, types,
// variables, and constants as Name; methods, struct fields, and interface
// methods as Type.Name. Each name keeps the lines of its first declaration.
func (c *checker) goDecls(file string) (*goFile, error) {
	if decls, ok := c.declCache[file]; ok {
		return decls, nil
	}
	src, err := os.ReadFile(c.path(file))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("%s does not parse: %v", file, err)
	}
	decls := &goFile{src: src, spans: map[string]span{}}
	declare := func(name string, node ast.Node) {
		if _, ok := decls.spans[name]; !ok {
			decls.spans[name] = lineSpan(src, fset.Position(node.Pos()).Offset, fset.Position(node.End()).Offset)
		}
	}
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			declare(funcName(d), d)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					declare(s.Name.Name, s)
					for _, member := range typeMembers(s.Type) {
						declare(s.Name.Name+"."+member.name, member.node)
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						declare(name.Name, s)
					}
				}
			}
		}
	}
	if c.declCache == nil {
		c.declCache = map[string]*goFile{}
	}
	c.declCache[file] = decls
	return decls, nil
}

// lineSpan widens the bytes from..to of src to the whole lines they are on.
func lineSpan(src []byte, from, to int) span {
	from = bytes.LastIndexByte(src[:from], '\n') + 1
	if i := bytes.IndexByte(src[to:], '\n'); i >= 0 {
		return span{from, to + i}
	}
	return span{from, len(src)}
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

// typeMember is a struct field or interface method, by name.
type typeMember struct {
	name string
	node ast.Node
}

// typeMembers lists the fields of a struct or the methods of an interface;
// an embedded member is named by its type.
func typeMembers(expr ast.Expr) []typeMember {
	var list *ast.FieldList
	switch t := expr.(type) {
	case *ast.StructType:
		list = t.Fields
	case *ast.InterfaceType:
		list = t.Methods
	default:
		return nil
	}
	var members []typeMember
	for _, field := range list.List {
		for _, name := range field.Names {
			members = append(members, typeMember{name.Name, field})
		}
		if len(field.Names) == 0 {
			if name := receiverName(field.Type); name != "" {
				members = append(members, typeMember{name, field})
			}
		}
	}
	return members
}

// scriptExts are the JavaScript and TypeScript files code: references may
// name declarations in.
var scriptExts = map[string]bool{".js": true, ".jsx": true, ".mjs": true, ".ts": true, ".tsx": true}

var (
	// scriptDecl is a line declaring a function, class, interface, type,
	// enum, namespace or variable, and the name it declares.
	scriptDecl = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?(?:async\s+)?` +
		`(?:function\s*\*?\s*|(?:class|interface|type|(?:const\s+)?enum|const|let|var|namespace)\s+)([A-Za-z_$][\w$]*)`)
	// scriptMember is a line in a body declaring a member (a method,
	// property, accessor or enum member), and the member's name.
	scriptMember = regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|async|override|readonly|abstract|declare|get|set)\s+)*` +
		`\*?\s*(#?[A-Za-z_$][\w$]*)\s*[?!]?\s*(?:[(:=<;,]|$)`)
	// scriptNoise is a string literal or a line comment, whose braces do
	// not count.
	scriptNoise = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|` + "`(?:[^`\\\\]|\\\\.)*`" + `|//.*$`)
)

// scriptFile is a JavaScript or TypeScript file's lines, and the line that
// first declares each name.
type scriptFile struct {
	lines []string
	decls map[string]int
}

func (c *checker) scriptDecls(file string) (*scriptFile, error) {
	if decls, ok := c.scriptCache[file]; ok {
		return decls, nil
	}
	data, err := os.ReadFile(c.path(file))
	if err != nil {
		return nil, err
	}
	decls := &scriptFile{lines: strings.Split(string(data), "\n"), decls: map[string]int{}}
	for i, line := range decls.lines {
		if m := scriptDecl.FindStringSubmatch(line); m != nil {
			if _, ok := decls.decls[m[1]]; !ok {
				decls.decls[m[1]] = i
			}
		}
	}
	if c.scriptCache == nil {
		c.scriptCache = map[string]*scriptFile{}
	}
	c.scriptCache[file] = decls
	return decls, nil
}

// find returns the lines that declare symbol, from its declaring line
// through its matching brace: Name, or Owner.member for a member declared
// on a line directly inside Owner's braces.
func (f *scriptFile) find(symbol string) (string, bool) {
	owner, member, dotted := strings.Cut(symbol, ".")
	at, ok := f.decls[owner]
	if !ok {
		return "", false
	}
	end := f.close(at)
	if !dotted {
		return strings.Join(f.lines[at:end+1], "\n"), true
	}
	depth := 0
	for i := at; i <= end; i++ {
		if i > at && depth == 1 {
			if m := scriptMember.FindStringSubmatch(f.lines[i]); m != nil && m[1] == member {
				return strings.Join(f.lines[i:f.close(i)+1], "\n"), true
			}
		}
		code := scriptNoise.ReplaceAllString(f.lines[i], "")
		depth += strings.Count(code, "{") - strings.Count(code, "}")
	}
	return "", false
}

// close returns the last line of the declaration on line i: the line of its
// matching brace, or the end of a statement without one (the skill's
// brace_span).
func (f *scriptFile) close(i int) int {
	depth, seen := 0, false
	for j := i; j < len(f.lines); j++ {
		code := scriptNoise.ReplaceAllString(f.lines[j], "")
		depth += strings.Count(code, "{") - strings.Count(code, "}")
		seen = seen || strings.Contains(code, "{")
		end := strings.TrimRightFunc(code, unicode.IsSpace)
		switch trimmed := strings.TrimSpace(code); {
		case seen && depth <= 0:
			return j
		case j == i && !seen && !strings.Contains(code, "(") && !strings.HasSuffix(end, "=") && !strings.HasSuffix(end, ",") && !strings.HasSuffix(end, "+"):
			return i
		case !seen && j > i && (strings.HasSuffix(end, ";") || trimmed == "" || trimmed == ")"):
			return j
		}
	}
	return len(f.lines) - 1
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
