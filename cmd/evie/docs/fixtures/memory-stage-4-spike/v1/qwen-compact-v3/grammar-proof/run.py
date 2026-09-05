#!/usr/bin/env python3
"""Test the actual v3 predispatch schemas with pinned Ollama converter/parser.

Uses the unchanged v2 source-proof compiler harness and its immutable source
manifest. No model/server/inference is used. --offline forbids source fetches.
Outputs go only to the caller's scratch directory, never the frozen fixtures.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parent
base = root.parent
v2 = base.parent / 'qwen-compact-v2'
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--cache', type=Path, required=True)
p.add_argument('--output', type=Path, required=True)
p.add_argument('--offline', action='store_true')
a = p.parse_args()
cache, output = a.cache.resolve(), a.output.resolve()
if any(path == base.parent or base.parent in path.parents for path in (cache, output)):
    p.error('proof cache/output must be outside all frozen fixture subtrees')
output.mkdir(parents=True, exist_ok=True)
# Rebuild from exact hash-checked upstream sources; the inherited 62 v2 cases
# separately prove the unchanged closed-reference alternatives.
command = [sys.executable, str(v2 / 'grammar-proof/run.py'), '--schema',
           str(v2 / 'output.schema.json'), '--cache', str(cache),
           '--output', str(output / 'base-proof')]
if a.offline:
    command.append('--offline')
subprocess.run(command, check=True)
plan = json.loads((base / 'reports/predispatch.json').read_text())
assert plan['wire_version'] == 'compact-v3'
assert plan['schema_derivation_version'] == 'compact-category-v1'
records = []
for request in plan['prepared_requests']:
    req = json.loads(request['request'])
    case_id = request['seal']['window_id']
    schema = json.dumps(req['format'], sort_keys=True, separators=(',', ':')).encode()
    assert 'sha256:' + hashlib.sha256(schema).hexdigest() == request['schema_sha256']
    assert 'sha256:' + hashlib.sha256(req['system'].encode()).hexdigest() == request['system_sha256']
    schema_path = output / (case_id + '.schema.json')
    schema_path.write_bytes(schema)
    cases = root / 'cases' / (case_id + '.cases.json')
    grammar = output / (case_id + '.gbnf')
    checked = subprocess.run([str(cache / 'proof'), str(schema_path), str(grammar),
                              str(cases)], text=True, capture_output=True)
    (output / (case_id + '.results.txt')).write_text(checked.stdout + checked.stderr)
    if checked.returncode:
        raise SystemExit(checked.stdout + checked.stderr)
    rendered = ('<|im_start|>system\n' + req['system'] + '<|im_end|>\n'
                '<|im_start|>user\n' + req['prompt'] + '<|im_end|>\n'
                '<|im_start|>assistant\n')
    records.append({'case_id': case_id, 'schema_sha256': request['schema_sha256'],
                    'system_sha256': request['system_sha256'],
                    'request_sha256': 'sha256:' + hashlib.sha256(request['request'].encode()).hexdigest(),
                    'schema_bytes': len(schema), 'system_bytes': len(req['system'].encode()),
                    'input_bytes': len(req['prompt'].encode()), 'full_rendered_bytes': len(rendered.encode()),
                    'including_output768_reserve64': len(rendered.encode()) + 2 + 768 + 64,
                    'grammar_bytes': grammar.stat().st_size,
                    'cases_passed': len(json.loads(cases.read_text()))})
empty_cases = 0
for name in ['const-empty', 'maxItems0-no-items']:
    cases = root / 'cases' / (name + '.cases.json')
    checked = subprocess.run([str(cache / 'proof'),
                              str(root / 'cases' / (name + '.schema.json')),
                              str(output / (name + '.gbnf')), str(cases)],
                             text=True, capture_output=True)
    (output / (name + '.results.txt')).write_text(checked.stdout + checked.stderr)
    if checked.returncode:
        raise SystemExit(checked.stdout + checked.stderr)
    empty_cases += len(json.loads(cases.read_text()))
summary = {'schema_version': 1, 'derivation_version': 'compact-category-v1',
           'predispatch_sha256': 'sha256:' + hashlib.sha256((base / 'reports/predispatch.json').read_bytes()).hexdigest(),
           'source_manifest_sha256': 'sha256:' + hashlib.sha256((v2 / 'grammar-proof/sources.json').read_bytes()).hexdigest(),
           'cases': records, 'dynamic_and_empty_array_cases_passed': sum(r['cases_passed'] for r in records) + empty_cases,
           'inherited_closed_schema_cases_passed': 62,
           'maximum_context_bound': max(r['including_output768_reserve64'] for r in records),
           'maximum_grammar_bytes': max(r['grammar_bytes'] for r in records),
           'inference_requests': 0, 'limitations': 'ASCII grammar/source-structure proof only; no Unicode token/model quality or semantic entailment claim.'}
(output / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
print(f"PASS: {summary['dynamic_and_empty_array_cases_passed']} dynamic/empty-array cases; max context {summary['maximum_context_bound']}; max grammar {summary['maximum_grammar_bytes']} bytes")
