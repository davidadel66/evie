import hashlib, json, os, pathlib, re, subprocess, tempfile
root = pathlib.Path(__file__).resolve().parents[2]
# Script lives in <repo>/.scratch/memory-stage-4.
assert (root / 'go.mod').exists()
record_dir = root / '.scratch/memory-stage-4'
index = record_dir / '136-checkpoint.index'
env = dict(os.environ, GIT_INDEX_FILE=str(index))
def git(*args, data=None):
    return subprocess.check_output(['git', *args], cwd=root, env=env, input=data)
base = git('rev-parse','HEAD').decode().strip()
git('read-tree', base)
owner = json.loads((record_dir/'136-engineering-checkpoint.json').read_text())
paths = sorted(owner['owned_files'])
owned = {name: (root/name).read_bytes() for name in paths}
for name, content in owned.items():
    assert hashlib.sha256(content).hexdigest() == owner['owned_files'][name], name
for name, function in [('cmd/evie/main.go','runCompilerManagement'),('internal/eviedb/db.go','ensureCompilerSchema')]:
    current = (root/name).read_text()
    original = git('show', f'{base}:{name}').decode()
    if function == 'runCompilerManagement':
        block = '\tif handled, err := runCompilerManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {\n\t\tif err != nil {\n\t\t\tlog.Fatalf("memory compiler: %v", err)\n\t\t}\n\t\treturn\n\t}\n'
        anchor = '\tkernelStore := eviedb.NewStore(db)\n'
    else:
        block = '\tif err := ensureCompilerSchema(ctx, db); err != nil {\n\t\tdb.Close()\n\t\treturn nil, fmt.Errorf("create memory compiler schema: %w", err)\n\t}\n'
        anchor = '\tif err := checkSemanticProjectionStartup(ctx, db); err != nil {\n'
    assert current.count(block) == 1, name
    assert original.count(anchor) == 1, name
    owned[name] = original.replace(anchor, anchor+block if function == 'runCompilerManagement' else block+anchor, 1).encode()
for name, content in sorted(owned.items()):
    blob = git('hash-object','-w','--stdin',data=content).decode().strip()
    git('update-index','--add','--cacheinfo','100644',blob,name)
tree = git('write-tree').decode().strip()
record = {'base':base,'tree':tree,'index':str(index),'files':{name:hashlib.sha256(data).hexdigest() for name,data in sorted(owned.items())},'note':'Independent #136 engineering tree; excludes #140 hooks, user main.go hunk, and all #135 artifacts. Exact selected-wire/live observation acceptance remains pending.'}
(record_dir/'136-checkpoint.json').write_text(json.dumps(record,indent=2)+'\n')
(record_dir/'136-checkpoint.patch').write_bytes(git('diff','--binary',base,tree))
print(json.dumps({'base':base,'tree':tree,'files':len(owned)}))
