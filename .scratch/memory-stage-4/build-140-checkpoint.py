import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[2]
records=root/'.scratch/memory-stage-4'
parent=json.loads((records/'136-checkpoint.json').read_text())['tree']
assert parent=='b444a6e81510591b131b83ca40d8d74ee810ed92','Unexpected #136 tree; root must explicitly review the changed parent.'
index=records/'140-checkpoint.index'
env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
git('read-tree',parent)
checkpoint=json.loads((records/'140-engineering-checkpoint.json').read_text())
owned={}
for item in checkpoint['new_owned_files']+checkpoint['owned_tracked_hook_files']:
 path=item['path'];data=(root/path).read_bytes()
 assert hashlib.sha256(data).hexdigest()==item['sha256'].removeprefix('sha256:'),path
 owned[path]=data
hooks={
 'cmd/evie/main.go':('\tif handled, err := runOwnerReviewManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {\n\t\tif err != nil {\n\t\t\tlog.Fatalf("memory review: %v", err)\n\t\t}\n\t\treturn\n\t}\n','\tif _, err := kernelStore.ImportDefaultLegacyTodoList(context.Background()); err != nil {\n'),
 'internal/eviedb/db.go':('\tif err := ensureCandidateReviewSchema(ctx, db); err != nil {\n\t\tdb.Close()\n\t\treturn nil, fmt.Errorf("create candidate review schema: %w", err)\n\t}\n','\tif err := checkSemanticProjectionStartup(ctx, db); err != nil {\n')
}
for path,(block,anchor) in hooks.items():
 current=(root/path).read_text();before=git('show',f'{parent}:{path}').decode()
 assert current.count(block)==1,path
 assert before.count(anchor)==1 and block not in before,path
 owned[path]=before.replace(anchor,block+anchor,1).encode()
for path,data in sorted(owned.items()):
 blob=git('hash-object','-w','--stdin',data=data).decode().strip()
 git('update-index','--add','--cacheinfo','100644',blob,path)
tree=git('write-tree').decode().strip()
record={'base':parent,'tree':tree,'index':str(index),'files':{p:hashlib.sha256(b).hexdigest() for p,b in sorted(owned.items())},'note':'#140 independent review tree layered only on frozen verified #136; excludes #137 and preexisting user changes.'}
(records/'140-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n')
(records/'140-checkpoint.patch').write_bytes(git('diff','--binary',parent,tree))
print(json.dumps({'base':parent,'tree':tree,'files':len(owned)}))
