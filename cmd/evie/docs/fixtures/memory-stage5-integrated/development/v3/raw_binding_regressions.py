#!/usr/bin/env python3
"""Replay retained development payloads; never generate or judge reader answers."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import types
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
import analyze
import resources


def read(path):
    return json.loads(Path(path).read_text())


def file_sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def save(path, value):
    Path(path).write_text(json.dumps(value, indent=2, sort_keys=True) + '\n')


def retained_inputs(freeze_path, evidence_path):
    freeze, evidence = read(freeze_path), read(evidence_path)
    if freeze['partition'] != 'development' or evidence['freeze_sha256'] != file_sha(freeze_path):
        raise ValueError('only the exact retained development attempt is authorized')
    manifest_path = freeze_path.parent / 'input-manifest.json'
    if file_sha(manifest_path) != freeze['input_manifest_sha256']:
        raise ValueError('canonical manifest changed')
    for name, expected in read(manifest_path).items():
        path = Path(name)
        if path.is_absolute() or '..' in path.parts or file_sha(Path(freeze['inputs_path']) / path) != expected:
            raise ValueError('canonical input differs: ' + name)
    results_manifest = evidence_path.parent / 'results-manifest.json'
    if file_sha(results_manifest) != evidence['results_manifest_sha256']:
        raise ValueError('retained result manifest differs')
    results = Path(evidence['results_path'])
    actual = {str(p.relative_to(results)): file_sha(p) for p in results.rglob('*') if p.is_file()}
    if actual != read(results_manifest):
        raise ValueError('retained raw result differs')
    return freeze, results, actual


class RawBindingControls(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.freeze, cls.results, cls.original_hashes = retained_inputs(ARGS.freeze, ARGS.evidence_report)
        cls.freeze_hash = file_sha(ARGS.freeze)
        # This is an explicitly new evaluator diagnostic using current code and
        # unchanged old raw inputs, not a new reader attempt or a rescored v2 report.
        cls.generated = analyze.build_report(cls.freeze, cls.results, cls.freeze_hash)

    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory(prefix='evie-stage5-raw-binding-')
        self.addCleanup(self.scratch.cleanup)
        self.output = Path(self.scratch.name)
        save(self.output / 'evidence-report.json', self.generated['report'])
        save(self.output / 'results-manifest.json', self.generated['results_manifest'])
        (self.output / 'assessment-packets').mkdir()
        for name, packet in self.generated['packets'].items():
            save(self.output / 'assessment-packets' / name, packet)

    def aggregate_binding(self):
        report = resources.Report.__new__(resources.Report)
        report.freeze, report.freeze_hash = self.freeze, self.freeze_hash
        report.checks, report.inputs, report.summaries = [], {}, {}
        report.helper = analyze
        report.stage('quality', lambda: report.reader_binding(
            self.output / 'evidence-report.json', read(self.output / 'evidence-report.json')))
        return report.finish()

    def assert_rejected(self, gate):
        report = self.aggregate_binding()
        self.assertFalse(report['all_required_gates_pass'], report)
        self.assertTrue(any(item['id'] == gate and item['status'] == 'fail' for item in report['gates']), report)

    def test_unchanged_raw_reconstruction_is_exact_and_pure(self):
        before = {str(p.relative_to(self.output)): file_sha(p) for p in self.output.rglob('*') if p.is_file()}
        self.assertEqual(self.generated, analyze.build_report(self.freeze, self.results, self.freeze_hash))
        report = self.aggregate_binding()
        self.assertTrue(report['all_required_gates_pass'], report)
        self.assertFalse(report['release_ready'])
        self.assertEqual(len(self.generated['packets']), 144)
        self.assertEqual(before, {str(p.relative_to(self.output)): file_sha(p) for p in self.output.rglob('*') if p.is_file()})

    def test_preloaded_foreign_auditor_cannot_replace_frozen_dependency(self):
        def poisoned(*args, **kwargs):
            raise AssertionError('foreign cached auditor was executed')
        foreign = types.SimpleNamespace(audit_evidence=poisoned, merge_support=poisoned)
        with patch.dict(sys.modules, {'audit_sources': foreign}):
            loaded = resources.load_module(ROOT / 'analyze.py', 'isolated_frozen_analyzer_control')
            self.assertEqual(loaded.build_report(self.freeze, self.results, self.freeze_hash), self.generated)
            self.assertIs(sys.modules['audit_sources'], foreign)

    def test_changed_answer_cannot_pass_with_matching_packet_hash(self):
        name = next(iter(self.generated['packets']))
        path = self.output / 'assessment-packets' / name
        packet = read(path)
        packet['final_answer'] = 'An edited answer that the retained reader never produced.'
        save(path, packet)
        # Refreshing the mutable assessment's answer/packet hashes cannot bind
        # this answer to a raw response. No semantic label is inferred here.
        superficial_assessment = {'answer_sha256': hashlib.sha256(packet['final_answer'].encode()).hexdigest(),
                                  'packet_sha256': file_sha(path)}
        self.assertEqual(superficial_assessment['packet_sha256'], file_sha(path))
        self.assert_rejected('evidence:exact_raw_packets')

    def test_changed_support_and_consistent_summary_cannot_pass(self):
        report = read(self.output / 'evidence-report.json')
        row = next(item for item in report['cases'] if item['required'] and not item['union'])
        # Keep the report and packet mutually consistent, including each source
        # count. Only the original captured provider requests expose the lie.
        for field in ('first', 'final', 'union', 'present_records'):
            row[field] = list(row['required'])
        for field in ('initial_source_hits', 'final_source_hits', 'union_source_hits'):
            row[field] = len(row['required'])
        row['complete_final_support'], row['unwanted'] = True, []
        condition = row['condition']
        cohort = [item for item in report['cases'] if item['condition'] == condition]
        answerable = [item for item in cohort if item['source_denominator']]
        denominator = sum(item['source_denominator'] for item in cohort)
        for field in ('initial', 'final', 'union'):
            report['conditions'][condition][field + '_source_micro_recall'] = sum(item[field + '_source_hits'] for item in cohort) / denominator
            report['conditions'][condition][field + '_source_macro_recall'] = sum(item[field + '_source_hits'] / item['source_denominator'] for item in answerable) / len(answerable)
        report['conditions'][condition]['complete_final_support_fraction'] = sum(item['complete_final_support'] for item in answerable) / len(answerable)
        total_present = sum(len(item['present_records']) for item in cohort)
        report['conditions'][condition]['unwanted_evidence_fraction'] = sum(len(item['unwanted']) for item in cohort) / total_present if total_present else None
        save(self.output / 'evidence-report.json', report)
        path = self.output / 'assessment-packets' / (row['case_id'] + '-' + condition + '.json')
        packet = read(path);packet['evidence_metrics'] = row;save(path, packet)
        self.assert_rejected('evidence:exact_raw_recomputation')

    def test_missing_or_extra_packet_is_incomplete_evidence(self):
        name = next(iter(self.generated['packets']))
        path = self.output / 'assessment-packets' / name
        path.unlink()
        self.assert_rejected('evidence:exact_raw_packet_identities')
        save(path, self.generated['packets'][name])
        save(path.with_name('undeclared.json'), self.generated['packets'][name])
        self.assert_rejected('evidence:exact_raw_packet_identities')

    def test_edited_provider_input_cannot_become_working_context(self):
        name = next(iter(self.generated['packets']))
        path = self.output / 'assessment-packets' / name
        packet = read(path)
        packet['actual_provider_inputs'].append({'input':[{'type':'message','role':'user','content':'Invented original context.'}]})
        save(path, packet)
        self.assert_rejected('evidence:exact_raw_packets')

    def test_cli_writer_keeps_output_shape_and_refuses_overwrite(self):
        destination = self.output / 'new-audit';destination.mkdir()
        analyze.evidence_report(self.freeze, self.results, destination, self.freeze_hash)
        self.assertEqual(read(destination / 'evidence-report.json'), self.generated['report'])
        self.assertEqual(read(destination / 'results-manifest.json'), self.generated['results_manifest'])
        self.assertEqual({p.name:read(p) for p in (destination/'assessment-packets').glob('*.json')}, self.generated['packets'])
        with self.assertRaises(FileExistsError):
            analyze.evidence_report(self.freeze, self.results, destination, self.freeze_hash)

    @classmethod
    def tearDownClass(cls):
        actual = {str(p.relative_to(cls.results)): file_sha(p) for p in cls.results.rglob('*') if p.is_file()}
        if actual != cls.original_hashes:
            raise AssertionError('diagnostic mutated original retained result')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--freeze', type=lambda p: Path(p).resolve(), required=True)
    parser.add_argument('--evidence-report', type=lambda p: Path(p).resolve(), required=True)
    ARGS = parser.parse_args()
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(RawBindingControls)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    raise SystemExit(not result.wasSuccessful())
