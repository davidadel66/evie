package eviedb

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// changes.spec.md story 5c: Changes leave their notices in the one
// notifications store, beside workflow runs': once per event, immutable but
// for their read state.
func TestChangeNotificationsShareTheNotificationStore(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	notice := ChangeNotification{WorkspaceID: ws.ID, ChangeID: "pr:7", Kind: ChangeNotifyMerged, Key: "pr:7:merged:abc",
		Detail: "Pull request #7 merged once its checks passed."}
	for range 2 {
		if err := store.RecordChangeNotification(ctx, notice); err != nil {
			t.Fatal(err)
		}
	}
	list, err := store.ListWorkflowNotifications(ctx, WorkflowNotificationFilter{WorkspaceID: ws.ID})
	if err != nil || len(list) != 1 {
		t.Fatalf("notices = %+v, %v", list, err)
	}
	n := list[0]
	if n.Kind != ChangeNotifyMerged || n.ChangeID != "pr:7" || n.Workflow != "" || n.RunID != "" || n.Surface != "changes" || n.Detail != notice.Detail {
		t.Fatalf("notice = %+v", n)
	}
	if counts, err := store.WorkflowNotificationCounts(ctx, ws.ID); err != nil || counts.Unread != 1 {
		t.Fatalf("counts = %+v, %v", counts, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workflow_notifications SET change_id='pr:8' WHERE id=?`, n.ID); err == nil {
		t.Fatal("a notice's Change was changed")
	}
	if marked, err := store.MarkWorkflowNotifications(ctx, []int64{n.ID}, true); err != nil || marked != 1 {
		t.Fatalf("mark = %d, %v", marked, err)
	}
	for _, bad := range []ChangeNotification{
		{WorkspaceID: ws.ID, Kind: ChangeNotifyMerged, Key: "k1", Detail: "no Change"},
		{WorkspaceID: ws.ID, ChangeID: "pr:7", Kind: "succeeded", Key: "k2", Detail: "a workflow kind"},
		{WorkspaceID: ws.ID, ChangeID: "pr:7", Kind: ChangeNotifyMerged, Detail: "no key"},
	} {
		if err := store.RecordChangeNotification(ctx, bad); err == nil {
			t.Fatalf("%+v was recorded", bad)
		}
	}
}

// Story 5c's migration: a database whose notifications table predates
// Change notices is rebuilt with every notice, index and protection kept,
// and takes Change notices after.
func TestTheNotificationsTableMigratesForChangeNotices(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	// The table as sub-stage 11 created it, holding one schedule notice.
	if _, err := db.ExecContext(ctx, `DROP TABLE workflow_notifications;
 CREATE TABLE workflow_notifications (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 dedup_key TEXT NOT NULL UNIQUE,
 workspace_id TEXT NOT NULL,
 workflow TEXT NOT NULL CHECK(length(workflow) BETWEEN 1 AND 64),
 run_id TEXT REFERENCES workflow_runs(id),
 kind TEXT NOT NULL CHECK(kind IN (`+workflowNotificationKindsBeforeChanges+`)),
 run_state TEXT NOT NULL,
 detail TEXT NOT NULL,
 origin_surface TEXT NOT NULL,
 session_id TEXT,
 created_at TEXT NOT NULL,
 read_at TEXT,
 CHECK((kind IN ('schedule_skipped','schedule_failed')) = (run_id IS NULL)));
 CREATE INDEX workflow_notifications_by_workspace ON workflow_notifications(workspace_id, id);`); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := insertScheduleNotification(ctx, conn, "schedule:old", ws.ID, "finance-daily", WorkflowNotifyScheduleSkipped, "skipped", time.Now()); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	db.Close()

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewStore(db)
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='workflow_notifications'`).Scan(&definition); err != nil || !strings.Contains(definition, "change_id") {
		t.Fatalf("definition = %s, %v", definition, err)
	}
	list, err := store.ListWorkflowNotifications(ctx, WorkflowNotificationFilter{WorkspaceID: ws.ID, All: true})
	if err != nil || len(list) != 1 || list[0].Kind != WorkflowNotifyScheduleSkipped || list[0].Workflow != "finance-daily" {
		t.Fatalf("kept notices = %+v, %v", list, err)
	}
	for _, name := range []string{"workflow_notifications_by_workspace", "workflow_notifications_by_session", "workflow_notifications_by_run",
		"workflow_notifications_content", "workflow_notifications_no_delete", "workflow_runs_notify"} {
		var found int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name=?`, name).Scan(&found); err != nil || found != 1 {
			t.Fatalf("%s: %d, %v", name, found, err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE workflow_notifications SET detail='changed' WHERE dedup_key='schedule:old'`); err == nil {
		t.Fatal("a migrated notice is not immutable")
	}
	if err := store.RecordChangeNotification(ctx, ChangeNotification{WorkspaceID: ws.ID, ChangeID: "pr:7", Kind: ChangeNotifyCleared, Key: "pr:7:cleared",
		Detail: "The check ci failed."}); err != nil {
		t.Fatal(err)
	}
}
