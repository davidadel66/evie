import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4';index=records/'147-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
base='97e9768e1d9ef26f44938f4b80de413a4ffda0cb';git('read-tree',base);owner=json.loads((records/'147-engineering-checkpoint.json').read_text());owned={}
for item in owner['files']:
 path=item['path'];before=item['before_sha256'];data=(records/'147-frozen-files'/path).read_bytes()
 if before and before!="absent":assert hashlib.sha256(git('show',f'{base}:{path}')).hexdigest()==before,path
 else:assert not git('ls-tree',base,'--',path),path
 assert hashlib.sha256(data).hexdigest()==item['after_sha256'],path
 owned[path]=hashlib.sha256(data).hexdigest();blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100644',blob,path)
tree=git('write-tree').decode().strip();record={'base':base,'tree':tree,'index':str(index),'files':owned,'note':'147 frozen generation/recurrence files on final146 tree; excludes148 and preexisting user changes.'}
(records/'147-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n');(records/'147-checkpoint.patch').write_bytes(git('diff','--binary',base,tree));print(json.dumps({'base':base,'tree':tree,'files':len(owned)}))
