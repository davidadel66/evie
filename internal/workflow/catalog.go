package workflow

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// CapabilityContract is what a workflow needs to know about one Capability
// to validate and simulate a capability step: its canonical ID and contract
// version, whether it reads or performs an external effect, whether its
// provider proves idempotency for a repeated Effect Intent, and its typed
// input and output.
type CapabilityContract struct {
	ID         string
	Contract   string
	Effect     bool
	Idempotent bool
	Input      *Type
	Output     *Type
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
	Connections []ConnectionAccess
}

// ConnectionAccess binds one Connection ID to its exact allowed resources.
type ConnectionAccess struct {
	ID        string
	Resources []string
}

func (a WorkspaceAccess) connection(id string) (ConnectionAccess, bool) {
	for _, c := range a.Connections {
		if c.ID == id {
			return c, true
		}
	}
	return ConnectionAccess{}, false
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
}

// Source reads one definition's files by slash-separated path relative to
// its root. Today that is a directory; procedural Git (sub-stage 2) will
// read the reviewed tree at one commit through the same seam.
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
