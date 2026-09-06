import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4';index=records/'148-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
base=json.loads((records/'147-checkpoint.json').read_text())['tree'];git('read-tree',base);owner=json.loads((records/'148-engineering-checkpoint.json').read_text());owned={}
owner['files'] += json.loads((records/'148-root-frozen-manifest.json').read_text())
for item in owner['files']:
 path=item['path'];before=item['before_sha256'];data=(records/('148-root-frozen-files' if path in ['internal/eviedb/db.go','cmd/evie/main.go','internal/web/serve.go'] else '148-frozen-files')/path).read_bytes()
 if before and before!="absent":assert hashlib.sha256(git('show',f'{base}:{path}')).hexdigest()==before,path
 else:assert not git('ls-tree',base,'--',path),path
 assert hashlib.sha256(data).hexdigest()==item['after_sha256'],path
 owned[path]=hashlib.sha256(data).hexdigest();blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100644',blob,path)
tree=git('write-tree').decode().strip();record={'base':base,'tree':tree,'index':str(index),'files':owned,'note':'148 diagnostics, bounded legacy status, HTTP/UI and narrow root hooks on the frozen147 tree; excludes preexisting user changes.'}
(records/'148-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n');(records/'148-checkpoint.patch').write_bytes(git('diff','--binary',base,tree));print(json.dumps({'base':base,'tree':tree,'files':len(owned)}))
