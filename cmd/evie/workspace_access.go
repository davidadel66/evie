package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// workspaceAccess answers `evie workspaces access <workspace-id>`, which
// prints the Workspace's current access ceiling, and `evie workspaces access
// <workspace-id> --expect <revision> --set <connection>=<resource>,...`
// (repeatable) or `--clear`, which replaces it as a new revision when
// expected is still the latest (Stage 14 sub-stage 2, 2026-10-04). Access is
// the outer bound of every workflow's Standing Authority and grants nothing
// by itself; a removal blocks every approved workflow it no longer covers,
// at once and until the owner approves again.
func workspaceAccess(ctx context.Context, args []string, out io.Writer, store *eviedb.Store) error {
	usage := errors.New(workspacesUsage)
	id := memory.WorkspaceID(args[2])
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if len(args) == 3 {
		current, err := store.WorkspaceAccess(ctx, id)
		if err != nil {
			return err
		}
		return encoder.Encode(current)
	}
	expected := int64(-1)
	clear := false
	access := workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{}}
	for i := 3; i < len(args); i++ {
		switch args[i] {
		case "--clear":
			clear = true
		case "--expect", "--set":
			if i+1 >= len(args) {
				return usage
			}
			value := args[i+1]
			if args[i] == "--expect" {
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || n < 0 || expected >= 0 {
					return usage
				}
				expected = n
			} else {
				connection, resources, ok := strings.Cut(value, "=")
				if !ok || connection == "" || resources == "" {
					return usage
				}
				access.Connections = append(access.Connections, workflow.ConnectionAccess{ID: connection, Resources: strings.Split(resources, ",")})
			}
			i++
		default:
			return usage
		}
	}
	if expected < 0 || clear == (len(access.Connections) > 0) {
		return usage
	}
	changed, err := store.SetWorkspaceAccess(ctx, id, expected, access)
	if err != nil {
		return err
	}
	return encoder.Encode(changed)
}
