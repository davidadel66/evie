import hashlib,json,os,pathlib,subprocess,tarfile,io,tempfile
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4';index=records/'149-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
base='9d697bf57c7835d7522da391c44163e15f416189';git('read-tree',base);owned={}
for manifest,folder in [('149-owner-refined-frozen-manifest.json','149-owner-refined-frozen-files'),('149-root-frozen-manifest.json','149-root-frozen-files')]:
 entries=json.loads((records/manifest).read_text());entries=entries['files'] if isinstance(entries,dict) else entries
 for item in entries:
  path=item['path'];assert path not in owned,path;before=item['before_sha256'];data=(records/folder/path).read_bytes()
  if before and before!='absent':assert hashlib.sha256(git('show',f'{base}:{path}')).hexdigest()==before,path
  else:assert not git('ls-tree',base,'--',path),path
  assert hashlib.sha256(data).hexdigest()==item['after_sha256'],path
  owned[path]=item['after_sha256'];blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100755' if item.get('executable') else '100644',blob,path)
tree=git('write-tree').decode().strip();snapshot=pathlib.Path(tempfile.mkdtemp(prefix='evie-149-checkpoint-'))
with tarfile.open(fileobj=io.BytesIO(git('archive',tree))) as t:t.extractall(snapshot,filter='data')
(snapshot/'internal/web/ui/node_modules').symlink_to(root/'internal/web/ui/node_modules',target_is_directory=True)
r={'base':base,'tree':tree,'index':str(index),'files':owned,'snapshot':str(snapshot),'note':'149 conformance and physical connection startup retry on frozen148; excludes150/151 and preexisting user work.'}
(records/'149-checkpoint.json').write_text(json.dumps(r,indent=2)+'\n');(records/'149-checkpoint.patch').write_bytes(git('diff','--binary',base,tree));print(json.dumps(r))
