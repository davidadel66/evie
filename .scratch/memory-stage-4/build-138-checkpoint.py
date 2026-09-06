import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4';index=records/'138-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
base='0138e394fbe763f805eb198206ce0d8c0a4d41d4';git('read-tree',base)
worker=json.loads((records/'137-checkpoint.json').read_text())
assert worker.get('verification_exit_code')==0
for path in worker['files']:
 assert git('ls-tree',worker['base'],'--',path)==git('ls-tree',base,'--',path),path
 meta=git('ls-tree',worker['tree'],'--',path).decode().split('\t')[0].split();git('update-index','--add','--cacheinfo',meta[0],meta[2],path)
parent=git('write-tree').decode().strip();owner=json.loads((records/'138-engineering-checkpoint.json').read_text());owned={}
for item in owner['files']:
 path=item['path'];before=item['before_sha256'];data=(records/'138-frozen-files'/path).read_bytes()
 if before:assert hashlib.sha256(git('show',f'{parent}:{path}')).hexdigest()==before,path
 else:assert not git('ls-tree',parent,'--',path),path
 assert hashlib.sha256(data).hexdigest()==item['after_sha256'],path
 owned[path]=data
for path,h in json.loads((records/'138-root-hooks.json').read_text()).items():
 data=(records/'138-root-frozen-files'/path).read_bytes();assert hashlib.sha256(data).hexdigest()==h,path;owned[path]=data
for path,data in owned.items():
 blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100644',blob,path)
tree=git('write-tree').decode().strip();record={'base':parent,'tree':tree,'index':str(index),'files':{p:hashlib.sha256(d).hexdigest() for p,d in owned.items()},'note':'138 frozen files and root hooks; predecessor tree contains committed135/136/140 and verified137. Excludes141 and user UI/database changes.'}
(records/'138-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n');(records/'138-checkpoint.patch').write_bytes(git('diff','--binary',parent,tree));print(json.dumps({'base':parent,'tree':tree,'files':len(owned)}))
