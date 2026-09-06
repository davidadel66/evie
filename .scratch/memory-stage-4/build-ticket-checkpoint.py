import hashlib,json,os,pathlib,subprocess,tarfile,io,tempfile,sys
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4'
ticket,base,manifest,folder,tag=sys.argv[1:];index=records/f'{tag}-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
git('read-tree',base);owned={};entries=json.loads((records/manifest).read_text());entries=entries['files'] if isinstance(entries,dict) else entries
for item in entries:
 path=item['path'];assert path not in owned,path;before=item.get('before_sha256');data=(records/folder/path).read_bytes()
 if before and before!='absent':assert hashlib.sha256(git('show',f'{base}:{path}')).hexdigest()==before,path
 else:assert not git('ls-tree',base,'--',path),path
 assert hashlib.sha256(data).hexdigest()==item['after_sha256'],path
 owned[path]=item['after_sha256'];blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100755' if item.get('executable') else '100644',blob,path)
tree=git('write-tree').decode().strip();snapshot=pathlib.Path(tempfile.mkdtemp(prefix=f'evie-{tag}-checkpoint-'))
with tarfile.open(fileobj=io.BytesIO(git('archive',tree))) as archive:archive.extractall(snapshot,filter='data')
(snapshot/'internal/web/ui/node_modules').symlink_to(root/'internal/web/ui/node_modules',target_is_directory=True)
result={'ticket':int(ticket),'base':base,'tree':tree,'index':str(index),'files':owned,'snapshot':str(snapshot),'note':'Isolated ticket contribution; preexisting user work excluded.'}
(records/f'{tag}-checkpoint.json').write_text(json.dumps(result,indent=2)+'\n');(records/f'{tag}-checkpoint.patch').write_bytes(git('diff','--binary',base,tree));print(json.dumps({'tree':tree,'snapshot':str(snapshot),'files':len(owned)}))
