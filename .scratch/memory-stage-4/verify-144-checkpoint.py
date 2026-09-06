import json,pathlib,subprocess,tempfile,os,tarfile,io
root=pathlib.Path(__file__).resolve().parents[2]
record_path=root/'.scratch/memory-stage-4/144-checkpoint.json'
r=json.loads(record_path.read_text())
snapshot=pathlib.Path(tempfile.mkdtemp(prefix='evie-144-checkpoint-'))
archive=subprocess.check_output(['git','archive',r['tree']],cwd=root)
with tarfile.open(fileobj=io.BytesIO(archive)) as t: t.extractall(snapshot,filter='data')
(snapshot/'internal/web/ui/node_modules').symlink_to(root/'internal/web/ui/node_modules',target_is_directory=True)
gitdir=subprocess.check_output(['git','rev-parse','--absolute-git-dir'],cwd=root,text=True).strip()
env=dict(os.environ,GIT_DIR=gitdir,GIT_WORK_TREE=str(snapshot),GIT_INDEX_FILE=r['index'])
p=root/'.scratch/memory-stage-4/verify-144-isolated.log'
r['snapshot']=str(snapshot);r['verification_log']=str(p);record_path.write_text(json.dumps(r,indent=2)+'\n')
print('Verifying isolated #144 tree '+r['tree']+' in '+str(snapshot),flush=True)
with p.open('w') as f: result=subprocess.run(['./scripts/verify-change.sh'],cwd=snapshot,env=env,stdout=f,stderr=subprocess.STDOUT)
r['verification_exit_code']=result.returncode;record_path.write_text(json.dumps(r,indent=2)+'\n')
print('Verification exit:',result.returncode,flush=True);print('\n'.join(p.read_text().splitlines()[-25:]),flush=True)
raise SystemExit(result.returncode)
