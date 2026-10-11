package eviedb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

// Change notices (changes.spec.md story 5c): what the owner should hear
// about a Change that moved without them, "Merge when checks pass" above
// all, goes into the one notifications store beside workflow runs'
// notices, so the notifications list shows both. A Change's notice names
// its Change and no workflow or run.

const (
	// ChangeNotifyMerged is a Change merged once its checks passed.
	ChangeNotifyMerged WorkflowNotificationKind = "change_merged"
	// ChangeNotifyCleared is "Merge when checks pass" cleared without a
	// merge: a check failed, or GitHub refused the merge.
	ChangeNotifyCleared WorkflowNotificationKind = "change_merge_cleared"
	// ChangeNotifyNeedsRebase is a Change whose merge found a conflict.
	ChangeNotifyNeedsRebase WorkflowNotificationKind = "change_needs_rebase"
)

// ChangeNotification is one notice about a Change. Key names the event
// within the Workspace, so the same event notifies once.
type ChangeNotification struct {
	WorkspaceID memory.WorkspaceID
	ChangeID    string
	Kind        WorkflowNotificationKind
	Key         string
	Detail      string
}

// RecordChangeNotification records n unless a notice with its key exists.
func (s *Store) RecordChangeNotification(ctx context.Context, n ChangeNotification) error {
	switch {
	case !strings.HasPrefix(string(n.Kind), "change_"):
		return fmt.Errorf("%q is not a Change's notice", n.Kind)
	case n.WorkspaceID == "" || n.ChangeID == "" || n.Key == "":
		return errors.New("a Change's notice needs its Workspace, its Change and its key")
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workflow_notifications(dedup_key,workspace_id,workflow,run_id,kind,run_state,detail,origin_surface,session_id,created_at,read_at,change_id)
 VALUES(?,?,'',NULL,?,'',?,'changes',NULL,?,NULL,?)`, "change:"+string(n.WorkspaceID)+":"+n.Key, n.WorkspaceID, string(n.Kind), n.Detail, formatTime(s.now()), n.ChangeID)
	if err != nil {
		return fmt.Errorf("record the Change's notice: %w", err)
	}
	return nil
}
