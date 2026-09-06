package eviedb
import("context";"path/filepath";"testing";"time";"github.com/davidadel66/evie/internal/task")
func TestStage4UnchangedTaskTimeOrderingDiagnosis(t *testing.T) {
 db,err:=OpenDBAt(filepath.Join(t.TempDir(),"tasks.db"));if err!=nil{t.Fatal(err)};defer db.Close();s:=NewStore(db)
 now:=time.Date(2026,9,5,12,0,0,100000000,time.UTC);s.now=func()time.Time{return now}
 if _,err=s.CreateGlobalTask(mutationContext("local","first","seed"),task.CreateInput{Title:"first",IdempotencyKey:"first"});err!=nil{t.Fatal(err)}
 now=now.Add(10*time.Millisecond)
 if _,err=s.CreateGlobalTask(mutationContext("local","second","seed"),task.CreateInput{Title:"second",IdempotencyKey:"second"});err!=nil{t.Fatal(err)}
 items,err:=s.ListGlobalTasks(context.Background(),task.ListFilter{});if err!=nil{t.Fatal(err)}
 if len(items)!=2||items[0].Title!="first"{t.Fatalf("chronological order=%v; formatted .1Z sorts after .11Z",taskTitles(items))}
}
