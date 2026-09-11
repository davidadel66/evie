from pathlib import Path
import subprocess,os,json,hashlib,re
root=Path('/Users/davidboktor/code/evie-memory-stage-5');base=Path('/tmp/evie-memory-stage5')
def raw(*a,data=None,env=None): return subprocess.check_output(['git',*a],cwd=root,input=data,env=env)
def git(*a,data=None,env=None): return raw(*a,data=data,env=env).decode().strip()
head=git('rev-parse','HEAD');assert head=='52d00bf5aa40c6323321253b54e66bf153de7ccd'
chain=git('rev-list','--reverse','f27546d..'+head).splitlines();assert len(chain)==12
issues=[int(re.search(r'\(#(\d+)\)$',git('log','-1','--format=%s',c)).group(1)) for c in chain]
assert len(set(issues))==12 and set(issues)==set(range(156,168))
assert not git('diff','--cached','--name-only')
paths=json.loads((base/'164-165-preservation-staging-manifest.json').read_text())
for issue,group in json.loads((base/'reader-source-preservation-owner-manifest.json').read_text()).items():
 for item in group['files']:
  name=item['path'];assert name not in paths
  paths[name]=dict(sha256=item['sha256'],bytes=item['bytes'],issue=int(issue))
assert len(paths)==41
blobs={}
for name,item in paths.items():
 data=(root/name).read_bytes();assert hashlib.sha256(data).hexdigest()==item['sha256'] and len(data)==item['bytes']
 first=issues.index(item['issue']); tracked=bool(git('ls-tree',head,name))
 for c in chain[first:]:
  if tracked:assert raw('show',c+':'+name)==raw('show',head+':'+name)
  else:assert not git('ls-tree',c,name)
 blobs[name]=git('hash-object','-w','--stdin',data=data)
first=min(issues.index(x['issue']) for x in paths.values());nextparent=git('rev-parse',chain[first]+'^');mapping={}
for i,c in enumerate(chain[first:],start=first):
 index=base/('source-preservation-fold-'+str(i)+'.index');assert not index.exists()
 env=os.environ.copy();env['GIT_INDEX_FILE']=str(index);git('read-tree',c,env=env)
 included={name for name,item in paths.items() if issues.index(item['issue'])<=i}
 for name in included:git('update-index','--add','--cacheinfo','100644,'+blobs[name]+','+name,env=env)
 tree=git('write-tree',env=env)
 author=git('log','-1','--format=%an%x00%ae%x00%aI',c).split('\0');env.update(GIT_AUTHOR_NAME=author[0],GIT_AUTHOR_EMAIL=author[1],GIT_AUTHOR_DATE=author[2])
 updated=git('commit-tree',tree,'-p',nextparent,data=raw('log','-1','--format=%B',c),env=env)
 assert set(git('diff','--name-only',c,updated).splitlines())==included
 mapping[c]=updated;nextparent=updated
for name,item in paths.items():assert hashlib.sha256((root/name).read_bytes()).hexdigest()==item['sha256']
manifest=json.loads((base/'integrated-development-v4/compiled-source-manifest.json').read_text())
for name,expected in manifest.items():
 assert hashlib.sha256((root/name).read_bytes()).hexdigest()==expected
 assert hashlib.sha256(raw('show',nextparent+':'+name)).hexdigest()==expected
branch=git('symbolic-ref','HEAD');assert branch=='refs/heads/codex/memory-stage-5';assert not git('diff','--cached','--name-only')
git('update-ref','refs/codex/memory-stage5-pre-source-preservation-fold',head)
git('update-ref',branch,nextparent,head);git('read-tree',nextparent)
assert len(git('rev-list','f27546d..HEAD').splitlines())==12
record=dict(mapping=mapping,head=nextparent,commit_count=12,owning_issues={str(issues[i]):mapping.get(c,c) for i,c in enumerate(chain)},paths=paths,compiled_inputs_equal_frozen_and_committed=len(manifest),preserved_uncommitted_issue167_and_168=True)
(base/'source-preservation-owner-fold.json').write_text(json.dumps(record,indent=2)+'\n');print(json.dumps({k:v for k,v in record.items() if k!='paths'},indent=2))
