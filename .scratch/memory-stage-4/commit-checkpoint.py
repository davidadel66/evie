import json,os,pathlib,subprocess,sys
root=pathlib.Path(__file__).resolve().parents[2];records=root/'.scratch/memory-stage-4';ticket=sys.argv[1];checkpoint=json.loads((records/f'{ticket}-checkpoint.json').read_text());message=records/f'{ticket}-commit-message.txt'
assert checkpoint.get('verification_exit_code')==0,'Full isolated verification required.'
def git(*args,env=None):return subprocess.check_output(['git',*args],cwd=root,env=env)
assert git('branch','--show-current').decode().strip()=='codex/memory-stage-4'
subprocess.run(['git','diff','--cached','--quiet'],cwd=root,check=True)
old=git('rev-parse','HEAD').decode().strip();index=records/f'{ticket}-commit.index';env=dict(os.environ,GIT_INDEX_FILE=str(index));git('read-tree',old,env=env)
for path in sorted(checkpoint['files']):
 before=git('ls-tree',checkpoint['base'],'--',path)
 current=git('ls-tree',old,'--',path)
 assert before==current,f'Prior committed changes overlap ticket {ticket}: {path}'
 line=git('ls-tree',checkpoint['tree'],'--',path).decode().strip();assert line,path
 metadata,actual=line.split('\t',1);mode,kind,blob=metadata.split();assert actual==path and kind=='blob'
 git('update-index','--add','--cacheinfo',mode,blob,path,env=env)
tree=git('write-tree',env=env).decode().strip()
subprocess.run(['git','diff','--check',old,tree],cwd=root,check=True)
assert git('rev-parse','HEAD').decode().strip()==old
subprocess.run(['git','diff','--cached','--quiet'],cwd=root,check=True)
with (records/f'{ticket}-commit.log').open('w') as log:
 result=subprocess.run(['git','commit','-F',str(message)],cwd=root,env=env,stdout=log,stderr=subprocess.STDOUT)
if result.returncode:
 print('\n'.join((records/f'{ticket}-commit.log').read_text().splitlines()[-35:]));raise SystemExit(result.returncode)
new=git('rev-parse','HEAD').decode().strip();assert git('rev-parse',new+'^{tree}').decode().strip()==tree
# Fast-forward only the index, using Git's two-tree merge to preserve or reject
# concurrent staged edits. No working-tree update and no reset/discard.
subprocess.run(['git','read-tree','-i','-m',old,new],cwd=root,check=True)
record={'ticket':int(ticket),'parent':old,'commit':new,'tree':tree,'isolated_tree':checkpoint['tree'],'files':len(checkpoint['files']),'issue_closed':False,'note':'Commit records verified engineering; acceptance/model/human gates remain separately tracked.'}
(records/f'{ticket}-commit.json').write_text(json.dumps(record,indent=2)+'\n');print(json.dumps(record))
