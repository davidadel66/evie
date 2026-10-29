package main

// The written half of the explorer data. Field names are the JSON contract
// documented in docs/architecture/explorer/README.md; decoding is strict, so
// an unknown or wrongly cased key is malformed data.

// header is read first to choose how to decode a data file.
type header struct {
	Format int    `json:"format"`
	Kind   string `json:"kind"`
}

// Content is one subsystem's written content.
type Content struct {
	Format int    `json:"format"`
	Kind   string `json:"kind"`
	Owner  string `json:"owner"`
	Nodes  []Node `json:"nodes"`
	Edges  []Edge `json:"edges"`
	Flows  []Flow `json:"flows"`
}

// Statement is one sentence and the references it rests on.
type Statement struct {
	Text   string   `json:"text"`
	Refs   []string `json:"refs"`
	Detail string   `json:"detail"`
}

// Node is one explorer node: a person, system, container, package, or a part
// beneath one.
type Node struct {
	ID         string      `json:"id"`
	Level      string      `json:"level"`
	Kind       string      `json:"kind"`
	Title      string      `json:"title"`
	Summary    *Statement  `json:"summary"`
	Does       []Statement `json:"does"`
	Owns       []Statement `json:"owns"`
	Interfaces []Statement `json:"interfaces"`
	Gotchas    []Statement `json:"gotchas"`
	Terms      []string    `json:"terms"`
	Diagrams   []Diagram   `json:"diagrams"`
	Links      []string    `json:"links"`
	Contains   []string    `json:"contains"`
	Group      string      `json:"group"`
	Subgroup   string      `json:"subgroup"`
	AlsoIn     []string    `json:"alsoIn"`
	Children   []Node      `json:"children"`
}

// Edge is an authored connection between two nodes.
type Edge struct {
	From  string   `json:"from"`
	To    string   `json:"to"`
	Label string   `json:"label"`
	Via   string   `json:"via"`
	Refs  []string `json:"refs"`
}

// Flow is a key runtime flow drawn as a sequence diagram.
type Flow struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Summary      *Statement `json:"summary"`
	Trigger      *Statement `json:"trigger"`
	Participants []string   `json:"participants"`
	Steps        []Step     `json:"steps"`
}

// Step is one message of a flow or sequence diagram.
type Step struct {
	From  string   `json:"from"`
	To    string   `json:"to"`
	Label string   `json:"label"`
	Kind  string   `json:"kind"`
	Via   string   `json:"via"`
	Refs  []string `json:"refs"`
}

// Diagram is a picture attached to a node. Which fields apply depends on its
// kind: graph, sequence, state, or er.
type Diagram struct {
	ID      string     `json:"id"`
	Title   string     `json:"title"`
	Kind    string     `json:"kind"`
	Summary *Statement `json:"summary"`
	// graph
	Nodes []DiagramNode `json:"nodes"`
	Edges []DiagramEdge `json:"edges"`
	Lanes []Lane        `json:"lanes"`
	// sequence
	Trigger      *Statement `json:"trigger"`
	Participants []string   `json:"participants"`
	Steps        []Step     `json:"steps"`
	// state
	States      []State      `json:"states"`
	Initial     string       `json:"initial"`
	Transitions []Transition `json:"transitions"`
	// er
	Database string   `json:"database"`
	Tables   []string `json:"tables"`
}

// DiagramNode is a node local to a graph diagram, optionally standing for an
// explorer node.
type DiagramNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Node  string `json:"node"`
	Lane  string `json:"lane"`
}

// DiagramEdge connects two local nodes of a graph diagram.
type DiagramEdge struct {
	From  string   `json:"from"`
	To    string   `json:"to"`
	Label string   `json:"label"`
	Refs  []string `json:"refs"`
}

// Lane groups local nodes of a graph diagram.
type Lane struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// State is one state of a state diagram.
type State struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Refs  []string `json:"refs"`
}

// Transition moves a state diagram between two states.
type Transition struct {
	From  string   `json:"from"`
	To    string   `json:"to"`
	Label string   `json:"label"`
	Refs  []string `json:"refs"`
}

// Proposal is a design proposal: what one feature would add, change, or
// remove, toggled on top of the built system.
type Proposal struct {
	Format       int         `json:"format"`
	Kind         string      `json:"kind"`
	ID           string      `json:"id"`
	Feature      string      `json:"feature"`
	Title        string      `json:"title"`
	Status       string      `json:"status"`
	Spec         string      `json:"spec"`
	Summary      *Statement  `json:"summary"`
	Nodes        []NodeOp    `json:"nodes"`
	Edges        []EdgeOp    `json:"edges"`
	Flows        []FlowOp    `json:"flows"`
	Requirements []Statement `json:"requirements"`
	Decisions    []Statement `json:"decisions"`
}

// NodeOp adds, changes, or removes a node; the node's fields sit beside op.
type NodeOp struct {
	Op string `json:"op"`
	Node
}

// EdgeOp adds, changes, or removes an edge, identified by from, to, and via.
type EdgeOp struct {
	Op string `json:"op"`
	Edge
}

// FlowOp adds, changes, or removes a flow, identified by id.
type FlowOp struct {
	Op string `json:"op"`
	Flow
}

// The closed vocabularies of the format.
var (
	nodeLevels     = []string{"context", "container", "component"}
	nodeKinds      = []string{"person", "system", "external-system", "process", "user-interface", "cli", "store", "package"}
	groups         = []string{"memory", "runtime", "intelligence", "infrastructure"}
	vias           = []string{"import", "call", "http", "sql", "process", "network", "file", "event"}
	stepKinds      = []string{"call", "return", "async", "note"}
	diagramKinds   = []string{"graph", "sequence", "state", "er"}
	statuses       = []string{"proposed", "accepted", "building", "example"}
	operations     = []string{"add", "change", "remove"}
	partLevel      = "part"
	componentLevel = "component"
)
