#!/usr/bin/env python3
"""Freeze empirical budgets only from the recorded, completed exploration.

The pinned Ollama 0.6.3 runner reports the full post-truncation prompt count;
counts strictly below num_ctx establish these exact observed inputs did not
trigger its input truncation branch. This is not a general tokenizer.
"""
import hashlib
import json
from pathlib import Path

base = Path(__file__).resolve().parents[2] / 'cmd/evie/docs/fixtures/memory-stage-4-spike/v1'
out = base / 'token-budgets.json'
if out.exists():
    raise SystemExit('token-budgets.json already exists; do not rewrite frozen evidence')

def digest(data):
    return 'sha256:' + hashlib.sha256(data).hexdigest()

files = ['prompt.txt', 'output.schema.json', 'runtime-manifest.json', 'runtime-api-metadata.json']
result = {'version': 'empirical-frozen-request-budgets-v1',
          'file_sha256': {p: digest((base / p).read_bytes()) for p in files},
          'proof': 'Ollama v0.6.3 llamarunner NewSequence truncates oversized input to numCtx, then records numPromptInputs=len(inputs). PromptEvalCount exposes that count. A positive recorded count strictly below the same configured context cannot have taken that truncation branch. This budget applies only to the exact recorded request and pinned runtime/model/template; changed or unmeasured requests fail closed.',
          'primary_source': 'https://github.com/ollama/ollama/blob/v0.6.3/runner/llamarunner/runner.go#L98-L152',
          'transport_source': 'https://github.com/ollama/ollama/blob/v0.6.3/server/routes.go#L282-L315',
          'reserve_tokens': 64, 'entries': []}
for name in ['development-schema.json', 'development-json.json']:
    path = base / 'reports' / name
    raw = path.read_bytes()
    report = json.loads(raw)
    for i, run in enumerate(report['runs']):
        if run['server_release'] != 'finished_response' or not 0 < run['prompt_tokens'] < report['context_tokens']:
            raise SystemExit(f'no truncation proof for {name} run {i}')
        if run['prompt_tokens'] + report['max_output_tokens'] + 64 > report['context_tokens']:
            raise SystemExit(f'no output reserve for {name} run {i}')
        result['entries'].append({'request_sha256': run['request_sha256'],
                                  'prompt_tokens': run['prompt_tokens'],
                                  'context_tokens': report['context_tokens'],
                                  'max_output_tokens': report['max_output_tokens'],
                                  'model': report['model'], 'mode': report['mode'],
                                  'report': 'reports/' + name, 'report_sha256': digest(raw),
                                  'run_index': i})
with out.open('x') as f:
    json.dump(result, f, indent=2)
    f.write('\n')
print(f'Frozen {len(result["entries"])} exact request budgets')
