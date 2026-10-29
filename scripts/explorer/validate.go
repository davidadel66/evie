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
	packages map[string]bool   // package paths from the facts

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
		}
	}
	c.checkPackageCoverage()
}

// index records every node id, including children, and reports duplicates.
func (c *checker) index(files []dataFile) {
	c.nodes = map[string]string{}
	c.packages = map[string]bool{}
	for _, pkg := range c.facts.Packages {
		c.packages[pkg.Path] = true
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
			walk(file, n.Children)
		}
	}
	for _, file := range files {
		if file.Content != nil {
			walk(file.Path, file.Content.Nodes)
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

// nodeRules adjusts node validation to where the node appears.
type nodeRules struct {
	part     bool            // a child beneath another node
	proposed bool            // added by a design proposal, so its package may not exist yet
	extra    map[string]bool // node ids that exist only inside that proposal
}

// validateNode checks one node and its children.
func (c *checker) validateNode(file string, n Node, rules nodeRules) {
	part := rules.part
	item := "node " + n.ID
	if n.ID == "" {
		item = fmt.Sprintf("node %q", n.Title)
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
	c.statement(file, item+" summary", n.Summary, rules.extra)
	c.statements(file, item+" does", n.Does, rules.extra)
	c.statements(file, item+" owns", n.Owns, rules.extra)
	c.statements(file, item+" interfaces", n.Interfaces, rules.extra)
	c.statements(file, item+" gotchas", n.Gotchas, rules.extra)
	for _, term := range n.Terms {
		c.ref(file, item+" terms", "term:"+term, rules.extra)
	}
	c.refs(file, item+" links", n.Links, rules.extra)
	for _, id := range n.Contains {
		c.ref(file, item+" contains", "node:"+id, rules.extra)
	}
	if n.Level == componentLevel {
		c.oneOf(file, item, "group", n.Group, groups, true)
	} else {
		c.oneOf(file, item, "group", n.Group, groups, false)
	}
	for _, g := range n.AlsoIn {
		c.oneOf(file, item, "alsoIn", g, groups, true)
	}
	if pkgPath, ok := strings.CutPrefix(n.ID, packagePrefix); ok {
		if n.Level != componentLevel || n.Kind != "package" {
			c.add(file, item, "malformed data: a pkg: node is a component of kind package",
				"set level to component and kind to package, or drop the pkg: prefix")
		}
		if !rules.proposed && !c.packages[pkgPath] {
			c.add(file, item, "the node names no package "+pkgPath, "remove the node, or correct its id to an existing package")
		}
	}
	for _, child := range n.Children {
		c.validateNode(file, child, nodeRules{part: true, proposed: rules.proposed, extra: rules.extra})
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
