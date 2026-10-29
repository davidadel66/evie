package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"
)

// packageRoutes reads every Handle and HandleFunc registration on a ServeMux
// in pkg's non-test files.
func packageRoutes(pkg packageInfo) ([]Route, error) {
	var routes []Route
	var err error
	for _, file := range pkg.files {
		httpName := importName(file, "net/http")
		if httpName == "" {
			continue
		}
		name := pkg.fset.Position(file.Package).Filename
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			r := routeReader{pkg: pkg.Path, file: name, function: funcName(fn), http: httpName, muxes: serveMuxes(fn, httpName), locals: localValues(fn)}
			if fn.Recv != nil && len(fn.Recv.List) > 0 && len(fn.Recv.List[0].Names) > 0 {
				r.receiver = fn.Recv.List[0].Names[0].Name
			}
			var stack []ast.Node
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if n == nil {
					stack = stack[:len(stack)-1]
					return false
				}
				if call, ok := n.(*ast.CallExpr); ok && err == nil {
					var found []Route
					found, err = r.registrations(call, stack)
					routes = append(routes, found...)
				}
				stack = append(stack, n)
				return true
			})
			if err != nil {
				return nil, err
			}
		}
	}
	return routes, nil
}

// routeReader reads the registrations inside one function.
type routeReader struct {
	pkg, file, function string
	http                string                // the file's name for net/http
	receiver            string                // the method's receiver, if any
	muxes               map[string]bool       // local names that hold a ServeMux
	locals              map[string][]ast.Expr // values assigned to each local name
}

// registrations returns the routes a Handle or HandleFunc call registers;
// enclosing holds the nodes around the call, innermost last.
func (r routeReader) registrations(call *ast.CallExpr, enclosing []ast.Node) ([]Route, error) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") || len(call.Args) != 2 {
		return nil, nil
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok || (!r.muxes[recv.Name] && recv.Name != r.http) {
		return nil, nil
	}
	found, ok := patterns(call.Args[0], enclosing)
	if !ok {
		return nil, fmt.Errorf("%s %s: cannot read the route pattern %s: want a string literal, or a literal prefix joined to the key of a range over a map literal with literal keys or to the element of a range over a slice literal of strings",
			r.file, r.function, types.ExprString(call.Args[0]))
	}
	var routes []Route
	guard := r.guard(call.Args[1])
	for _, pattern := range found {
		route := Route{Package: r.pkg, Guard: guard, Source: Source{File: r.file, Function: r.function}}
		route.Method, route.Pattern = splitMethod(pattern)
		routes = append(routes, route)
	}
	return routes, nil
}

// patterns evaluates a route pattern expression: a string literal, or a
// literal prefix joined to the key of an enclosing range over a map literal
// with literal keys, or to the element of a range over a slice literal of
// string literals, which gives one pattern per key or element.
func patterns(expr ast.Expr, enclosing []ast.Node) ([]string, bool) {
	if s, ok := stringLit(expr); ok {
		return []string{s}, true
	}
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != token.ADD {
		return nil, false
	}
	prefix, ok := stringLit(bin.X)
	id, isIdent := bin.Y.(*ast.Ident)
	if !ok || !isIdent {
		return nil, false
	}
	values, ok := rangeValues(id.Name, enclosing)
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, prefix+v)
	}
	return out, ok
}

// rangeValues returns the literal strings name takes in the innermost
// enclosing range that binds it; false when no range binds it or that range
// is not over a literal with literal keys or elements.
func rangeValues(name string, enclosing []ast.Node) ([]string, bool) {
	for i := len(enclosing) - 1; i >= 0; i-- {
		rng, ok := enclosing[i].(*ast.RangeStmt)
		if !ok {
			continue
		}
		key, _ := rng.Key.(*ast.Ident)
		value, _ := rng.Value.(*ast.Ident)
		isKey, isValue := key != nil && key.Name == name, value != nil && value.Name == name
		if !isKey && !isValue {
			continue
		}
		lit, ok := rng.X.(*ast.CompositeLit)
		if !ok {
			return nil, false
		}
		var values []string
		switch lit.Type.(type) {
		case *ast.MapType:
			if !isKey {
				return nil, false
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					return nil, false
				}
				s, ok := stringLit(kv.Key)
				if !ok {
					return nil, false
				}
				values = append(values, s)
			}
		case *ast.ArrayType:
			if !isValue {
				return nil, false
			}
			for _, elt := range lit.Elts {
				s, ok := stringLit(elt)
				if !ok {
					return nil, false
				}
				values = append(values, s)
			}
		default:
			return nil, false
		}
		return values, true
	}
	return nil, false
}

// stringLit returns the value of a string literal.
func stringLit(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// guard names the server method wrapping a handler: a method called on the
// enclosing method's receiver with the handler as an argument. It looks
// through conversions such as http.HandlerFunc, local variables, and a
// closure that delegates to a guarded handler with ServeHTTP.
func (r routeReader) guard(handler ast.Expr) string {
	return r.guardOf(handler, map[string]bool{})
}

func (r routeReader) guardOf(handler ast.Expr, seen map[string]bool) string {
	switch h := handler.(type) {
	case *ast.ParenExpr:
		return r.guardOf(h.X, seen)
	case *ast.Ident:
		if seen[h.Name] {
			return ""
		}
		seen[h.Name] = true
		return r.sameGuard(r.locals[h.Name], seen)
	case *ast.FuncLit:
		var delegates []ast.Expr
		ast.Inspect(h.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "ServeHTTP" {
					delegates = append(delegates, sel.X)
				}
			}
			return true
		})
		return r.sameGuard(delegates, seen)
	case *ast.CallExpr:
		sel, ok := h.Fun.(*ast.SelectorExpr)
		if !ok {
			return ""
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return ""
		}
		switch {
		case x.Name == r.receiver && r.receiver != "" && len(h.Args) > 0:
			return sel.Sel.Name
		case x.Name == r.http && (sel.Sel.Name == "HandlerFunc" || sel.Sel.Name == "Handler") && len(h.Args) == 1:
			return r.guardOf(h.Args[0], seen)
		}
	}
	return ""
}

// sameGuard returns the guard every handler shares, or "" when they differ
// or there are none.
func (r routeReader) sameGuard(handlers []ast.Expr, seen map[string]bool) string {
	guard := ""
	for i, h := range handlers {
		g := r.guardOf(h, seen)
		if g == "" || (i > 0 && g != guard) {
			return ""
		}
		guard = g
	}
	return guard
}

// localValues maps each name assigned alone in fn (name := value, name =
// value, var name = value) to every value it is given.
func localValues(fn *ast.FuncDecl) map[string][]ast.Expr {
	locals := map[string][]ast.Expr{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			if len(s.Lhs) == len(s.Rhs) {
				for i, lhs := range s.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						locals[id.Name] = append(locals[id.Name], s.Rhs[i])
					}
				}
			}
		case *ast.ValueSpec:
			if len(s.Names) == len(s.Values) {
				for i, name := range s.Names {
					locals[name.Name] = append(locals[name.Name], s.Values[i])
				}
			}
		}
		return true
	})
	return locals
}

// splitMethod splits "GET /x" into its method and pattern; "/x" has none.
func splitMethod(pattern string) (string, string) {
	method, rest, ok := strings.Cut(pattern, " ")
	if !ok || method == "" || strings.Trim(method, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return "", pattern
	}
	return method, strings.TrimLeft(rest, " \t")
}

// serveMuxes returns the names that hold a ServeMux in fn: parameters and
// variables of type *http.ServeMux, and variables set from http.NewServeMux.
func serveMuxes(fn *ast.FuncDecl, httpName string) map[string]bool {
	muxes := map[string]bool{}
	isMuxType := func(expr ast.Expr) bool {
		if star, ok := expr.(*ast.StarExpr); ok {
			expr = star.X
		}
		return isSelector(expr, httpName, "ServeMux")
	}
	isNewMux := func(expr ast.Expr) bool {
		call, ok := expr.(*ast.CallExpr)
		return ok && isSelector(call.Fun, httpName, "NewServeMux")
	}
	for _, field := range fn.Type.Params.List {
		if isMuxType(field.Type) {
			for _, name := range field.Names {
				muxes[name.Name] = true
			}
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			if len(s.Lhs) == len(s.Rhs) {
				for i, rhs := range s.Rhs {
					if id, ok := s.Lhs[i].(*ast.Ident); ok && isNewMux(rhs) {
						muxes[id.Name] = true
					}
				}
			}
		case *ast.ValueSpec:
			for i, name := range s.Names {
				if (s.Type != nil && isMuxType(s.Type)) || (i < len(s.Values) && isNewMux(s.Values[i])) {
					muxes[name.Name] = true
				}
			}
		}
		return true
	})
	return muxes
}

func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg && sel.Sel.Name == name
}

// importName returns the name file uses for the import path, or "" when the
// file does not import it by name.
func importName(file *ast.File, importPath string) string {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != importPath {
			continue
		}
		if spec.Name != nil {
			if spec.Name.Name == "_" || spec.Name.Name == "." {
				return ""
			}
			return spec.Name.Name
		}
		return importPath[strings.LastIndex(importPath, "/")+1:]
	}
	return ""
}

// funcName names a declaration as code references do: Name, or Type.Name
// for a method.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// sortRoutes orders routes by pattern, then method, then source.
func sortRoutes(routes []Route) {
	sort.Slice(routes, func(i, j int) bool {
		a, b := routes[i], routes[j]
		if a.Pattern != b.Pattern {
			return a.Pattern < b.Pattern
		}
		if a.Method != b.Method {
			return a.Method < b.Method
		}
		if a.Source.File != b.Source.File {
			return a.Source.File < b.Source.File
		}
		return a.Source.Function < b.Source.Function
	})
}
