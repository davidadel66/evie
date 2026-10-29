package main

import "fmt"

// validateProposals checks every design proposal against the built system: a
// change or remove targets an existing node, edge, or flow, and an add does
// not collide with one. Existing edges include the facts' import edges,
// which the viewer draws between package nodes.
func (c *checker) validateProposals(files []dataFile) {
	edges := map[string]bool{}
	for _, file := range files {
		if file.Content != nil {
			for _, e := range file.Content.Edges {
				edges[edgeItem(e)] = true
			}
		}
	}
	for _, imp := range c.facts.Imports {
		edges[edgeItem(Edge{From: packageNodeID(imp.From), To: packageNodeID(imp.To), Via: "import"})] = true
	}
	ids := map[string]string{}
	for _, file := range files {
		if file.Proposal == nil {
			continue
		}
		p := file.Proposal
		item := "proposal " + p.ID
		if p.ID == "" {
			c.missing(file.Path, item, "id")
		} else if first, dup := ids[p.ID]; dup {
			c.add(file.Path, item, "duplicate proposal id, also defined in "+first, "give each design proposal a unique id")
		} else {
			ids[p.ID] = file.Path
		}
		c.validateProposal(file.Path, item, p, edges)
	}
}

func (c *checker) validateProposal(file, item string, p *Proposal, edges map[string]bool) {
	if p.Feature == "" {
		c.missing(file, item, "feature")
	}
	if p.Title == "" {
		c.missing(file, item, "title")
	}
	c.oneOf(file, item, "status", p.Status, statuses, true)
	sc := scope{proposal: true, added: map[string]bool{}}
	for _, op := range p.Nodes {
		if op.Op == "add" && op.ID != "" {
			sc.added[op.ID] = true
		}
	}
	if p.Spec != "" {
		c.ref(file, item+" spec", p.Spec, sc.added)
	}
	if p.Summary == nil {
		c.missing(file, item, "summary")
	}
	c.statement(file, item+" summary", p.Summary, sc.added)
	c.statements(file, item+" requirements", p.Requirements, sc.added)
	c.statements(file, item+" decisions", p.Decisions, sc.added)

	for _, op := range p.Nodes {
		opItem := fmt.Sprintf("%s %s node %s", item, op.Op, op.ID)
		c.oneOf(file, opItem, "op", op.Op, operations, true)
		_, exists := c.nodes[op.ID]
		if len(op.Children) > 0 {
			c.add(file, opItem, "malformed data: a design proposal node operation carries no children; children are reserved for the depth pass",
				"remove children")
		}
		switch op.Op {
		case "add":
			if exists {
				c.add(file, opItem, "the node already exists", "use change, or choose a new id")
			}
			c.validateNode(file, op.Node, nodeRules{prefix: item + " add ", scope: sc})
		case "change":
			c.targetExists(file, opItem, exists, "node")
			c.topLevel(file, opItem, op.ID)
			c.validateNodeChange(file, opItem, op.Node, sc)
		case "remove":
			c.targetExists(file, opItem, exists, "node")
			c.topLevel(file, opItem, op.ID)
		}
	}
	for _, op := range p.Edges {
		opItem := fmt.Sprintf("%s %s %s", item, op.Op, edgeItem(op.Edge))
		c.oneOf(file, opItem, "op", op.Op, operations, true)
		exists := edges[edgeItem(op.Edge)]
		switch op.Op {
		case "add":
			if exists {
				c.add(file, opItem, "the edge already exists", "use change, or add a different edge")
			}
			c.validateEdge(file, item+" add ", op.Edge, sc)
		case "change", "remove":
			c.targetExists(file, opItem, exists, "edge")
			c.refs(file, opItem, op.Refs, sc.added)
		}
	}
	for _, op := range p.Flows {
		opItem := fmt.Sprintf("%s %s flow %s", item, op.Op, op.ID)
		c.oneOf(file, opItem, "op", op.Op, operations, true)
		_, exists := c.flows[op.ID]
		switch op.Op {
		case "add":
			if exists {
				c.add(file, opItem, "the flow already exists", "use change, or choose a new id")
			}
			c.validateFlow(file, opItem, op.Flow, false, sc)
		case "change":
			c.targetExists(file, opItem, exists, "flow")
			c.statement(file, opItem+" summary", op.Summary, sc.added)
			c.statement(file, opItem+" trigger", op.Trigger, sc.added)
			for _, id := range op.Participants {
				c.ref(file, opItem+" participants", "node:"+id, sc.added)
			}
			for i, step := range op.Steps {
				for _, ref := range step.Refs {
					c.ref(file, fmt.Sprintf("%s step %d", opItem, i+1), ref, sc.added)
				}
			}
		case "remove":
			c.targetExists(file, opItem, exists, "flow")
		}
	}
}

// topLevel reports a design proposal node operation that targets a part;
// node operations change and remove top-level nodes only.
func (c *checker) topLevel(file, item, id string) {
	if parent, ok := c.parts[id]; ok {
		c.add(file, item, "the node is a part of another node ("+parent+"); design proposal node operations target top-level nodes only",
			"target "+parent+" instead")
	}
}

func (c *checker) targetExists(file, item string, exists bool, what string) {
	if !exists {
		c.add(file, item, "the "+what+" does not exist in the built system", "target an existing "+what+", or use add")
	}
}

// validateNodeChange checks the fields a change operation sets; the rest of
// the node stays as built.
func (c *checker) validateNodeChange(file, item string, n Node, sc scope) {
	c.oneOf(file, item, "level", n.Level, nodeLevels, false)
	c.oneOf(file, item, "kind", n.Kind, nodeKinds, false)
	c.oneOf(file, item, "group", n.Group, groups, false)
	c.nodeFields(file, item, n, sc)
}
