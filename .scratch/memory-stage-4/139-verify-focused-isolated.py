from pathlib import Path
import subprocess,tarfile,tempfile,json,shutil
root=Path.cwd();scratch=root/'.scratch/memory-stage-4';record=json.loads((scratch/'139-engineering-checkpoint.json').read_text())
target=Path(tempfile.mkdtemp(prefix='evie-139-focused-'));archive=target/'source.tar'
with archive.open('wb') as f:subprocess.run(['git','archive',record['base']],stdout=f,check=True)
with tarfile.open(archive) as f:f.extractall(target,filter='data')
archive.unlink()
for entry in record['files']:
 p=entry['path'];dest=target/p;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(scratch/'139-frozen-files'/p,dest)
for p in ['cmd/evie/main.go','internal/eviedb/db.go']:shutil.copyfile(scratch/'139-root-frozen-files'/p,target/p)
(target/'internal/web/ui/node_modules').symlink_to(root/'internal/web/ui/node_modules',target_is_directory=True)
build=subprocess.run(['npm','--prefix','internal/web/ui','run','build'],cwd=target,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
(scratch/'139-ui-build-isolated.log').write_text(build.stdout)
if build.returncode:print(build.stdout);raise SystemExit(build.returncode)
results=[]
for name,args in [('normal',['go','test','./internal/eviedb','./cmd/evie','-run','^TestCompiler(History|Activation|Worker|Intervals)','-count=1']),('race',['go','test','-race','./internal/eviedb','./cmd/evie','-run','^TestCompilerHistory','-count=1'])]:
 r=subprocess.run(args,cwd=target,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 log=scratch/('139-'+name+'-isolated.log');log.write_text('$ '+' '.join(args)+'\n'+r.stdout)
 results.append({'command':args,'cwd':str(target),'exit_code':r.returncode,'log':str(log),'output':r.stdout})
 print(r.stdout,flush=True)
 if r.returncode:break
(scratch/'139-isolated-focused-verification.json').write_text(json.dumps(results,indent=2)+'\n')
print('snapshot',target,flush=True)
