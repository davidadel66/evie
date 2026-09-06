import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4';base='b444a6e81510591b131b83ca40d8d74ee810ed92';index=records/'137-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
git('read-tree',base);owner=json.loads((records/'137-engineering-checkpoint.json').read_text());owned={}
for path,h in owner['modified_originals'].items():assert hashlib.sha256(git('show',f'{base}:{path}')).hexdigest()==h,path
for path,h in owner['owned_files'].items():
 data=(records/'137-frozen-files'/path).read_bytes();assert hashlib.sha256(data).hexdigest()==h,path;owned[path]=h
 blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100644',blob,path)
tree=git('write-tree').decode().strip();record={'base':base,'tree':tree,'index':str(index),'files':owned,'note':'Independent137 on frozen136; excludes140,138 and user changes. Uses frozen owner bytes, not active shared files.'}
(records/'137-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n');(records/'137-checkpoint.patch').write_bytes(git('diff','--binary',base,tree));print(json.dumps({'base':base,'tree':tree,'files':len(owned)}))
