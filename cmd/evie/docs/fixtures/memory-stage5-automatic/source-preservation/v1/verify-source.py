#!/usr/bin/env python3
"""Verify source supplements against immutable original source inventories; no build or model calls."""
import argparse,hashlib,json,pathlib,tarfile

def sha(data):return hashlib.sha256(data).hexdigest()
def verify(directory):
 directory=pathlib.Path(directory).resolve();repo=next(p for p in directory.parents if (p/'go.mod').is_file());reports=[]
 for mp in sorted(directory.glob('*-source-manifest.json')):
  m=json.loads(mp.read_text());ip=directory/m['declared_inventory'];assert sha(ip.read_bytes())==m['declared_inventory_sha256']
  inv=json.loads(ip.read_text());assert inv['missing']==[] and inv['verified_entries']==inv['declared_entries']==len(inv['entries'])
  references={}
  for name,digest in m['immutable_run_metadata'].items():
   p=repo/name;assert sha(p.read_bytes())==digest,name;references[p.name]=json.loads(p.read_text())
  f=references['freeze.json'];meta=references.get('run-metadata.json',{})
  if 'source_sha256' in f:declared=f['source_sha256']
  elif 'production_files_sha256' in f:declared=dict(f['production_files_sha256'])
  else:declared={e['path']:e['sha256'] for e in meta['production_go_files']}
  declared.update({'internal/agent/'+name:digest for name,digest in f.get('fixture_files_sha256',{}).items()})
  if 'test_sha256' in f:declared['internal/agent/retrieval_reader_evaluation_test.go']=f['test_sha256']
  if 'adapter_sha256' in f:declared['internal/agent/retrieval_production_reader_evaluation_test.go']=f['adapter_sha256']
  assert declared=={e['path']:e['sha256'] for e in inv['entries']},m['label']
  ap=directory/m['archive'];assert sha(ap.read_bytes())==m['archive_sha256'];assert ap.stat().st_size==m['archive_bytes']
  with tarfile.open(ap,'r:gz') as tf:
   assert sorted(tf.getnames())==sorted(e['path'] for e in m['members'])
   for e in m['members']:
    info=tf.getmember(e['path']);assert info.isfile() and not pathlib.PurePosixPath(info.name).is_absolute() and '..' not in pathlib.PurePosixPath(info.name).parts
    data=tf.extractfile(info).read();assert sha(data)==e['sha256'] and len(data)==e['bytes'];assert info.mode==e['mode']
    # PAX stores original floating mtime while exact observed ns remains in the manifest.
    assert abs(float(info.mtime)-e['origin_mtime_ns']/1e9)<1e-6
    if e['frozen_hash_declared']:assert declared[e['path']]==e['sha256']
  assert len(m['members'])==m['source_file_count'];assert sum(e['bytes'] for e in m['members'])==m['uncompressed_source_bytes']
  assert m['local_go_embed_directives']==[]
  reports.append({'label':m['label'],'source_files_verified':len(m['members']),'declared_inventory_entries_matched_to_original_freeze':len(declared),'archive_sha256':m['archive_sha256'],'archive_bytes':m['archive_bytes'],'complete':True})
 assert reports,'no manifests found'
 return {'directory':str(directory),'source_archives':reports,'verified':True,'no_build_or_model_calls':True}
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('directory',nargs='?',default=str(pathlib.Path(__file__).parent));args=p.parse_args();print(json.dumps(verify(args.directory),indent=2))
