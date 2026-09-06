import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4';index=records/'139-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
base='8c14c7748eff2f594efa29f7a7d1348660dd2554';git('read-tree',base);owner=json.loads((records/'139-engineering-checkpoint.json').read_text());owned={}
entries=[(item,records/'139-frozen-files') for item in owner['files']]
for path, hashes in json.loads((records/'139-root-hooks.json').read_text())['files'].items():entries.append((dict(path=path,**hashes),records/'139-root-frozen-files'))
for item,directory in entries:
 path=item['path'];before=item['before_sha256'];data=(directory/path).read_bytes()
 if before and before!='absent':assert hashlib.sha256(git('show',f'{base}:{path}')).hexdigest()==before,path
 else:assert not git('ls-tree',base,'--',path),path
 assert hashlib.sha256(data).hexdigest()==item['after_sha256'],path
 owned[path]=hashlib.sha256(data).hexdigest();blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100644',blob,path)
tree=git('write-tree').decode().strip();record={'base':base,'tree':tree,'index':str(index),'files':owned,'note':'139 frozen owner and root hook files on final142 tree; excludes143 and preexisting user changes.'}
(records/'139-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n');(records/'139-checkpoint.patch').write_bytes(git('diff','--binary',base,tree));print(json.dumps({'base':base,'tree':tree,'files':len(owned)}))
