from pathlib import Path
import json,subprocess,tempfile,shutil
p=Path('.scratch/memory-stage-4');x=json.loads((p/'149-startup-probe-location.json').read_text());binary=str(Path(x['probe'])/'probe');subprocess.run(['go','build','-o',binary,'.'],cwd=x['probe'],check=True);rows=[]
for trial in range(40):
 directory=Path(tempfile.mkdtemp(prefix='evie-startup-retry-'));db=directory/'probe.db'
 try:
  for phase in ['fresh','existing']:
   procs=[subprocess.Popen([binary,str(db),'public'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True) for _ in range(4)]
   for proc in procs:proc.stdin.write('x');proc.stdin.flush()
   for n,proc in enumerate(procs):
    out,err=proc.communicate(timeout=25);result=json.loads(out) if out else {'error':err};rows.append({'trial':trial,'phase':phase,'process':n,'exit_code':proc.returncode,**result})
 finally:shutil.rmtree(directory)
fails=[r for r in rows if r['exit_code']];(p/'149-startup-retry-results.json').write_text(json.dumps({'base_tree':'fae9bdd86e9e0f0f2f42bac18d0cbc063684412c','prototype_only':True,'rows':rows},indent=2)+'\n');print(json.dumps({'runs':len(rows),'failures':len(fails),'examples':fails[:5]}))
