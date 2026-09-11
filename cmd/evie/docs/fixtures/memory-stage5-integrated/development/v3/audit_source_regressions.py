#!/usr/bin/env python3
"""Replay retained public v2 HTTP evidence through the source-audit API.

These are deterministic evaluator regressions, not fresh reader measurements.
The retained freeze, canonical source maps, audit report and raw HTTP artifacts
are read-only. Mutated negative cases exist only in memory.
"""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import unittest


def read(path):
    return json.loads(Path(path).read_text())


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


class SourceRegressions(unittest.TestCase):
    @classmethod
    def configure(cls, arguments):
        cls.freeze_path = Path(arguments.freeze).resolve()
        cls.freeze = read(cls.freeze_path)
        cls.report_path = Path(arguments.evidence_report).resolve()
        cls.report = read(cls.report_path)
        if cls.report['freeze_sha256'] != sha(cls.freeze_path):
            raise ValueError('evidence report belongs to another freeze')
        cls.manifest = read(cls.freeze_path.parent / 'input-manifest.json')
        if sha(cls.freeze_path.parent / 'input-manifest.json') != cls.freeze['input_manifest_sha256']:
            raise ValueError('canonical input manifest changed')
        manifest = cls.report_path.parent / 'results-manifest.json'
        if sha(manifest) != cls.report['results_manifest_sha256']:
            raise ValueError('actual HTTP result manifest changed')
        cls.results = read(manifest)
        spec = importlib.util.spec_from_file_location('regression_auditor', arguments.auditor)
        cls.auditor = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.auditor)

    def case(self, case, condition):
        seed_path = Path(self.freeze['inputs_path']) / case / 'seed.json'
        self.assertEqual(sha(seed_path), self.manifest[case + '/seed.json'])
        seed = read(seed_path)
        paths = sorted(Path(self.report['results_path']).glob(case + '-' + condition + '-*-wire-request.json'))
        self.assertTrue(paths)
        groups = []
        for path in paths:
            self.assertEqual(sha(path), self.results[path.name])
            payload = read(path)
            evidence = []
            for message in payload.get('input', payload.get('messages', [])):
                content = message.get('content')
                if isinstance(content, str) and content.startswith('EVIE_MEMORY_DATA\n'):
                    evidence.extend(json.loads(content.split('\n', 1)[1]).get('evidence') or [])
            groups.append(evidence)
        return seed, groups

    def test_active_original_retrieved_historically_retains_source_support(self):
        seed, groups = self.case('dev03_uncompiled_bike', 'tool_only')
        record = 'dev03_bike'
        event = seed['bindings'][record]['source']['event_id']
        item = next(e for group in groups for e in group if e['sources'][0]['event_id'] == event)
        self.assertEqual((item['intent'], item['current_status']), ('historical', 'active'))
        # Minimized replay: one original, one binding, one actual source item.
        seed = {'bindings': {record: seed['bindings'][record]}}
        result = self.auditor.audit_evidence([item], seed)
        self.assertEqual(result['violations'], [])
        self.assertEqual(result['supported_records'], [record])

    def test_accepted_bridge_original_proves_its_display_alias_without_query_marker(self):
        seed, groups = self.case('dev11_graph_bridge', 'automatic_deeper')
        record = 'dev11_bridge'
        item = next(e for group in groups for e in group
                    if e.get('claim_id') == seed['bindings'][record]['claim_id'])
        self.assertFalse(item.get('identity_matches'))
        result = self.auditor.audit_evidence([item], seed)
        self.assertEqual(result['violations'], [])
        self.assertEqual(result['supported_records'], [record])

    def test_structured_accepted_subject_proves_exact_entity_without_query_marker(self):
        seed, groups = self.case('dev10_exact_entity_id', 'automatic_deeper')
        record = 'dev10_soren'
        item = copy.deepcopy(next(e for group in groups for e in group
                                  if e.get('claim_id') == seed['bindings'][record]['claim_id']))
        item.pop('identity_matches', None)
        self.assertEqual(item['claim']['subject_entity_id'], seed['bindings'][record]['subject_entity_id'])
        result = self.auditor.audit_evidence([item], seed)
        self.assertEqual(result['violations'], [])
        self.assertEqual(result['supported_records'], [record])

    def test_fact_interval_proves_historical_day_despite_different_query_clock_time(self):
        seed, groups = self.case('dev15_historical_validity', 'automatic_deeper')
        record = 'dev15_old'
        item = next(e for group in groups for e in group
                    if e.get('claim_id') == seed['bindings'][record]['claim_id'])
        self.assertEqual(item['valid_at'], '2022-06-15T12:00:00Z')
        self.assertEqual(seed['bindings'][record]['exact_reference']['valid_at'], '2022-06-15T00:00:00Z')
        result = self.auditor.audit_evidence([item], seed)
        self.assertEqual(result['violations'], [])
        self.assertEqual(result['supported_records'], [record])

    def test_acceptable_current_context_does_not_inherit_gold_historical_date(self):
        seed, groups = self.case('dev15_historical_validity', 'tool_only')
        record = 'dev15_current'
        item = next(e for group in groups for e in group
                    if e.get('claim_id') == seed['bindings'][record]['claim_id'])
        self.assertIn(record, seed['case']['gold']['acceptable_context_record_ids'])
        self.assertEqual(item['effective_valid_time'], {'from': None, 'to': None})
        result = self.auditor.audit_evidence([item], seed)
        self.assertEqual(result['violations'], [])
        self.assertEqual(result['supported_records'], [record])
        self.assertFalse(set(result['supported_records']) & set(seed['case']['gold']['support_sets'][0]))

    def test_required_target_dominates_context_label_and_standalone_remains_strict(self):
        for variation in ['required_and_context', 'standalone']:
            with self.subTest(variation=variation):
                seed, groups = self.case('dev15_historical_validity', 'tool_only')
                record = 'dev15_current'
                item = next(e for group in groups for e in group
                            if e.get('claim_id') == seed['bindings'][record]['claim_id'])
                if variation == 'required_and_context':
                    seed['case']['gold']['support_sets'][0].append(record)
                else:
                    seed.pop('case')
                self.assertEqual(self.auditor.audit_evidence([item], seed)['supported_records'], [])

    def accepted(self, case, record):
        seed, groups = self.case(case, 'automatic_deeper')
        item = next(e for group in groups for e in group
                    if e.get('claim_id') == seed['bindings'][record]['claim_id'])
        return seed, copy.deepcopy(item)

    def test_full_accepted_alias_origin_is_sufficient_but_not_raw_alias_text(self):
        record = 'dev09_ines'
        seed, item = self.accepted('dev09_exact_alias', record)
        item.pop('identity_matches', None)
        self.assertEqual(self.auditor.audit_evidence([item], seed)['supported_records'], [record])
        _, groups = self.case('dev09_exact_alias', 'automatic_deeper')
        raw = next(e for g in groups for e in g if e['kind'] == 'conversation_excerpt'
                   and e['sources'][0]['event_id'] == seed['bindings'][record]['source']['event_id'])
        self.assertEqual(self.auditor.audit_evidence([raw], seed)['supported_records'], [])

    def test_alias_fallback_requires_exact_canonical_mapping_and_full_origin(self):
        record = 'dev11_bridge'
        for field, value in [('missing_alias', None), ('entity_id', 'unaccepted-entity'),
                             ('source_event_id', 'another-origin'), ('operation_id', 'another-operation'),
                             ('scope_key', 'another-scope'), ('value', 'Luci')]:
            with self.subTest(field=field):
                seed, item = self.accepted('dev11_graph_bridge', record)
                alias_id = seed['bindings'][record]['exact_reference']['identity_matches'][0]['alias_id']
                aliases = seed['bindings'][record]['accepted_aliases']
                if field == 'missing_alias':
                    seed['bindings'][record]['accepted_aliases'] = [a for a in aliases if a['alias_id'] != alias_id]
                else:
                    next(a for a in aliases if a['alias_id'] == alias_id)[field] = value
                self.assertEqual(self.auditor.audit_evidence([item], seed)['supported_records'], [])

    def test_supplied_forged_identity_marker_cannot_fall_back_to_valid_original(self):
        for field, value in [('alias_id', 'unaccepted-alias'), ('entity_id', 'unaccepted-entity'),
                             ('alias_value', 'KIN-48'), ('source', {})]:
            with self.subTest(field=field):
                seed, item = self.accepted('dev09_exact_alias', 'dev09_ines')
                item['identity_matches'][0][field] = value
                result = self.auditor.audit_evidence([item], seed)
                self.assertTrue(result['violations'])
                self.assertEqual(result['supported_records'], [])

    def test_source_metadata_and_accepted_identity_remain_exact(self):
        for field, value in [('source_link_id', 'another-link'), ('authority', 'none'),
                             ('actor', 'assistant'), ('source_scope_key', 'another-scope'),
                             ('observed_at', '2026-09-11T04:26:36.533921001Z')]:
            with self.subTest(source_field=field):
                seed, item = self.accepted('dev11_graph_bridge', 'dev11_bridge')
                item['sources'][0][field] = value
                result = self.auditor.audit_evidence([item], seed)
                self.assertTrue(result['violations'])
                self.assertEqual(result['supported_records'], [])
        for field, value in [('claim_operation_id', 'another-operation'), ('subject_entity_id', 'another-entity')]:
            with self.subTest(claim_field=field):
                seed, item = self.accepted('dev10_exact_entity_id', 'dev10_soren')
                item.pop('identity_matches', None)
                (item['claim'] if field == 'subject_entity_id' else item)[field] = value
                result = self.auditor.audit_evidence([item], seed)
                self.assertTrue(result['violations'])
                self.assertEqual(result['supported_records'], [])

    def test_revoked_canonical_source_never_regains_support_from_public_original(self):
        for case, record in [('dev11_graph_bridge', 'dev11_bridge'), ('dev01_fresh_tea', 'dev01_tea')]:
            with self.subTest(record=record):
                seed, item = self.accepted(case, record)
                seed['bindings'][record]['source']['eligibility'] = 'retracted'
                result = self.auditor.audit_evidence([item], seed)
                self.assertTrue(result['violations'])
                self.assertEqual(result['supported_records'], [])

    def test_source_bytes_and_full_alias_origin_cannot_be_replaced_by_a_valid_hash(self):
        for variation in ['forged_bytes', 'clipped_origin']:
            with self.subTest(variation=variation):
                seed, item = self.accepted('dev11_graph_bridge', 'dev11_bridge')
                source = item['sources'][0]
                if variation == 'forged_bytes':
                    source['evidence'] = source['evidence'].replace('Omar', 'Iris')
                else:
                    source['evidence'] = 'Lucia'
                    source['locator_kind'] = 'utf8_byte_range'
                    source['locator_value'] = '0:5'
                source['evidence_sha256'] = hashlib.sha256(source['evidence'].encode()).hexdigest()
                result = self.auditor.audit_evidence([item], seed)
                self.assertTrue(result['violations'])
                self.assertEqual(result['supported_records'], [])

    def test_exact_entity_fallback_requires_delivered_structured_subject(self):
        seed, item = self.accepted('dev10_exact_entity_id', 'dev10_soren')
        item.pop('identity_matches', None)
        item.pop('claim', None)
        result = self.auditor.audit_evidence([item], seed)
        self.assertEqual(result['supported_records'], [])

    def test_retired_current_or_forged_current_status_remain_forbidden(self):
        for field, value in [('intent', 'current'), ('current_status', 'active'), ('status', 'active')]:
            with self.subTest(field=field):
                seed, item = self.accepted('dev15_historical_validity', 'dev15_old')
                item[field] = value
                result = self.auditor.audit_evidence([item], seed)
                self.assertTrue(result['violations'])
                self.assertEqual(result['supported_records'], [])

    def test_wrong_unknown_or_noncovering_fact_interval_cannot_prove_target(self):
        for variation in ['wrong_interval', 'unknown_interval', 'target_at_exclusive_end']:
            with self.subTest(variation=variation):
                seed, item = self.accepted('dev15_historical_validity', 'dev15_old')
                if variation == 'wrong_interval':
                    item['effective_valid_time']['from'] = '2022-04-01T00:00:00Z'
                elif variation == 'unknown_interval':
                    item['effective_valid_time'] = {'from': None, 'to': None}
                    seed['bindings']['dev15_old']['valid_time'] = {'from': None, 'to': None}
                else:
                    seed['bindings']['dev15_old']['exact_reference']['valid_at'] = '2022-11-01T00:00:00Z'
                self.assertEqual(self.auditor.audit_evidence([item], seed)['supported_records'], [])

    def test_graph_support_requires_both_accepted_records_not_raw_bridge_text(self):
        seed, groups = self.case('dev11_graph_bridge', 'automatic_deeper')
        result = self.auditor.merge_support(groups, seed)
        self.assertEqual(result['violations'], [])
        self.assertTrue({'dev11_bridge', 'dev11_preference'} <= set(result['supported_records']))
        claim_id = seed['bindings']['dev11_bridge']['claim_id']
        without_bridge = [[e for e in g if e.get('claim_id') != claim_id] for g in groups]
        result = self.auditor.merge_support(without_bridge, seed)
        self.assertNotIn('dev11_bridge', result['supported_records'])
        self.assertIn('dev11_preference', result['supported_records'])

    def test_all_retained_public_payloads_remain_valid_without_losing_prior_support(self):
        self.assertEqual(len(self.report['cases']), 144)
        for row in self.report['cases']:
            with self.subTest(case=row['case_id'], condition=row['condition']):
                seed, groups = self.case(row['case_id'], row['condition'])
                result = self.auditor.merge_support(groups, seed)
                self.assertEqual(result['violations'], [])
                self.assertTrue(set(row['union']) <= set(result['supported_records']))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--freeze', required=True)
    parser.add_argument('--evidence-report', required=True)
    parser.add_argument('--auditor', default=str(Path(__file__).resolve().parents[2] / 'audit_sources.py'))
    arguments, unittest_args = parser.parse_known_args()
    SourceRegressions.configure(arguments)
    unittest.main(argv=[sys.argv[0], *unittest_args], verbosity=2)


if __name__ == '__main__':
    main()
