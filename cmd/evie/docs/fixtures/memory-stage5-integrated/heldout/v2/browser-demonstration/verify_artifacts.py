#!/usr/bin/env python3
"""Verify retained browser artifacts; never run inference or write their databases."""
import argparse
import hashlib
import json
from pathlib import Path
import sqlite3
import tarfile
import tempfile


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-root', type=Path)
    args = parser.parse_args()
    base = Path(__file__).resolve().parent
    direct = json.loads((base / 'ARTIFACTS.sha256.json').read_text())
    actual = {str(p.relative_to(base)) for p in base.rglob('*') if p.is_file()}
    assert actual == set(direct) | {'ARTIFACTS.sha256.json'}, 'direct file inventory changed'
    for name, expected in direct.items():
        raw = (base / name).read_bytes()
        assert digest(raw) == expected['sha256'] and len(raw) == expected['bytes'], name
    members = json.loads((base / 'archive-members.json').read_text())
    databases = 0
    with tarfile.open(base / 'session-artifacts.tar.gz', 'r:gz') as archive:
        entries = archive.getmembers()
        assert len(entries) == len(members)
        assert {m.name for m in entries} == set(members)
        with tempfile.TemporaryDirectory(prefix='evie-browser-integrity-') as temporary:
            for member in entries:
                assert member.isfile(), member.name
                stream = archive.extractfile(member)
                assert stream is not None
                raw = stream.read()
                expected = members[member.name]
                assert digest(raw) == expected['sha256'] and len(raw) == expected['bytes'], member.name
                if member.name.endswith('/browser.db'):
                    path = Path(temporary) / ('database-' + str(databases) + '.db')
                    path.write_bytes(raw)
                    with sqlite3.connect(path.as_uri() + '?mode=ro&immutable=1', uri=True) as database:
                        assert database.execute('PRAGMA quick_check').fetchall() == [('ok',)]
                    assert digest(path.read_bytes()) == expected['sha256']
                    databases += 1
            for name in ['ready.json', 'closed.json']:
                assert archive.extractfile('live-v1/' + name).read() == (base / ('live-' + name)).read_bytes()
    assert databases == 4
    closed = json.loads((base / 'live-closed.json').read_text())
    assert closed['fresh_browser_answer_persisted'] and closed['fresh_scripted_response_returned']
    assert not closed['manual_checks_attested_by_fixture']
    fresh = closed['cases'][0]
    assert len(fresh['answer_ids']) == len(fresh['snapshot_ids']) == 1
    assert fresh['memory_statuses'] == ['success']
    observations = json.loads((base / 'manual/observations.json').read_text())
    assert len(observations['observations']) == 11 and len(observations['screenshots']) == 8
    assert observations['scriptedDemonstration'] and observations['notReaderQualityEvaluation']
    assert all((base / 'manual' / item['path']).is_file() for item in observations['screenshots'])
    checked_sources = None
    if args.source_root is not None:
        frozen = json.loads((base / 'frozen-compiled-source-manifest.json').read_text())
        assert len(frozen) == 394
        for name, expected in frozen.items():
            assert digest((args.source_root / name).read_bytes()) == expected, name
        helper = args.source_root / 'cmd/evie/memory_stage5_browser_test.go'
        assert helper.read_bytes() == (base / 'source/memory_stage5_browser_test.go.txt').read_bytes()
        checked_sources = len(frozen)
    print(json.dumps({'status': 'pass', 'direct_files': len(direct), 'archive_members': len(members),
                      'databases': databases, 'captured_requests': sum('/request-' in name for name in members),
                      'accessibility_observations': 11, 'screenshots': 8, 'frozen_sources_verified': checked_sources,
                      'quality_measurements_added': 0}, sort_keys=True))


if __name__ == '__main__':
    main()
