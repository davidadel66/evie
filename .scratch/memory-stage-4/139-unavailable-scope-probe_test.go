package eviedb

import (
 "context"
 "testing"
 "time"

 "github.com/davidadel66/evie/internal/memory"
)

func TestHistoryUnavailableScopeDoesNotStarveOtherRequest(t *testing.T) {
 f:=newWorkerFixture(t);ctx:=context.Background()
 project,err:=f.store.RegisterProject(ctx,"Will archive",t.TempDir());if err!=nil{t.Fatal(err)}
 session,err:=f.store.CreateProjectSession(ctx,project.ID);if err!=nil{t.Fatal(err)}
 lease,err:=f.store.AcquireTurnLease(ctx,session.ID,"probe",time.Minute);if err!=nil{t.Fatal(err)}
 root,err:=f.store.AppendEventWithLease(ctx,session.ID,lease.HolderID,lease.FencingToken,memory.EventInput{Type:memory.EventUserMessage,Role:memory.RoleUser,Content:"I prefer tea."});if err!=nil{t.Fatal(err)}
 end,err:=f.store.AppendEventWithLease(ctx,session.ID,lease.HolderID,lease.FencingToken,memory.EventInput{Type:memory.EventAssistantMessage,Role:memory.RoleAssistant,ParentID:root.ID,Content:"Noted."});if err!=nil{t.Fatal(err)}
 scope:="project:"+string(project.ID)
 req:=memory.CompilerHistoryRequest{RequestID:"first-project",Ranges:[]memory.CompilerHistoryRange{{SourceScope:scope,Destination:scope,SessionID:session.ID,FirstSequence:root.Sequence,LastSequence:end.Sequence,FirstEventID:root.ID,LastEventID:end.ID}}}
 if _,err=f.store.SelectCompilerHistory(ctx,[]memory.ScopeContext{session.ScopeContext()},req,f.generation,&activationScript{});err!=nil{t.Fatal(err)}
 a,b:=historyRoot(t,f,"Independent global assertion.");historySelect(t,f,"later-global",historyRange(f,a,b))
 if _,err=f.db.Exec(`UPDATE projects SET archived=1 WHERE id=?`,project.ID);err!=nil{t.Fatal(err)}
 for range 12 { _,err=f.store.ReconcileCompilerHistory(ctx,f.config(&activationScript{})); if err!=nil{t.Log(err)} }
 var n int
 if err=f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_jobs WHERE session_id=?`,f.owner.SessionID).Scan(&n);err!=nil{t.Fatal(err)}
 if n!=1{t.Fatalf("unavailable first scope starved independent later selection: global jobs=%d",n)}
}
