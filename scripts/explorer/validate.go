package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// checker collects problems while validating the explorer data against the
// freshly extracted facts.
type checker struct {
	root     string
	facts    Facts
	problems []problem
	nodes    map[string]string // node id -> data file that defines it
	flows    map[string]string // flow id -> data file that defines it
	diagrams map[string]string // diagram id -> data file that defines it
	packages map[string]bool   // package paths from the facts
	imports  map[Import]bool   // import edges from the facts

	declCache    map[string]map[string]bool // Go file -> declared names
	headingCache map[string]map[string]bool // Markdown file -> anchors
}

func (c *checker) add(file, item, message, fix string) {
	c.problems = append(c.problems, problem{File: file, Item: item, Message: message, Fix: fix})
}

func (c *checker) path(rel string) string {
	return filepath.Join(c.root, filepath.FromSlash(rel))
}

// validate checks every decoded data file.
func (c *checker) validate(files []dataFile) {
	c.index(files)
	for _, file := range files {
		if file.Content != nil {
			if file.Content.Owner == "" {
				c.add(file.Path, "content", "malformed data: missing owner", "set owner to the subsystem id")
			}
			for _, n := range file.Content.Nodes {
				c.validateNode(file.Path, n, nodeRules{})
			}
			for _, e := range file.Content.Edges {
				c.validateEdge(file.Path, "", e, scope{})
			}
			for _, f := range file.Content.Flows {
				c.validateFlow(file.Path, "flow "+f.ID, f, false, scope{})
			}
		}
	}
	c.validateProposals(files)
	c.checkPackageCoverage()
}

// index records every node id, including children, and reports duplicates.
func (c *checker) index(files []dataFile) {
	c.nodes = map[string]string{}
	c.flows = map[string]string{}
	c.diagrams = map[string]string{}
	c.packages = map[string]bool{}
	c.imports = map[Import]bool{}
	for _, pkg := range c.facts.Packages {
		c.packages[pkg.Path] = true
	}
	for _, imp := range c.facts.Imports {
		c.imports[imp] = true
	}
	var walk func(file string, nodes []Node)
	walk = func(file string, nodes []Node) {
		for _, n := range nodes {
			if n.ID != "" {
				if first, dup := c.nodes[n.ID]; dup {
					c.add(file, "node "+n.ID, "duplicate node id, also defined in "+first, "give each node a unique id")
				} else {
					c.nodes[n.ID] = file
				}
			}
			for _, d := range n.Diagrams {
				if first, dup := c.diagrams[d.ID]; dup && d.ID != "" {
					c.add(file, "node "+n.ID+" diagram "+d.ID, "duplicate diagram id, also defined in "+first, "give each diagram a unique id")
				} else if d.ID != "" {
					c.diagrams[d.ID] = file
				}
			}
			walk(file, n.Children)
		}
	}
	for _, file := range files {
		if file.Content == nil {
			continue
		}
		walk(file.Path, file.Content.Nodes)
		for _, f := range file.Content.Flows {
			if first, dup := c.flows[f.ID]; dup && f.ID != "" {
				c.add(file.Path, "flow "+f.ID, "duplicate flow id, also defined in "+first, "give each flow a unique id")
			} else if f.ID != "" {
				c.flows[f.ID] = file.Path
			}
		}
	}
}

// checkPackageCoverage reports every package without a component node.
func (c *checker) checkPackageCoverage() {
	for _, pkg := range c.facts.Packages {
		id := packageNodeID(pkg.Path)
		if _, ok := c.nodes[id]; !ok {
			c.add(factsPath, "package "+pkg.Path, fmt.Sprintf("no component node has id %q", id),
				fmt.Sprintf("add a component node with id %q, a group, and a cited summary to the content file that owns it", id))
		}
	}
}

const packagePrefix = "pkg:"

func packageNodeID(pkgPath string) string { return packagePrefix + pkgPath }

// scope says whether an item belongs to the built system or to a design
// proposal. A proposal describes code that may not exist yet, so its added
// packages and import edges are not matched against the facts, and the
// nodes it adds resolve as node references inside it.
type scope struct {
	proposal bool
	added    map[string]bool // node ids the proposal adds
}

// nodeRules adjusts node validation to where the node appears.
type nodeRules struct {
	part   bool   // a child beneath another node
	prefix string // names the proposal operation that holds the node, if any
	scope
}

// validateNode checks one node and its children.
func (c *checker) validateNode(file string, n Node, rules nodeRules) {
	part := rules.part
	item := rules.prefix + "node " + n.ID
	if n.ID == "" {
		item = fmt.Sprintf("%snode %q", rules.prefix, n.Title)
		c.missing(file, item, "id")
	}
	if n.Title == "" {
		c.missing(file, item, "title")
	}
	if part {
		c.oneOf(file, item, "level", n.Level, []string{partLevel}, true)
		c.oneOf(file, item, "kind", n.Kind, nodeKinds, false)
	} else {
		c.oneOf(file, item, "level", n.Level, nodeLevels, true)
		c.oneOf(file, item, "kind", n.Kind, nodeKinds, true)
	}
	if n.Summary == nil {
		c.missing(file, item, "summary")
	}
	c.nodeFields(file, item, n, rules.scope)
	if n.Level == componentLevel {
		c.oneOf(file, item, "group", n.Group, groups, true)
	} else {
		c.oneOf(file, item, "group", n.Group, groups, false)
	}
	if pkgPath, ok := strings.CutPrefix(n.ID, packagePrefix); ok {
		if n.Level != componentLevel || n.Kind != "package" {
			c.add(file, item, "malformed data: a pkg: node is a component of kind package",
				"set level to component and kind to package, or drop the pkg: prefix")
		}
		if !rules.proposal && !c.packages[pkgPath] {
			c.add(file, item, "the node names no package "+pkgPath, "remove the node, or correct its id to an existing package")
		}
	}
	for _, child := range n.Children {
		c.validateNode(file, child, nodeRules{part: true, prefix: rules.prefix, scope: rules.scope})
	}
}

// nodeFields checks a node's optional fields: statements, glossary terms,
// links, contained nodes, diagrams, and other groups.
func (c *checker) nodeFields(file, item string, n Node, sc scope) {
	c.statement(file, item+" summary", n.Summary, sc.added)
	c.statements(file, item+" does", n.Does, sc.added)
	c.statements(file, item+" owns", n.Owns, sc.added)
	c.statements(file, item+" interfaces", n.Interfaces, sc.added)
	c.statements(file, item+" gotchas", n.Gotchas, sc.added)
	for _, term := range n.Terms {
		c.ref(file, item+" terms", "term:"+term, sc.added)
	}
	c.refs(file, item+" links", n.Links, sc.added)
	for _, id := range n.Contains {
		c.ref(file, item+" contains", "node:"+id, sc.added)
	}
	for _, d := range n.Diagrams {
		c.validateDiagram(file, item, d, sc)
	}
	for _, g := range n.AlsoIn {
		c.oneOf(file, item, "alsoIn", g, groups, true)
	}
}

// statements checks a statement list; each item is numbered from 1.
func (c *checker) statements(file, item string, list []Statement, extra map[string]bool) {
	for i := range list {
		c.statement(file, fmt.Sprintf("%s %d", item, i+1), &list[i], extra)
	}
}

func (c *checker) missing(file, item, field string) {
	c.add(file, item, "malformed data: missing "+field, "add "+field)
}

// oneOf checks that value belongs to a closed vocabulary.
func (c *checker) oneOf(file, item, field, value string, allowed []string, required bool) {
	if value == "" {
		if required {
			c.add(file, item, "malformed data: missing "+field, "set "+field+" to one of "+strings.Join(allowed, ", "))
		}
		return
	}
	if !slices.Contains(allowed, value) {
		c.add(file, item, fmt.Sprintf("malformed data: %s %q is not one of %s", field, value, strings.Join(allowed, ", ")),
			"use one of "+strings.Join(allowed, ", "))
	}
}

// edgeItem names an edge by its identity: from, to, and via.
func edgeItem(e Edge) string {
	return fmt.Sprintf("edge %s -> %s (%s)", e.From, e.To, e.Via)
}

// validateEdge checks an authored edge. An import edge must be in the facts'
// import graph unless a design proposal adds it; any other edge cites a
// source.
func (c *checker) validateEdge(file, prefix string, e Edge, sc scope) {
	item := prefix + edgeItem(e)
	for field, value := range map[string]string{"from": e.From, "to": e.To} {
		if value == "" {
			c.missing(file, item, field)
		}
	}
	if e.From != "" {
		c.ref(file, item, "node:"+e.From, sc.added)
	}
	if e.To != "" {
		c.ref(file, item, "node:"+e.To, sc.added)
	}
	if e.Label == "" {
		c.missing(file, item, "label")
	}
	c.oneOf(file, item, "via", e.Via, vias, true)
	c.refs(file, item, e.Refs, sc.added)
	if e.Via != "import" {
		if len(e.Refs) == 0 && slices.Contains(vias, e.Via) {
			c.add(file, item, "the edge has no source", "cite at least one reference in refs")
		}
		return
	}
	c.importEdge(file, item, e.From, e.To, sc.proposal)
}

// importEdge checks that from imports to in the facts. Edges a design
// proposal adds describe code that does not exist yet, so only their shape
// is checked.
func (c *checker) importEdge(file, item, from, to string, proposed bool) {
	fromPkg, fromOK := strings.CutPrefix(from, packagePrefix)
	toPkg, toOK := strings.CutPrefix(to, packagePrefix)
	if !fromOK || !toOK {
		c.add(file, item, "malformed data: an import edge joins two package nodes", "use package node ids, or a via other than import")
		return
	}
	if proposed || c.imports[Import{From: fromPkg, To: toPkg}] {
		return
	}
	c.add(file, item, fmt.Sprintf("%s does not import %s", fromPkg, toPkg),
		"remove it, reverse it, or use a via other than import with a cited source")
}

// validateFlow checks a flow, or a sequence diagram, which has the flow's
// shape without requiring a trigger.
func (c *checker) validateFlow(file, item string, f Flow, sequence bool, sc scope) {
	if !sequence {
		if f.ID == "" {
			c.missing(file, item, "id")
		}
		if f.Title == "" {
			c.missing(file, item, "title")
		}
		if f.Summary == nil {
			c.missing(file, item, "summary")
		}
		if f.Trigger == nil {
			c.missing(file, item, "trigger")
		}
		c.statement(file, item+" summary", f.Summary, sc.added)
	}
	c.statement(file, item+" trigger", f.Trigger, sc.added)
	if len(f.Participants) == 0 {
		c.missing(file, item, "participants")
	}
	participants := map[string]bool{}
	for _, id := range f.Participants {
		participants[id] = true
		c.ref(file, item+" participants", "node:"+id, sc.added)
	}
	if len(f.Steps) == 0 {
		c.missing(file, item, "steps")
	}
	for i, step := range f.Steps {
		c.validateStep(file, fmt.Sprintf("%s step %d", item, i+1), step, participants, sc)
	}
}

// validateStep checks one message between two participants.
func (c *checker) validateStep(file, item string, step Step, participants map[string]bool, sc scope) {
	for _, end := range []string{step.From, step.To} {
		if end == "" {
			c.add(file, item, "malformed data: missing from or to", "name the participants the step joins")
		} else if !participants[end] {
			c.add(file, item, end+" is not a participant", "add it to participants, or correct the step")
		}
	}
	if step.Label == "" {
		c.missing(file, item, "label")
	}
	c.oneOf(file, item, "kind", step.Kind, stepKinds, true)
	c.oneOf(file, item, "via", step.Via, vias, false)
	if len(step.Refs) == 0 {
		c.add(file, item, "the step has no source", "cite at least one reference in refs")
	}
	c.refs(file, item, step.Refs, sc.added)
	if step.Via == "import" {
		from, to := step.From, step.To
		if step.Kind == "return" {
			from, to = to, from
		}
		c.importEdge(file, item, from, to, sc.proposal)
	}
}
