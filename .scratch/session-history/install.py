from pathlib import Path
import datetime,hashlib,json,os,shutil,signal,sqlite3,subprocess,time,urllib.request
repo=Path('/Users/davidboktor/code/evie'); runtime=Path('/Users/davidboktor/.evie');binary=Path('/Users/davidboktor/go/bin/evie')
stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');backup=runtime/'backups'/('before-session-history-'+stamp);backup.mkdir(mode=0o700)
shutil.copy2(binary,backup/'evie');shutil.copy2(runtime/'.env',backup/'.env')
src=sqlite3.connect('file:'+str(runtime/'evie.db')+'?mode=ro',uri=True);dst=sqlite3.connect(backup/'evie.db');src.backup(dst);dst.close();src.close();os.chmod(backup/'evie.db',0o600)
oldPID=json.loads((repo/'.scratch/session-history/runtime.json').read_text())['pid']
os.kill(oldPID,signal.SIGTERM)
for _ in range(100):
 try:os.kill(oldPID,0)
 except ProcessLookupError:break
 time.sleep(.1)
else:raise RuntimeError('old server did not stop')
replacement=binary.with_name('evie-session-history-new');shutil.copy2(repo/'.scratch/session-history/evie',replacement);os.replace(replacement,binary)
logpath=runtime/'logs'/('serve-session-history-'+stamp+'.log');fd=os.open(logpath,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
with os.fdopen(fd,'wb') as log:process=subprocess.Popen([str(binary),'serve'],cwd=runtime,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
record={'backup':str(backup),'pid':process.pid,'log':str(logpath),'sha256':hashlib.sha256(binary.read_bytes()).hexdigest()}
(repo/'.scratch/session-history/runtime.json').write_text(json.dumps(record))
for _ in range(100):
 if process.poll() is not None:raise RuntimeError('server exited; inspect private log locally')
 try:
  with urllib.request.urlopen('http://127.0.0.1:6687/',timeout=1) as response:
   if response.status==200:print(json.dumps(record));break
 except OSError:pass
 time.sleep(.1)
else:raise RuntimeError('server not ready')
