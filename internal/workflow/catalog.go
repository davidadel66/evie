package workflow

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CapabilityContract is what a workflow needs to know about one Capability
// to validate and simulate a capability step: its canonical ID and contract
// version, whether it reads or performs an external effect, whether its
// provider proves idempotency for a repeated Effect Intent, and its typed
// input and output. Connections and Resources, when a provider declares
// them, are the exact Connection IDs and resource selectors it serves
// (Stage 14 sub-stage 5); a step naming any other is invalid.
type CapabilityContract struct {
	ID          string
	Contract    string
	Effect      bool
	Idempotent  bool
	Input       *Type
	Output      *Type
	Connections []string
	Resources   []string
}

// SchemaSHA256 identifies the contract's schema: its typed input and output
// and its read or effect classification, as 64 lowercase hex digits (the
// form plugin schema evidence uses). The contract version label and the
// connections and resources a provider serves are not part of it; a run
// pins and checks them separately.
func (c CapabilityContract) SchemaSHA256() string {
	doc := struct {
		Effect     bool  `json:"effect"`
		Idempotent bool  `json:"idempotent"`
		Input      *Type `json:"input"`
		Output     *Type `json:"output"`
	}{c.Effect, c.Idempotent, canonicalType(c.Input), canonicalType(c.Output)}
	data, err := canonicalBytes(doc)
	if err != nil {
		// Types hold only JSON-encodable values.
		panic(fmt.Sprintf("encode capability schema: %v", err))
	}
	return digest(data)
}

// canonicalType is a copy of t with every object's required fields sorted,
// so equal types encode equally however they were built.
func canonicalType(t *Type) *Type {
	if t == nil {
		return nil
	}
	c := t.clone()
	var walk func(*Type)
	walk = func(t *Type) {
		if t == nil {
			return
		}
		sort.Strings(t.Required)
		for _, p := range t.Properties {
			walk(p)
		}
		walk(t.Items)
	}
	walk(c)
	return c
}

// Catalog resolves the Capability Contracts a definition may reference. It
// is metadata only: nothing in this package invokes a capability.
type Catalog interface {
	Capability(id string) (CapabilityContract, bool)
}

type mapCatalog map[string]CapabilityContract

func (c mapCatalog) Capability(id string) (CapabilityContract, bool) {
	contract, ok := c[id]
	return contract, ok
}

// NewCatalog returns a catalog of exactly these contracts.
func NewCatalog(contracts ...CapabilityContract) Catalog {
	c := mapCatalog{}
	for _, contract := range contracts {
		c[contract.ID] = contract
	}
	return c
}

// WorkspaceAccess is the authority ceiling a definition's capability steps
// and requested Standing Authority must stay within (ADR 0027).
type WorkspaceAccess struct {
	Connections []ConnectionAccess `json:"connections"`
}

// ConnectionAccess binds one Connection ID to its exact allowed resources.
type ConnectionAccess struct {
	ID        string   `json:"id"`
	Resources []string `json:"resources"`
}

func (a WorkspaceAccess) connection(id string) (ConnectionAccess, bool) {
	for _, c := range a.Connections {
		if c.ID == id {
			return c, true
		}
	}
	return ConnectionAccess{}, false
}

// Allows reports whether the access lists this exact resource on this
// connection.
func (a WorkspaceAccess) Allows(connection, resource string) bool {
	c, ok := a.connection(connection)
	return ok && c.allows(resource)
}

func (c ConnectionAccess) allows(resource string) bool {
	for _, r := range c.Resources {
		if r == resource {
			return true
		}
	}
	return false
}

// WorkerCeiling is the operator's subagent policy that bounds agent step
// budgets: the per-worker deadline and token budget.
type WorkerCeiling struct {
	Deadline time.Duration
	Tokens   int64
}

// Environment is what a definition is validated against.
type Environment struct {
	Capabilities Catalog
	Access       WorkspaceAccess
	Workers      WorkerCeiling
	// AccessUnchecked reviews a definition outside any Workspace (the
	// standalone validate and simulate commands): capability steps and
	// requested authority are not checked against Access, which only a
	// Workspace has; proposing, approving and running always check it.
	AccessUnchecked bool
}

// Source reads one definition's files by slash-separated path relative to
// its root: a directory (DirSource), or the reviewed tree at one procedural
// Git commit (procedural.Source, sub-stage 2).
type Source interface {
	ReadFile(name string) ([]byte, error)
}

// maxSourceFileBytes bounds any one file a Source reads; each kind of file
// has its own smaller limit.
const maxSourceFileBytes = 1 << 20

// DirSource reads a definition from a directory through os.Root, so links
// and paths cannot escape it.
type DirSource struct {
	Dir string
}

// ReadFile reads one regular file inside the directory.
func (d DirSource) ReadFile(name string) ([]byte, error) {
	root, err := os.OpenRoot(d.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSourceFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSourceFileBytes {
		return nil, fmt.Errorf("%s is larger than %d KiB", name, maxSourceFileBytes>>10)
	}
	return data, nil
}

func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }
