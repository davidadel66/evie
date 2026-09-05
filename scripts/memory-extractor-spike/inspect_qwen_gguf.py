#!/usr/bin/env python3
"""Verify the exact acquired Qwen artifact and its conditional BPE byte bound.

Reads bounded GGUF metadata and streams the model for identity; never loads
model tensors, invokes inference, or opens conversational data.
"""
import argparse
import datetime
import hashlib
import json
import struct
from pathlib import Path

EXPECTED = '2bada8a7450677000f678be90653b85d364de7db25eb5ea54136ada5f3933730'
p = argparse.ArgumentParser()
p.add_argument('--model-file', type=Path, required=True)
p.add_argument('--output', type=Path, required=True)
a = p.parse_args()
if a.output.exists():
    p.error('output must be new')

with a.model_file.open('rb') as f:
    def exact(n):
        if n < 0 or f.tell() + n > 32 << 20:
            raise ValueError('metadata exceeds 32 MiB inspection bound')
        b = f.read(n)
        if len(b) != n:
            raise ValueError('truncated metadata')
        return b

    def scalar(fmt):
        return struct.unpack('<' + fmt, exact(struct.calcsize('<' + fmt)))[0]

    def string():
        return exact(scalar('Q')).decode('utf-8')

    def value(t):
        if t == 8:
            return string()
        if t == 9:
            item_type, count = scalar('I'), scalar('Q')
            if count > 1_000_000 or item_type == 9:
                raise ValueError('array inspection bound/type')
            return [value(item_type) for _ in range(count)]
        return scalar({0: 'B', 1: 'b', 2: 'H', 3: 'h', 4: 'I', 5: 'i',
                       6: 'f', 7: '?', 10: 'Q', 11: 'q', 12: 'd'}[t])

    if exact(4) != b'GGUF' or scalar('I') != 3:
        raise ValueError('expected GGUF v3')
    tensor_count, count = scalar('Q'), scalar('Q')
    if count > 1024:
        raise ValueError('metadata field bound')
    metadata = {}
    for _ in range(count):
        name = string()
        if name in metadata:
            raise ValueError('duplicate metadata key')
        metadata[name] = value(scalar('I'))
    end = f.tell()
    f.seek(0)
    metadata_hash = hashlib.sha256(f.read(end)).hexdigest()
    f.seek(0)
    identity = hashlib.sha256()
    for chunk in iter(lambda: f.read(8 << 20), b''):
        identity.update(chunk)
if identity.hexdigest() != EXPECTED or a.model_file.stat().st_size != 4_683_073_952:
    raise ValueError('actual model identity/size differs from authorized artifact')
for key, expected in {'general.architecture': 'qwen2',
                      'tokenizer.ggml.model': 'gpt2',
                      'tokenizer.ggml.pre': 'qwen2',
                      'tokenizer.ggml.add_bos_token': False}.items():
    if metadata.get(key) != expected:
        raise ValueError('unproven tokenizer metadata: ' + key)

tokens = metadata['tokenizer.ggml.tokens']
types = metadata['tokenizer.ggml.token_type']
merges = metadata['tokenizer.ggml.merges']
if len(tokens) != len(types) or len(set(tokens)) != len(tokens):
    raise ValueError('ambiguous token vocabulary')
vocabulary = set(tokens)
# The pinned runtime's unicode_byte_to_utf8 map, including its 68 remapped bytes.
direct = list(range(33, 127)) + list(range(161, 173)) + list(range(174, 256))
byte_symbols = {i: chr(i) for i in direct}
for n, i in enumerate(i for i in range(256) if i not in byte_symbols):
    byte_symbols[i] = chr(256 + n)
if any(symbol not in vocabulary for symbol in byte_symbols.values()):
    raise ValueError('missing encoded byte symbol; B+2 is unproven')
for merge in merges:
    # llama_vocab::load finds the first ASCII space starting at byte position1.
    raw = merge.encode('utf-8')
    pos = raw.find(b' ', 1)
    if pos < 1 or pos == len(raw) - 1:
        raise ValueError('malformed merge entry')
    left, right = raw[:pos].decode('utf-8'), raw[pos+1:].decode('utf-8')
    if left + right not in vocabulary:
        raise ValueError('merge result absent from vocabulary; B+2 is unproven')
specials = [(i, token, types[i]) for i, token in enumerate(tokens) if types[i] in (2, 3, 4)]
if not specials or any(not text for _, text, _ in specials):
    raise ValueError('empty active special spelling; B+2 is unproven')

def array_hash(items):
    return 'sha256:' + hashlib.sha256(json.dumps(items, ensure_ascii=False,
                                                separators=(',', ':')).encode()).hexdigest()

result = {'version': 'qwen2-bpe-byte-bound-evidence-v1',
          'observed_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
          'model_sha256': 'sha256:' + identity.hexdigest(),
          'model_bytes': a.model_file.stat().st_size, 'metadata_end_bytes': end,
          'metadata_sha256': 'sha256:' + metadata_hash,
          'tensor_count': tensor_count,
          'metadata': {k: v for k, v in metadata.items() if not isinstance(v, list)
                       and (k.startswith('tokenizer.') or k in ('general.architecture', 'qwen2.context_length'))},
          'vocabulary_count': len(tokens), 'vocabulary_sha256': array_hash(tokens),
          'token_types_sha256': array_hash(types), 'merges_count': len(merges),
          'merges_sha256': array_hash(merges), 'byte_symbols_present': 256,
          'all_parsed_merge_results_present': True, 'active_special_count': len(specials),
          'special_min_utf8_bytes': min(len(text.encode()) for _, text, _ in specials),
          'specials_sha256': array_hash(specials),
          'proof_scope': 'Pinned C++ BPE path: full rendered UTF-8 bytes+2; includes conservative BOS/EOS allowance. Requires exact template execution and reserved output/context preflight.'}
with a.output.open('x') as f:
    json.dump(result, f, indent=2)
    f.write('\n')
print('verified model, 256 byte symbols, merge closure and nonempty specials')
