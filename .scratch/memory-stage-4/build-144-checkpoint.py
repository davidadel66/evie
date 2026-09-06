import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4';index=records/'144-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
base='6bffb612dc6d1815bf30e6530670a95b32636aee';git('read-tree',base);owner=json.loads((records/'144-engineering-checkpoint.json').read_text());owned={}
contributions=[(owner['files'],records/'144-frozen-files'),(json.loads((records/'144-root-frozen-manifest.json').read_text()),records/'144-root-frozen-files')]
for manifest,directory in contributions:
 for item in manifest:
  path=item['path'];assert path not in owned,path
  before=item['before_sha256'];data=(directory/path).read_bytes()
  if before and before!='absent':assert hashlib.sha256(git('show',f'{base}:{path}')).hexdigest()==before,path
  else:assert not git('ls-tree',base,'--',path),path
  assert hashlib.sha256(data).hexdigest()==item['after_sha256'],path
  assert len(data)==item['bytes'],path
  owned[path]=hashlib.sha256(data).hexdigest();blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100644',blob,path)
tree=git('write-tree').decode().strip();record={'base':base,'tree':tree,'index':str(index),'files':owned,'note':'Unified144 owner, migration, record-bound and CLI frozen files plus root R05 clarification; excludes146 and pre-existing user changes.'}
(records/'144-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n');(records/'144-checkpoint.patch').write_bytes(git('diff','--binary',base,tree));print(json.dumps({'base':base,'tree':tree,'files':len(owned)}))
