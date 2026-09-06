from pathlib import Path
import hashlib,json,subprocess,datetime
root=Path('.scratch/memory-stage-4')
new=['internal/memory/candidate_identity.go','internal/eviedb/compiler_identity.go','internal/eviedb/candidate_review_identity.go','internal/eviedb/candidate_review_identity_effect.go','internal/eviedb/candidate_review_identity_test.go','internal/eviedb/candidate_review_identity_encoding_test.go','internal/eviedb/testdata/candidate_review_encoding_v2.json','cmd/evie/candidate_identity_test.go','cmd/evie/docs/research/memory-stage-4-identity-review.md']
paths=set(new)
for original in (root/'141-originals').rglob('*'):
 if original.is_file():
  relative=str(original.relative_to(root/'141-originals'))
  if original.read_bytes()!=Path(relative).read_bytes():paths.add(relative)
files=[]
for path in sorted(paths):
 data=Path(path).read_bytes();original=root/'141-originals'/path
 check=subprocess.run(['git','diff','--no-index','--check','--',str(original) if original.exists() else '/dev/null',path],capture_output=True,text=True)
 if check.returncode not in (0,1) or check.stdout or check.stderr:raise Exception((path,check.returncode,check.stdout,check.stderr))
 if path.endswith('.go'):
  formatting=subprocess.run(['gofmt','-l',path],capture_output=True,text=True,check=True)
  if formatting.stdout:raise Exception(formatting.stdout)
 frozen=root/'141-frozen-files'/path;frozen.parent.mkdir(parents=True,exist_ok=True);frozen.write_bytes(data)
 files.append({'path':path,'before_sha256':hashlib.sha256(original.read_bytes()).hexdigest() if original.exists() else None,'after_sha256':hashlib.sha256(data).hexdigest(),'bytes':len(data)})
checkpoint={'ticket':141,'title':'Identity and Predicate owner review','branch':'codex/memory-stage-4','frozen_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'files':files,'verification':{'focused_normal':'PASS: go test ./internal/eviedb ./cmd/evie -run TestOwnerReviewIdentity|TestOwnerReviewEncodingGolden -count=1; eviedb1.346s CLI0.703s before final Predicate label bound guard','broader':'PASS: go test ./internal/memory ./internal/eviedb ./internal/localextractor -run Compiler|OwnerReview -count=1; eviedb12.511s; other packages no matching tests','focused_race':'final rerun in progress session60925; prior eviedb13.105s CLI3.000s passed','whitespace':'PASS exact owned before/after git diff --no-index --check including new files; exit1 denotes differences, no whitespace diagnostic','gofmt':'PASS all owned Go files','full_required':'root runs ./scripts/verify-change.sh on isolated exact ticket tree','independent_review':'pending root two-axis review'},'notes':['No staging, commit, push, branch, model configuration, or live inference.','Original v1 candidate/request/preview/v6 bytes preserved with omitted extensions; golden v1 unchanged, new golden v2 added.','141-originals compiler_source.go includes final138 root-bound fix, excluding only141 compilerIdentityContext hook.','Saved137 originals compiler.go/compiler_validation.go match137 frozen files.','New identity choice revisions are the narrow binding-contract prerequisite; general editing/batches remain144.','Earlier combined race run CLI compile was transiently blocked by145 symbols; separate CLI rerun passed.']}
(root/'141-engineering-checkpoint.json').write_text(json.dumps(checkpoint,indent=2)+'\n')
(root/'141-owned.txt').write_text('\n'.join(sorted(paths))+'\n')
print(json.dumps({'files':len(files),'bytes':sum(f['bytes'] for f in files),'paths':[f['path'] for f in files]},indent=2))
