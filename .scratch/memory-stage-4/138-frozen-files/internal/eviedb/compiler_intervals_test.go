package eviedb

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestCompilerIntervalsFillEarlierGapsWithoutOverlappingLaterOwners(t *testing.T) {
	f := newWorkerFixture(t)
	activationStart(t, f)
	ctx := context.Background()
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "An assertion."})
	selection := memory.CompilationSelection{SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: 7, Destination: "global"}
	err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		own := func(first, last int64) error {
			_, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_selections(selection_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,state,window) VALUES(?,?,'global',?,?,?,?,'failed','{}')`, fmt.Sprintf("interval-%d-%d", first, last), f.generationID, f.owner.SessionID, root.ID, first, last)
			return err
		}
		if err := own(3, 3); err != nil {
			return err
		}
		if err := own(5, 5); err != nil {
			return err
		}
		for _, want := range [][2]int64{{1, 2}, {4, 4}, {6, 7}} {
			interval, err := nextCompilerInterval(ctx, conn, f.generationID, selection, 1)
			if err != nil {
				return err
			}
			if interval.First != want[0] || interval.Last != want[1] || interval.SelectionID != "" {
				t.Fatalf("next unowned interval %+v want%v", interval, want)
			}
			if err := own(interval.First, interval.Last); err != nil {
				return err
			}
		}
		reused, err := nextCompilerInterval(ctx, conn, f.generationID, selection, 1)
		if err != nil {
			return err
		}
		if reused.SelectionID != "interval-6-7" || reused.State != "failed" {
			t.Fatalf("existing unresolved owner changed: %+v", reused)
		}
		var overlaps int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_selections a JOIN memory_compiler_selections b ON a.selection_id<b.selection_id AND a.first_sequence<=b.last_sequence AND b.first_sequence<=a.last_sequence`).Scan(&overlaps); err != nil {
			return err
		}
		if overlaps != 0 {
			t.Fatal("filled intervals overlap immutable owners")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
