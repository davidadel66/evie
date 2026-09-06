import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4'
base='a41aa6189191ed1ca3cdaa53e496269db489085d';index=records/'135-checkpoint.index';env=dict(os.environ,GIT_INDEX_FILE=str(index))
def git(*args,data=None):return subprocess.check_output(['git',*args],cwd=root,env=env,input=data)
git('read-tree',base)
files=[p for d in ['scripts/memory-extractor-spike','cmd/evie/docs/fixtures/memory-stage-4-spike'] for p in (root/d).rglob('*') if p.is_file()]
files.append(root/'cmd/evie/docs/research/memory-stage-4-local-extractor-spike.md')
old=json.loads((records/'implementation-baseline.json').read_text())['preexisting_paths'];owned={}
for p in files:
 path=str(p.relative_to(root));assert path not in old,path;assert not p.is_symlink(),path
 data=p.read_bytes();owned[path]=hashlib.sha256(data).hexdigest()
 blob=git('hash-object','-w','--stdin',data=data).decode().strip();git('update-index','--add','--cacheinfo','100644',blob,path)
tree=git('write-tree').decode().strip();record={'base':base,'tree':tree,'index':str(index),'files':owned,'note':'#135 standalone110-comparison engineering/evaluation snapshot. Actual output judgments pending; no adequate production extractor selected. Excludes all user files and #136/#140 production code.'}
(records/'135-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n');(records/'135-checkpoint.patch').write_bytes(git('diff','--binary',base,tree));print(json.dumps({'base':base,'tree':tree,'files':len(owned)}))
