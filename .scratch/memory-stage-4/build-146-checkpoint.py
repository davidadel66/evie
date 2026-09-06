import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4';index=records/'146-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
base='1151a945eab80163516bdbeecb07c8d77f1a1236';git('read-tree',base);owner=json.loads((records/'146-engineering-checkpoint.json').read_text());owned={}
for item in owner['files']:
 path=item['path'];before=item['before_sha256'];data=(records/'146-frozen-files'/path).read_bytes()
 if before and before!="absent":assert hashlib.sha256(git('show',f'{base}:{path}')).hexdigest()==before,path
 else:assert not git('ls-tree',base,'--',path),path
 assert hashlib.sha256(data).hexdigest()==item['after_sha256'],path
 owned[path]=hashlib.sha256(data).hexdigest();blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100644',blob,path)
tree=git('write-tree').decode().strip();record={'base':base,'tree':tree,'index':str(index),'files':owned,'note':'146 frozen web files on final144 tree; excludes147 and preexisting user changes.'}
(records/'146-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n');(records/'146-checkpoint.patch').write_bytes(git('diff','--binary',base,tree));print(json.dumps({'base':base,'tree':tree,'files':len(owned)}))
