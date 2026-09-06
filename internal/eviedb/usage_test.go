package eviedb

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/usage"
)

func TestConversationUsageIsBoundedContentFreeAndRestartStable(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Fixed timestamps exercise fractional seconds at the inclusive boundary.
	entries := []struct{ id, at, kind, role, payload, parent string }{
		{"trigger", "2026-09-01T03:59:59Z", "user_message", "user", `{}`, ""},
		{"snapshot", "2026-09-01T04:00:00Z", "context_snapshot", "", `{"schema_version":1,"canonical_model":"requested-model"}`, "trigger"},
		{"assistant", "2026-09-01T04:00:00.001Z", "assistant_message", "assistant", `{"usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20,"total_tokens":120},"secret":"NEVER_RETURN"}`, "trigger"},
		{"assistant2", "2026-09-01T05:00:00Z", "assistant_message", "assistant", `{"usage":{"input_tokens":900,"cached_input_tokens":0,"output_tokens":10,"total_tokens":910}}`, "trigger"},
		{"missing", "2026-09-01T06:00:00Z", "assistant_message", "assistant", `{}`, "trigger"},
		{"overflow", "2026-09-01T07:00:00Z", "assistant_message", "assistant", `{"usage":{"input_tokens":9223372036854775808,"cached_input_tokens":-1,"output_tokens":1.5}}`, "trigger"},
		{"outside", "2026-09-02T04:00:00Z", "assistant_message", "assistant", `{"usage":{"input_tokens":888}}`, "trigger"},
	}
	for i, e := range entries {
		content := "NEVER_RETURN"
		if e.kind == "context_snapshot" {
			content = ""
		}
		_, err := db.Exec(`INSERT INTO events(id,session_id,sequence,parent_id,event_type,role,content,payload_json,recorded_at) VALUES(?,?,?,NULLIF(?,''),?,NULLIF(?,''),?,?,?)`, e.id, session.ID, i+1, e.parent, e.kind, e.role, content, e.payload, e.at)
		if err != nil {
			t.Fatal(err)
		}
	}
	p, _ := usage.ParsePeriod("2026-09-01", "2026-09-02", "America/Detroit")
	got, err := store.ReadConversationUsage(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Metrics.Calls != 4 || got.Metrics.MeasuredCalls != 2 || *got.Metrics.Input.Value != 1000 || *got.Metrics.CachePercent != 8 {
		t.Fatalf("%+v", got.Metrics)
	}
	if len(got.Models) != 2 || got.Models[1].Name != "requested-model" || got.Models[1].Metrics.Calls != 1 {
		t.Fatalf("models %+v", got.Models)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "NEVER_RETURN") {
		t.Fatal("episode content leaked")
	}
	again, err := NewStore(db).ReadConversationUsage(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := json.Marshal(again)
	if string(encoded) != string(other) {
		t.Fatal("recreating reader changed results")
	}
}
