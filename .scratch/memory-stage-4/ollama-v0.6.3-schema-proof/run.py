#!/usr/bin/env python3
"""Compile and test pinned Ollama schema grammar without loading a model.

Requires Python 3, Apple clang++ and its SDK. On first run only, downloads the
small source files listed in sources.json from their immutable commit URLs.
Existing cache files are hash checked; compilation and testing are offline.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shlex
import subprocess
from urllib.request import urlopen

ROOT = Path(__file__).resolve().parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--schema', type=Path, required=True)
parser.add_argument('--cache', type=Path, required=True)
parser.add_argument('--output', type=Path)
parser.add_argument('--offline', action='store_true', help='Fail if a pinned source is not cached')
args = parser.parse_args()
manifest = json.loads((ROOT / 'sources.json').read_text())
schema = args.schema.resolve()
cache = args.cache.resolve()
output = (args.output or cache).resolve()
source = cache / 'source'
source.mkdir(parents=True, exist_ok=True)
output.mkdir(parents=True, exist_ok=True)
if hashlib.sha256(schema.read_bytes()).hexdigest() != manifest['tested_schema_sha256']:
    raise SystemExit('Input schema differs from the frozen tested schema SHA-256')
for entry in manifest['sources']:
    path = source / entry['local_name']
    if not path.exists():
        if args.offline:
            raise SystemExit(f'Missing offline source: {path}')
        data = urlopen(entry['url'], timeout=30).read()
        if hashlib.sha256(data).hexdigest() != entry['sha256']:
            raise SystemExit(f'Downloaded source hash mismatch: {path}')
        path.write_bytes(data)
    if hashlib.sha256(path.read_bytes()).hexdigest() != entry['sha256']:
        raise SystemExit(f'Cached source hash mismatch: {path}')

# Check that all three string helpers are exact copies of the pinned source.
common = (source / 'common.cpp').read_text()
helpers = (ROOT / 'link-support.cpp').read_text()
for begin, end in [
    ('std::string string_join(', '\nstd::vector<std::string> string_split('),
    ('std::vector<std::string> string_split(', '\nstd::string string_repeat('),
    ('std::string string_repeat(', '\nstd::string string_from('),
]:
    if common[common.index(begin):common.index(end)] not in helpers:
        raise SystemExit(f'Copied helper differs from pinned source: {begin}')

build = [
    'clang++', '-std=c++17', '-O2', '-ffunction-sections', '-fdata-sections',
    '-I' + str(source), str(source / 'json-schema-to-grammar.cpp'),
    str(source / 'llama-grammar.cpp'), str(ROOT / 'link-support.cpp'),
    str(ROOT / 'proof.cpp'), '-Wl,-dead_strip', '-o', str(cache / 'proof'),
]
(output / 'build-command.txt').write_text(shlex.join(build) + '\n')
version = subprocess.run(['clang++', '--version'], text=True, capture_output=True, check=True)
(output / 'compiler-version.txt').write_text(version.stdout)
compiled = subprocess.run(build, text=True, capture_output=True)
(output / 'build-output.txt').write_text(compiled.stdout + compiled.stderr)
if compiled.returncode:
    raise SystemExit(compiled.stderr)

full = subprocess.run([
    str(cache / 'proof'), str(schema), str(output / 'output.gbnf'),
    str(ROOT / 'cases.json'),
], text=True, capture_output=True)
(output / 'results.txt').write_text(full.stdout)
(output / 'test-stderr.txt').write_text(full.stderr)
if full.returncode:
    raise SystemExit(full.stdout + full.stderr)

schema_obj = json.loads(schema.read_text())
reference = schema_obj['properties']['candidates']['items']['properties']['sources']['items']
reference_schema = output / 'reference.schema.json'
reference_schema.write_text(json.dumps(reference, sort_keys=True, separators=(',', ':')) + '\n')
subprocess.run([
    str(cache / 'proof'), str(reference_schema), str(output / 'reference.gbnf'),
], check=True)
artifacts = {}
for name in ['output.gbnf', 'reference.gbnf', 'reference.schema.json', 'results.txt']:
    data = (output / name).read_bytes()
    artifacts[name] = {'sha256': hashlib.sha256(data).hexdigest(), 'bytes': len(data)}
(output / 'artifacts.json').write_text(json.dumps(artifacts, indent=2) + '\n')
print(f"PASS: {len(json.loads((ROOT / 'cases.json').read_text()))} grammar cases")
print(f"Full grammar: {artifacts['output.gbnf']['bytes']} bytes")
print(f'Output: {output}')
