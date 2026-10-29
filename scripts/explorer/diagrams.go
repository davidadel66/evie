package main

import (
	"fmt"
	"sort"
)

// validateDiagram checks one diagram attached to a node.
func (c *checker) validateDiagram(file, nodeItem string, d Diagram, sc scope) {
	item := nodeItem + " diagram " + d.ID
	if d.ID == "" {
		c.missing(file, item, "id")
	}
	if d.Title == "" {
		c.missing(file, item, "title")
	}
	if d.Summary == nil {
		c.missing(file, item, "summary")
	}
	c.statement(file, item+" summary", d.Summary, sc.added)
	c.oneOf(file, item, "kind", d.Kind, diagramKinds, true)
	c.foreignFields(file, item, d)
	switch d.Kind {
	case "graph":
		c.validateGraph(file, item, d, sc)
	case "sequence":
		c.validateFlow(file, item, Flow{Trigger: d.Trigger, Participants: d.Participants, Steps: d.Steps}, true, sc)
	case "state":
		c.validateStates(file, item, d, sc)
	case "er":
		c.validateER(file, item, d)
	}
}

// foreignFields reports fields that belong to another diagram kind.
func (c *checker) foreignFields(file, item string, d Diagram) {
	present := map[string]string{
		"nodes":        nonEmpty(len(d.Nodes) > 0, "graph"),
		"edges":        nonEmpty(len(d.Edges) > 0, "graph"),
		"lanes":        nonEmpty(len(d.Lanes) > 0, "graph"),
		"trigger":      nonEmpty(d.Trigger != nil, "sequence"),
		"participants": nonEmpty(len(d.Participants) > 0, "sequence"),
		"steps":        nonEmpty(len(d.Steps) > 0, "sequence"),
		"states":       nonEmpty(len(d.States) > 0, "state"),
		"initial":      nonEmpty(d.Initial != "", "state"),
		"transitions":  nonEmpty(len(d.Transitions) > 0, "state"),
		"database":     nonEmpty(d.Database != "", "er"),
		"tables":       nonEmpty(len(d.Tables) > 0, "er"),
	}
	fields := make([]string, 0, len(present))
	for field := range present {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		if owner := present[field]; owner != "" && owner != d.Kind && d.Kind != "" {
			c.add(file, item, fmt.Sprintf("malformed data: %s does not belong to a %s diagram", field, d.Kind),
				"remove "+field+", or change the diagram's kind")
		}
	}
}

func nonEmpty(present bool, kind string) string {
	if present {
		return kind
	}
	return ""
}

func (c *checker) validateGraph(file, item string, d Diagram, sc scope) {
	if len(d.Nodes) == 0 {
		c.missing(file, item, "nodes")
	}
	lanes := map[string]bool{}
	for _, lane := range d.Lanes {
		if lane.ID == "" || lane.Title == "" {
			c.add(file, item, "malformed data: a lane needs an id and a title", "add the missing field")
		}
		lanes[lane.ID] = true
	}
	local := map[string]bool{}
	for _, n := range d.Nodes {
		nodeItem := item + " node " + n.ID
		if n.ID == "" || n.Label == "" {
			c.add(file, nodeItem, "malformed data: a graph node needs an id and a label", "add the missing field")
		}
		if local[n.ID] {
			c.add(file, nodeItem, "malformed data: duplicate local node id", "give each graph node a unique id")
		}
		local[n.ID] = true
		if n.Node != "" {
			c.ref(file, nodeItem, "node:"+n.Node, sc.added)
		}
		if n.Lane != "" && !lanes[n.Lane] {
			c.add(file, nodeItem, "there is no lane "+n.Lane, "add the lane to lanes, or correct the node's lane")
		}
	}
	for i, e := range d.Edges {
		edgeItem := fmt.Sprintf("%s edge %d", item, i+1)
		for _, end := range []string{e.From, e.To} {
			if !local[end] {
				c.add(file, edgeItem, "there is no local node "+end, "join two nodes of this graph")
			}
		}
		c.refs(file, edgeItem, e.Refs, sc.added)
	}
}

func (c *checker) validateStates(file, item string, d Diagram, sc scope) {
	if len(d.States) == 0 {
		c.missing(file, item, "states")
	}
	states := map[string]bool{}
	for _, s := range d.States {
		stateItem := item + " state " + s.ID
		if s.ID == "" {
			c.missing(file, stateItem, "id")
		}
		if states[s.ID] {
			c.add(file, stateItem, "malformed data: duplicate state id", "give each state a unique id")
		}
		states[s.ID] = true
		if len(s.Refs) == 0 {
			c.add(file, stateItem, "the state has no source", "cite at least one reference in refs")
		}
		c.refs(file, stateItem, s.Refs, sc.added)
	}
	if d.Initial == "" {
		c.missing(file, item, "initial")
	} else if !states[d.Initial] {
		c.add(file, item, "the initial state "+d.Initial+" is not a state", "name one of the diagram's states")
	}
	for i, t := range d.Transitions {
		transitionItem := fmt.Sprintf("%s transition %d", item, i+1)
		for _, end := range []string{t.From, t.To} {
			if !states[end] {
				c.add(file, transitionItem, "there is no state "+end, "join two states of this diagram")
			}
		}
		if len(t.Refs) == 0 {
			c.add(file, transitionItem, "the transition has no source", "cite at least one reference in refs")
		}
		c.refs(file, transitionItem, t.Refs, sc.added)
	}
}

// validateER checks that an er diagram names a database and tables the facts
// hold; its columns and relationships come from the facts.
func (c *checker) validateER(file, item string, d Diagram) {
	if d.Database == "" {
		c.missing(file, item, "database")
		return
	}
	found := false
	for _, db := range c.facts.Databases {
		found = found || db.ID == d.Database
	}
	if !found {
		c.add(file, item, "there is no database "+d.Database+" in the facts", refFixes["table"])
	}
	if len(d.Tables) == 0 {
		c.missing(file, item, "tables")
	}
	for _, table := range d.Tables {
		c.ref(file, item, "table:"+d.Database+"."+table, nil)
	}
}
