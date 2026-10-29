package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Commands are read from argument comparisons (system-explorer.spec.md,
// Extraction rules): a string literal compared by ==, != or a switch case
// with an element of the command's argument slice is the word at that
// element's position, counted from the first argument after the binary
// name. The words at earlier positions come from the conditions the
// comparison sits under: enclosing if and switch branches, the left side of
// && and ||, and the rest of a block after an if that returns. The walk
// starts at main and follows calls into the package's own functions that
// receive the arguments, a slice of them, or one of them.
//
// Literals that begin with "-" are flags and the empty string is no word;
// both are skipped. Any other comparison whose position, or whose earlier
// words, cannot be known fails extraction.

// packageCommands extracts the word paths of a command package.
func packageCommands(pkg packageInfo) ([]Command, error) {
	if !pkg.Command {
		return nil, nil
	}
	r := &commandReader{
		pkg:     pkg,
		binary:  path.Base(pkg.Path),
		funcs:   map[string][]funcInfo{},
		methods: map[string][]funcInfo{},
		found:   map[string]foundCommand{},
		active:  map[*ast.FuncDecl]bool{},
	}
	for _, file := range pkg.files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			info := funcInfo{decl: fn, file: file, name: pkg.fset.Position(file.Package).Filename}
			if fn.Recv == nil {
				r.funcs[fn.Name.Name] = append(r.funcs[fn.Name.Name], info)
			} else {
				r.methods[fn.Name.Name] = append(r.methods[fn.Name.Name], info)
			}
		}
	}
	for _, main := range r.funcs["main"] {
		r.walkFunc(main, cmdState{env: argEnv{}, ctx: wordSet{}}, 0)
		if r.err != nil {
			return nil, r.err
		}
	}
	commands := make([]Command, 0, len(r.found))
	for _, f := range r.found {
		commands = append(commands, f.command)
	}
	return commands, nil
}

// sortCommands orders commands by binary, then word path.
func sortCommands(commands []Command) {
	sort.Slice(commands, func(i, j int) bool {
		a, b := commands[i], commands[j]
		if a.Binary != b.Binary {
			return a.Binary < b.Binary
		}
		for k := 0; k < len(a.Words) && k < len(b.Words); k++ {
			if a.Words[k] != b.Words[k] {
				return a.Words[k] < b.Words[k]
			}
		}
		return len(a.Words) < len(b.Words)
	})
}

type funcInfo struct {
	decl *ast.FuncDecl
	file *ast.File
	name string // the file's slash path relative to the repository root
}

// argValue is how an expression relates to the command's arguments: a slice
// of them whose first element is at pos, or the one argument at pos.
type argValue struct {
	slice bool
	pos   int
	known bool // whether pos is known
}

// argEnv maps local names to the arguments they hold.
type argEnv map[string]argValue

// wordSet holds the words known at each position: the literals the argument
// there must equal for the code to be reached. An empty set at a position
// means the code is unreachable.
type wordSet map[int][]string

type cmdState struct {
	env argEnv
	ctx wordSet
}

// foundCommand is a word path and the call depth from main of the
// comparison that placed it; the shallowest, first found, gives its source.
type foundCommand struct {
	command Command
	depth   int
}

type commandReader struct {
	pkg     packageInfo
	binary  string
	funcs   map[string][]funcInfo // plain functions by name
	methods map[string][]funcInfo // methods by name
	found   map[string]foundCommand
	active  map[*ast.FuncDecl]bool
	err     error
}

// frame is the function being walked.
type frame struct {
	fn    funcInfo
	depth int
	os    string // the file's name for package os
}

func (r *commandReader) walkFunc(fn funcInfo, st cmdState, depth int) {
	if r.active[fn.decl] || r.err != nil {
		return
	}
	r.active[fn.decl] = true
	defer delete(r.active, fn.decl)
	f := frame{fn: fn, depth: depth, os: importName(fn.file, "os")}
	r.walkBlock(f, fn.decl.Body.List, st)
}

// walkBlock walks statements in order and reports whether the block ends
// without reaching its end. Names the block declares go out of scope at its
// end, so outer names they shadow get their values back.
func (r *commandReader) walkBlock(f frame, stmts []ast.Stmt, st cmdState) (cmdState, bool) {
	outer := st.env
	var declared []string
	done := false
	for _, stmt := range stmts {
		declared = append(declared, declaredNames(stmt)...)
		st, done = r.walkStmt(f, stmt, st)
		if done {
			break
		}
	}
	return st.restore(outer, declared), done
}

func (r *commandReader) walkStmt(f frame, stmt ast.Stmt, st cmdState) (cmdState, bool) {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		r.walkExpr(f, s.X, st)
		return st, isExit(f, s.X)
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			r.walkExpr(f, rhs, st)
		}
		return st.assign(r, f, s.Lhs, s.Rhs), false
	case *ast.DeclStmt:
		if gen, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, v := range vs.Values {
						r.walkExpr(f, v, st)
					}
					lhs := make([]ast.Expr, len(vs.Names))
					for i, name := range vs.Names {
						lhs[i] = name
					}
					st = st.assign(r, f, lhs, vs.Values)
				}
			}
		}
	case *ast.ReturnStmt:
		for _, result := range s.Results {
			r.walkExpr(f, result, st)
		}
		return st, true
	case *ast.BranchStmt:
		// continue and goto leave the rest of the block; break and
		// fallthrough carry on after the enclosing statement.
		return st, s.Tok == token.CONTINUE || s.Tok == token.GOTO
	case *ast.BlockStmt:
		return r.walkBlock(f, s.List, st)
	case *ast.LabeledStmt:
		return r.walkStmt(f, s.Stmt, st)
	case *ast.IfStmt:
		return r.walkIf(f, s, st)
	case *ast.SwitchStmt:
		return r.walkSwitch(f, s, st)
	case *ast.ForStmt:
		outer := st.env
		if s.Init != nil {
			st, _ = r.walkStmt(f, s.Init, st)
		}
		r.walkExpr(f, s.Cond, st)
		body, _ := r.walkBlock(f, s.Body.List, st)
		if s.Post != nil {
			body, _ = r.walkStmt(f, s.Post, body)
		}
		return joinStates(st, body).restore(outer, declaredNames(s.Init)), false
	case *ast.RangeStmt:
		r.walkExpr(f, s.X, st)
		return r.walkRange(f, s, st), false
	case *ast.TypeSwitchStmt:
		if s.Init != nil {
			st, _ = r.walkStmt(f, s.Init, st)
		}
		for _, clause := range s.Body.List {
			r.walkBlock(f, clause.(*ast.CaseClause).Body, st)
		}
	case *ast.SelectStmt:
		for _, clause := range s.Body.List {
			r.walkBlock(f, clause.(*ast.CommClause).Body, st)
		}
	case *ast.GoStmt:
		r.walkExpr(f, s.Call, st)
	case *ast.DeferStmt:
		r.walkExpr(f, s.Call, st)
	case *ast.IncDecStmt:
		r.walkExpr(f, s.X, st)
	case *ast.SendStmt:
		r.walkExpr(f, s.Value, st)
	}
	return st, false
}

// walkRange walks a range body, where a range over the arguments gives each
// element at an unknown position. The loop may not run, so code after it
// is reached both ways.
func (r *commandReader) walkRange(f frame, s *ast.RangeStmt, st cmdState) cmdState {
	outer := st.env
	var declared []string
	inner := cmdState{env: argEnv{}, ctx: st.ctx}
	for k, v := range st.env {
		inner.env[k] = v
	}
	over := r.eval(f, s.X, st.env)
	for i, e := range []ast.Expr{s.Key, s.Value} {
		id, ok := e.(*ast.Ident)
		if !ok || id.Name == "_" {
			continue
		}
		if s.Tok == token.DEFINE {
			declared = append(declared, id.Name)
		}
		if i == 1 && over != nil && over.slice {
			inner.env[id.Name] = argValue{}
		} else {
			delete(inner.env, id.Name)
		}
	}
	body, _ := r.walkBlock(f, s.Body.List, inner)
	return joinStates(st, body).restore(outer, declared)
}

// walkIf walks both branches under what the condition implies; code after
// the if is reached through the branches that do not return.
func (r *commandReader) walkIf(f frame, s *ast.IfStmt, st cmdState) (cmdState, bool) {
	outer := st.env
	if s.Init != nil {
		st, _ = r.walkStmt(f, s.Init, st)
	}
	r.walkExpr(f, s.Cond, st)
	thenSt, thenDone := r.walkBlock(f, s.Body.List, st.with(r.implies(f, s.Cond, true, st.env)))
	elseSt, elseDone := st.with(r.implies(f, s.Cond, false, st.env)), false
	if s.Else != nil {
		elseSt, elseDone = r.walkStmt(f, s.Else, elseSt)
	}
	declared := declaredNames(s.Init)
	switch {
	case thenDone && elseDone:
		return st, true
	case thenDone:
		return elseSt.restore(outer, declared), false
	case elseDone:
		return thenSt.restore(outer, declared), false
	}
	return joinStates(thenSt, elseSt).restore(outer, declared), false
}

// walkSwitch walks each clause under the words its cases imply; code after
// the switch is reached through the clauses that do not return.
func (r *commandReader) walkSwitch(f frame, s *ast.SwitchStmt, st cmdState) (cmdState, bool) {
	outer := st.env
	if s.Init != nil {
		st, _ = r.walkStmt(f, s.Init, st)
	}
	r.walkExpr(f, s.Tag, st)
	tag := r.eval(f, s.Tag, st.env)
	clauses := make([]*ast.CaseClause, 0, len(s.Body.List))
	for _, stmt := range s.Body.List {
		clauses = append(clauses, stmt.(*ast.CaseClause))
	}
	// none holds when every listed case is false, for the default clause.
	none := st.ctx
	if s.Tag == nil {
		for _, clause := range clauses {
			for _, e := range clause.List {
				none = andWords(none, r.implies(f, e, false, st.env))
			}
		}
	}
	var ends []cmdState
	hasDefault, allDone := false, true
	prior := st.ctx
	for _, clause := range clauses {
		var body wordSet
		switch {
		case clause.List == nil:
			hasDefault, body = true, none
		case s.Tag == nil:
			holds := wordSet(nil)
			ctx := prior
			for i, e := range clause.List {
				r.walkExpr(f, e, st.with(ctx))
				if i == 0 {
					holds = r.implies(f, e, true, st.env)
				} else {
					holds = orWords(holds, r.implies(f, e, true, st.env))
				}
				ctx = andWords(ctx, r.implies(f, e, false, st.env))
			}
			body = andWords(prior, holds)
			prior = ctx
		default:
			body = andWords(st.ctx, r.caseWords(f, tag, clause, st))
		}
		end, done := r.walkBlock(f, clause.Body, st.withCtx(body))
		if !done {
			ends = append(ends, end)
			allDone = false
		}
	}
	if !hasDefault {
		ends = append(ends, st.withCtx(none))
		allDone = false
	}
	declared := declaredNames(s.Init)
	if allDone {
		return st, true
	}
	out := ends[0]
	for _, end := range ends[1:] {
		out = joinStates(out, end)
	}
	return out.restore(outer, declared), false
}

// caseWords records the comparisons of a tag switch's case and returns the
// words its body may assume.
func (r *commandReader) caseWords(f frame, tag *argValue, clause *ast.CaseClause, st cmdState) wordSet {
	if tag == nil || tag.slice {
		for _, e := range clause.List {
			r.walkExpr(f, e, st)
		}
		return wordSet{}
	}
	var lits []string
	for _, e := range clause.List {
		lit, ok := stringLit(e)
		if !ok {
			r.walkExpr(f, e, st)
			lits = nil
			continue
		}
		r.compare(f, *tag, lit, st)
		if isWord(lit) {
			lits = append(lits, lit)
		}
	}
	if !tag.known || len(lits) != len(clause.List) {
		return wordSet{}
	}
	return wordSet{tag.pos: sortedUnique(lits)}
}

// walkExpr records the comparisons in an expression and follows its calls.
// The right side of && is walked under what its left side implies when
// true, and of || when false.
func (r *commandReader) walkExpr(f frame, expr ast.Expr, st cmdState) {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		switch e.Op {
		case token.LAND:
			r.walkExpr(f, e.X, st)
			r.walkExpr(f, e.Y, st.with(r.implies(f, e.X, true, st.env)))
			return
		case token.LOR:
			r.walkExpr(f, e.X, st)
			r.walkExpr(f, e.Y, st.with(r.implies(f, e.X, false, st.env)))
			return
		case token.EQL, token.NEQ:
			if word, lit, ok := r.comparison(f, e, st.env); ok {
				r.compare(f, word, lit, st)
				return
			}
		}
		r.walkExpr(f, e.X, st)
		r.walkExpr(f, e.Y, st)
	case *ast.ParenExpr:
		r.walkExpr(f, e.X, st)
	case *ast.UnaryExpr:
		r.walkExpr(f, e.X, st)
	case *ast.CallExpr:
		r.walkExpr(f, e.Fun, st)
		for _, arg := range e.Args {
			r.walkExpr(f, arg, st)
		}
		r.call(f, e, st)
	case *ast.FuncLit:
		// A closure sees the names around it, less its own parameters.
		inner := cmdState{env: argEnv{}, ctx: st.ctx}
		for k, v := range st.env {
			inner.env[k] = v
		}
		for _, field := range e.Type.Params.List {
			for _, name := range field.Names {
				delete(inner.env, name.Name)
			}
		}
		r.walkBlock(f, e.Body.List, inner)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			r.walkExpr(f, elt, st)
		}
	case *ast.KeyValueExpr:
		r.walkExpr(f, e.Key, st)
		r.walkExpr(f, e.Value, st)
	case *ast.SelectorExpr:
		r.walkExpr(f, e.X, st)
	case *ast.IndexExpr:
		r.walkExpr(f, e.X, st)
		r.walkExpr(f, e.Index, st)
	case *ast.SliceExpr:
		r.walkExpr(f, e.X, st)
	case *ast.StarExpr:
		r.walkExpr(f, e.X, st)
	case *ast.TypeAssertExpr:
		r.walkExpr(f, e.X, st)
	}
}

// implies returns the words that hold when cond has the given truth.
func (r *commandReader) implies(f frame, cond ast.Expr, truth bool, env argEnv) wordSet {
	switch e := cond.(type) {
	case *ast.ParenExpr:
		return r.implies(f, e.X, truth, env)
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return r.implies(f, e.X, !truth, env)
		}
	case *ast.BinaryExpr:
		x := func(t bool) wordSet { return r.implies(f, e.X, t, env) }
		y := func(t bool) wordSet { return r.implies(f, e.Y, t, env) }
		switch {
		case e.Op == token.LAND && truth, e.Op == token.LOR && !truth:
			return andWords(x(truth), y(truth))
		case e.Op == token.LAND, e.Op == token.LOR:
			return orWords(x(truth), y(truth))
		case e.Op == token.EQL || e.Op == token.NEQ:
			word, lit, ok := r.comparison(f, e, env)
			if ok && word.known && isWord(lit) && (e.Op == token.EQL) == truth {
				return wordSet{word.pos: {lit}}
			}
		}
	}
	return wordSet{}
}

// comparison recognises an argument compared with a string literal.
func (r *commandReader) comparison(f frame, e *ast.BinaryExpr, env argEnv) (argValue, string, bool) {
	for _, pair := range [][2]ast.Expr{{e.X, e.Y}, {e.Y, e.X}} {
		lit, ok := stringLit(pair[1])
		if !ok {
			continue
		}
		if v := r.eval(f, pair[0], env); v != nil && !v.slice {
			return *v, lit, true
		}
	}
	return argValue{}, "", false
}

// compare records the word paths a comparison places: the words known at
// every earlier position, then the literal. It fails extraction when the
// position or an earlier word is unknown.
func (r *commandReader) compare(f frame, word argValue, lit string, st cmdState) {
	if r.err != nil || !isWord(lit) || unreachable(st.ctx) {
		return
	}
	fail := func(why string) {
		r.err = fmt.Errorf("%s %s: cannot place the comparison with %q: %s", f.fn.name, funcName(f.fn.decl), lit, why)
	}
	if !word.known {
		fail("the argument's position is not known")
		return
	}
	if word.pos < 0 {
		fail("it compares the binary name")
		return
	}
	paths := [][]string{{}}
	for q := 0; q < word.pos; q++ {
		known := st.ctx[q]
		if len(known) == 0 {
			fail(fmt.Sprintf("no word is known at position %d before it", q))
			return
		}
		var next [][]string
		for _, p := range paths {
			for _, w := range known {
				next = append(next, append(append([]string{}, p...), w))
			}
		}
		paths = next
	}
	for _, p := range paths {
		words := append(p, lit)
		key := r.binary + "\x00" + strings.Join(words, "\x00")
		found := foundCommand{
			command: Command{Binary: r.binary, Words: words, Package: r.pkg.Path, Source: Source{File: f.fn.name, Function: funcName(f.fn.decl)}},
			depth:   f.depth,
		}
		if prev, ok := r.found[key]; !ok || found.depth < prev.depth {
			r.found[key] = found
		}
	}
}

// call follows a call into one of the package's functions when it passes
// the arguments on.
func (r *commandReader) call(f frame, call *ast.CallExpr, st cmdState) {
	var callees []funcInfo
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		callees = r.funcs[fun.Name]
	case *ast.SelectorExpr:
		// Without type information a method is found by name; a call on an
		// imported package leaves the command package.
		if x, ok := fun.X.(*ast.Ident); !ok || !isImportName(f.fn.file, x.Name) {
			callees = r.methods[fun.Sel.Name]
		}
	}
	args := make([]*argValue, len(call.Args))
	passes := false
	for i, arg := range call.Args {
		args[i] = r.eval(f, arg, st.env)
		passes = passes || args[i] != nil
	}
	for _, callee := range callees {
		if !passes && !readsOSArgs(callee) {
			continue
		}
		env := argEnv{}
		i := 0
		for _, field := range callee.decl.Type.Params.List {
			for _, name := range field.Names {
				if i < len(args) && args[i] != nil {
					env[name.Name] = *args[i]
				}
				i++
			}
		}
		r.walkFunc(callee, cmdState{env: env, ctx: st.ctx}, f.depth+1)
	}
}

// eval returns how expr relates to the arguments, or nil.
func (r *commandReader) eval(f frame, expr ast.Expr, env argEnv) *argValue {
	switch e := expr.(type) {
	case *ast.Ident:
		if v, ok := env[e.Name]; ok {
			return &v
		}
	case *ast.ParenExpr:
		return r.eval(f, e.X, env)
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok && f.os != "" && x.Name == f.os && e.Sel.Name == "Args" {
			return &argValue{slice: true, pos: -1, known: true}
		}
	case *ast.SliceExpr:
		base := r.eval(f, e.X, env)
		if base == nil || !base.slice {
			return nil
		}
		if e.Low == nil {
			return base
		}
		if k, ok := intLit(e.Low); ok {
			return &argValue{slice: true, pos: base.pos + k, known: base.known}
		}
		return &argValue{slice: true}
	case *ast.IndexExpr:
		base := r.eval(f, e.X, env)
		if base == nil || !base.slice {
			return nil
		}
		if k, ok := intLit(e.Index); ok {
			return &argValue{pos: base.pos + k, known: base.known}
		}
		return &argValue{}
	}
	return nil
}

// isImportName reports whether name is a package that file imports.
func isImportName(file *ast.File, name string) bool {
	for _, spec := range file.Imports {
		p := strings.Trim(spec.Path.Value, `"`)
		if importName(file, p) == name {
			return true
		}
	}
	return false
}

// readsOSArgs reports whether a function reads os.Args itself.
func readsOSArgs(fn funcInfo) bool {
	osName := importName(fn.file, "os")
	if osName == "" {
		return false
	}
	reads := false
	ast.Inspect(fn.decl.Body, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok && isSelector(e, osName, "Args") {
			reads = true
		}
		return !reads
	})
	return reads
}

// intLit returns the value of an integer literal.
func intLit(expr ast.Expr) (int, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	n, err := strconv.Atoi(lit.Value)
	return n, err == nil
}

// isWord reports whether a compared literal can be a word: not empty and
// not a flag.
func isWord(lit string) bool {
	return lit != "" && !strings.HasPrefix(lit, "-")
}

// isExit reports whether a call ends the program: panic, os.Exit, or a log
// Fatal or Panic function.
func isExit(f frame, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == "panic"
	case *ast.SelectorExpr:
		x, ok := fun.X.(*ast.Ident)
		if !ok {
			return false
		}
		if x.Name == f.os && f.os != "" {
			return fun.Sel.Name == "Exit"
		}
		if log := importName(f.fn.file, "log"); log != "" && x.Name == log {
			return strings.HasPrefix(fun.Sel.Name, "Fatal") || strings.HasPrefix(fun.Sel.Name, "Panic")
		}
	}
	return false
}

// declaredNames lists the names a statement declares in its block.
func declaredNames(stmt ast.Stmt) []string {
	var names []string
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			for _, lhs := range s.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					names = append(names, id.Name)
				}
			}
		}
	case *ast.DeclStmt:
		if gen, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range vs.Names {
						names = append(names, name.Name)
					}
				}
			}
		}
	}
	return names
}

// with narrows the state's words by what also holds.
func (st cmdState) with(holds wordSet) cmdState {
	return cmdState{env: st.env, ctx: andWords(st.ctx, holds)}
}

// withCtx replaces the state's words.
func (st cmdState) withCtx(ctx wordSet) cmdState {
	return cmdState{env: st.env, ctx: ctx}
}

// assign binds each name on the left to how its value relates to the
// arguments; a name given anything else no longer holds an argument.
func (st cmdState) assign(r *commandReader, f frame, lhs, rhs []ast.Expr) cmdState {
	env := argEnv{}
	for k, v := range st.env {
		env[k] = v
	}
	for i, l := range lhs {
		id, ok := l.(*ast.Ident)
		if !ok || id.Name == "_" {
			continue
		}
		var v *argValue
		if len(lhs) == len(rhs) {
			v = r.eval(f, rhs[i], st.env)
		}
		if v != nil {
			env[id.Name] = *v
		} else {
			delete(env, id.Name)
		}
	}
	return cmdState{env: env, ctx: st.ctx}
}

// restore gives declared names their values from outer.
func (st cmdState) restore(outer argEnv, declared []string) cmdState {
	if len(declared) == 0 {
		return st
	}
	env := argEnv{}
	for k, v := range st.env {
		env[k] = v
	}
	for _, name := range declared {
		if v, ok := outer[name]; ok {
			env[name] = v
		} else {
			delete(env, name)
		}
	}
	return cmdState{env: env, ctx: st.ctx}
}

// joinStates merges two paths through the code: words that hold on both,
// and each name's value, with its position unknown where the paths differ.
func joinStates(a, b cmdState) cmdState {
	env := argEnv{}
	for k, v := range a.env {
		env[k] = v
	}
	for k, v := range b.env {
		if prev, ok := env[k]; ok && prev != v {
			env[k] = argValue{slice: prev.slice}
		} else {
			env[k] = v
		}
	}
	return cmdState{env: env, ctx: orWords(a.ctx, b.ctx)}
}

// andWords returns the words known when both sets hold.
func andWords(a, b wordSet) wordSet {
	out := wordSet{}
	for pos, words := range a {
		out[pos] = words
	}
	for pos, words := range b {
		if prev, ok := out[pos]; ok {
			out[pos] = intersect(prev, words)
		} else {
			out[pos] = words
		}
	}
	return out
}

// orWords returns the words known when either set holds.
func orWords(a, b wordSet) wordSet {
	if unreachable(a) {
		return b
	}
	if unreachable(b) {
		return a
	}
	out := wordSet{}
	for pos, words := range a {
		if other, ok := b[pos]; ok {
			out[pos] = sortedUnique(append(append([]string{}, words...), other...))
		}
	}
	return out
}

// unreachable reports whether no argument list satisfies the words.
func unreachable(w wordSet) bool {
	for _, words := range w {
		if len(words) == 0 {
			return true
		}
	}
	return false
}

func intersect(a, b []string) []string {
	out := []string{}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				out = append(out, x)
			}
		}
	}
	return out
}

func sortedUnique(words []string) []string {
	sort.Strings(words)
	out := words[:0]
	for i, w := range words {
		if i == 0 || w != words[i-1] {
			out = append(out, w)
		}
	}
	return out
}
