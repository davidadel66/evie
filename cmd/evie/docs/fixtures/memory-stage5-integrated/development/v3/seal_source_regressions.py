#!/usr/bin/env python3
"""Synthetic release-seal protocol controls; no models or measurements.

The source-commit boundary uses actual temporary Git repositories. Expensive
prior freeze/report validation and data preparation are explicit fixture stubs.
No evaluation question, reader output, or claimed passing measurement is made.
"""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import types
import unittest
from unittest.mock import patch


class SealSourceRegressions(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='evie-seal-source-control-')
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.repo = self.base / 'repository'
        self.repo.mkdir()
        self.git('init', '-q')
        self.git('config', 'user.name', 'Synthetic protocol control')
        self.git('config', 'user.email', 'protocol@example.invalid')
        self.git('config', 'commit.gpgsign', 'false')
        originals = {'go.mod': b'module protocol\n', 'go.sum': b'',
                     'internal/agent/source.go': b'package agent\n',
                     'internal/agent/guide.txt': b'embedded original\n'}
        for name, data in originals.items():
            path = self.repo / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
        self.commit('Synthetic source fixture #167')
        self.development = self.base / 'development'
        self.development.mkdir()
        self.manifest = {name: hashlib.sha256(data).hexdigest() for name, data in originals.items()}
        (self.development / 'compiled-source-manifest.json').write_text(json.dumps(self.manifest))
        (self.development / 'compiled-source.tar.gz').write_bytes(b'synthetic protocol archive placeholder')
        self.freeze = {'partition': 'development', 'source_root': str(self.repo),
                       'binary_path': '/synthetic/no-executable', 'binary_sha256': 'fixture-only',
                       'worker_environment': {}, 'endpoint': 'http://127.0.0.1:1'}
        for field in ('gates', 'rubric', 'operating', 'matrix', 'index', 'scorer',
                      'auditor', 'grader', 'assessment', 'procedure', 'resources'):
            path = self.development / (field + '.fixture')
            path.write_text('Synthetic protocol fixture; no measurement.\n')
            self.freeze[field + '_path'] = str(path)
        (self.development / 'freeze.json').write_text('{}')
        (self.development / 'report.json').write_text('{}')
        workload = self.base / 'workload.json'
        workload.write_text(json.dumps({'partition': 'heldout', 'protocol_fixture_only': True}))
        self.args = types.SimpleNamespace(development_freeze=str(self.development / 'freeze.json'),
            development_report=str(self.development / 'report.json'), repository=str(self.repo),
            output=str(self.base / 'seal'), workload=str(workload), version='synthetic-protocol-only')

    def git(self, *arguments):
        return subprocess.check_output(['git', *arguments], cwd=self.repo, stderr=subprocess.PIPE, text=True).strip()

    def commit(self, message):
        self.git('add', '--all')
        self.git('commit', '-q', '-m', message)
        return self.git('rev-parse', 'HEAD')

    def seal(self, preparation=None):
        def prepare(command, directory, output, environment):
            inputs = Path(environment['EVIE_MEMORY_INTEGRATED_INPUTS'])
            (inputs / 'protocol-only').mkdir(parents=True)
            (inputs / 'protocol-only/seed.db').write_bytes(b'synthetic protocol placeholder; not a database')
            if preparation:
                preparation()
        with patch.object(RUNNER, 'verify_freeze', return_value=copy.deepcopy(self.freeze)), \
             patch.object(RUNNER, 'verified_development_report', return_value={}), \
             patch.object(RUNNER, 'local_metadata', return_value={'protocol_fixture_only': True}), \
             patch.object(RUNNER, 'run_command', side_effect=prepare), patch('builtins.print'):
            RUNNER.seal_release(self.args)
        return json.loads((Path(self.args.output) / 'freeze.json').read_text())

    def test_issue_subject_cannot_seal_changed_compiled_source(self):
        (self.repo / 'internal/agent/source.go').write_text('package changed\n')
        self.commit('Synthetic changed compiled source #167')
        with self.assertRaisesRegex(RuntimeError, 'compiled source'):
            self.seal()
        self.assertFalse(Path(self.args.output).exists())

    def test_missing_frozen_embedded_input_rejects_before_preparation(self):
        (self.repo / 'internal/agent/guide.txt').unlink()
        self.commit('Synthetic removed embed input #167')
        with self.assertRaisesRegex(RuntimeError, 'compiled source missing.*guide.txt'):
            self.seal()
        self.assertFalse(Path(self.args.output).exists())

    def test_module_and_embed_bytes_are_checked_as_well_as_go_source(self):
        for name in ['go.mod', 'go.sum', 'internal/agent/guide.txt']:
            with self.subTest(path=name):
                path = self.repo / name
                original = path.read_bytes()
                path.write_bytes(original + b'changed bytes\n')
                self.commit('Synthetic changed non-Go compiled input #167')
                with self.assertRaisesRegex(RuntimeError, 'compiled source differs'):
                    self.seal()
                self.assertFalse(Path(self.args.output).exists())
                path.write_bytes(original)
                self.commit('Synthetic restored protocol input #167')

    def test_documentation_only_commit_can_seal_identical_compiled_inputs(self):
        (self.repo / 'pilot-report.md').write_text('Synthetic documentation addition; no metrics.\n')
        commit = self.commit('Synthetic pilot documentation #167')
        sealed = self.seal()
        self.assertEqual(sealed['source_issue167_commit'], commit)
        self.assertEqual(json.loads((Path(self.args.output) / 'compiled-source-manifest.json').read_text()), self.manifest)

    def test_commit_bytes_not_uncommitted_worktree_bytes_define_the_seal(self):
        committed = self.git('rev-parse', 'HEAD')
        (self.repo / 'internal/agent/source.go').write_text('uncommitted unrelated work\n')
        sealed = self.seal()
        self.assertEqual(sealed['source_issue167_commit'], committed)
        self.assertEqual((self.repo / 'internal/agent/source.go').read_text(), 'uncommitted unrelated work\n')

    def test_later_head_movement_cannot_replace_the_verified_commit_reference(self):
        committed = self.git('rev-parse', 'HEAD')
        def change_head():
            (self.repo / 'internal/agent/source.go').write_text('changed during protocol preparation\n')
            self.commit('Synthetic later source commit #167')
        sealed = self.seal(preparation=change_head)
        self.assertNotEqual(self.git('rev-parse', 'HEAD'), committed)
        self.assertEqual(sealed['source_issue167_commit'], committed)

    def test_empty_manifest_cannot_vacuously_prove_a_source_commit(self):
        (self.development / 'compiled-source-manifest.json').write_text('{}')
        with self.assertRaisesRegex(RuntimeError, 'compiled source manifest'):
            self.seal()
        self.assertFalse(Path(self.args.output).exists())

    def test_source_equality_does_not_replace_the_issue_commit_requirement(self):
        (self.repo / 'note.md').write_text('Synthetic docs only.\n')
        self.commit('Synthetic unrelated issue subject')
        with self.assertRaisesRegex(RuntimeError, 'verified #167 pilot'):
            self.seal()
        self.assertFalse(Path(self.args.output).exists())


def main():
    global RUNNER
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runner', default=str(Path(__file__).resolve().parents[2] / 'run.py'))
    arguments, tests = parser.parse_known_args()
    spec = importlib.util.spec_from_file_location('source_seal_runner', arguments.runner)
    RUNNER = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(RUNNER)
    unittest.main(argv=[sys.argv[0], *tests], verbosity=2)


if __name__ == '__main__':
    main()
