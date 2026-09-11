#!/usr/bin/env python3
"""Recompute frozen resource/boundary gates. Missing evidence cannot pass.

No provider calls or source mutations. The report binds an exact freeze and
independently checks retained samples; semantic labels remain explicit judgments.
Deterministic logs are supplied by the implementation owner, not fabricated here.
"""
import argparse
import copy
import gzip
import hashlib
import importlib.util
import json
import math
from pathlib import Path
import re
import sys
import zlib


HARD = ("fabricated_source_citations", "retired_as_current_assertions",
        "authority_or_speaker_violations", "silent_conflict_resolutions")
MODES = ("cancellation", "lease_replacement", "caller_deadline")


def read(path):
    return json.loads(Path(path).read_text(), parse_constant=lambda value: (_ for _ in ()).throw(ValueError("nonfinite JSON " + value)))


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def integer(value):
    return type(value) is int and value >= 0


def quantiles(values):
    ordered = sorted(values)
    return {"count": len(ordered), "p50_ns": ordered[math.ceil(len(ordered)*.5)-1] if ordered else None,
            "p95_ns": ordered[math.ceil(len(ordered)*.95)-1] if ordered else None,
            "max_ns": ordered[-1] if ordered else None}


def same_list(value):
    return value or []


def load_module(path, name):
    # Frozen analyzer imports its adjacent frozen source auditor.
    sys.path.insert(0, str(Path(path).parent))
    try:
        spec = importlib.util.spec_from_file_location(name, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module
    finally:
        sys.path.pop(0)


class Report:
    def __init__(self, freeze_path):
        self.freeze_path = Path(freeze_path).resolve()
        self.freeze_hash = sha(self.freeze_path)
        self.freeze = read(self.freeze_path)
        self.checks, self.inputs, self.summaries = [], {}, {}
        self.helper = None

    def gate(self, name, actual, expected, comparison="==", unit="count", details=None):
        missing = actual is None
        try:
            passed = not missing and ({"==": lambda: actual == expected,
                       "<=": lambda: actual <= expected, ">=": lambda: actual >= expected}[comparison])()
        except (TypeError, ValueError):
            missing, passed = True, False
        self.checks.append({"id": name, "status": "incomplete" if missing else "pass" if passed else "fail",
                            "actual": actual, "expected": expected, "comparison": comparison,
                            "unit": unit, "details": details or []})

    def capture(self, path):
        path = Path(path).resolve()
        self.inputs[str(path)] = sha(path)
        return path

    def json(self, path):
        return read(self.capture(path))

    def stage(self, name, callback):
        try:
            callback()
        except (OSError, ValueError, KeyError, TypeError, IndexError, AttributeError, EOFError, zlib.error) as error:
            self.gate(name + ":input_complete", None, True, unit="boolean", details=[str(error)])

    def setup(self):
        for label in ("workload", "gates", "matrix", "scorer", "auditor", "grader", "resources"):
            path = self.capture(self.freeze[label + "_path"])
            if sha(path) != self.freeze[label + "_sha256"]:
                raise ValueError("frozen " + label + " differs")
        if sha(__file__) != self.freeze["resources_sha256"]:
            raise ValueError("running resource aggregator differs from frozen source")
        self.workload = self.json(self.freeze["workload_path"])
        self.gates = self.json(self.freeze["gates_path"])
        self.limits = self.gates["resources"]
        self.matrix = self.json(self.freeze["matrix_path"])
        self.helper = load_module(self.freeze["scorer_path"], "integrated_frozen_analyzer")
        self.audit = load_module(self.freeze["auditor_path"], "integrated_frozen_auditor")
        manifest_path = self.freeze_path.parent / "input-manifest.json"
        manifest = self.json(manifest_path)
        if sha(manifest_path) != self.freeze["input_manifest_sha256"]:
            raise ValueError("frozen input manifest differs")
        for relative, expected_hash in manifest.items():
            path = Path(relative)
            if path.is_absolute() or ".." in path.parts:
                raise ValueError("canonical manifest path escapes inputs")
            if sha(self.capture(Path(self.freeze["inputs_path"]) / path)) != expected_hash:
                raise ValueError("canonical frozen input differs: " + relative)
        self.cases = self.workload["cases"]
        self.conditions = self.gates["conditions"]
        self.seeds = {case["id"]: self.json(Path(self.freeze["inputs_path"]) / case["id"] / "seed.json") for case in self.cases}
        self.gate("freeze:partition", self.workload["partition"], self.freeze["partition"], unit="label")
        self.gate("freeze:case_count", len(self.cases), self.gates["reader_cases"])
        self.gate("freeze:condition_count", len(set(self.conditions)), 6)
        self.gate("freeze:unique_cases", len({case["id"] for case in self.cases}), len(self.cases))

    def execution(self, directory, mode):
        directory = Path(directory)
        bound = self.json(directory / "execution-freeze.json")
        result = self.json(directory / "execution.json")
        integrity = self.json(directory / "input-integrity-after-run.json")
        log = self.capture(directory / "execution.log").read_text()
        name = {"reader": "TestMemoryStage5IntegratedReaderEvaluation", "local": "TestMemoryStage5IntegratedLocalEvaluation", "index": "TestMemoryStage5IntegratedIndexMeasurements",
                "operating": "TestMemoryStage5IntegratedOperatingMeasurements"}[mode]
        self.gate(mode + ":execution_binding", bound.get("freeze_sha256") == self.freeze_hash and bound.get("mode") == mode
                  and bound.get("binary_sha256") == self.freeze["binary_sha256"], True, unit="boolean")
        command = result.get("command") or []
        self.gate(mode + ":selected_test", "-test.run=^" + name + "$" in command and "-test.count=1" in command
                  and command[0] == self.freeze["binary_path"] and "--- PASS: " + name + " " in log, True, unit="boolean")
        self.gate(mode + ":exit_code", result.get("exit_code"), 0)
        self.gate(mode + ":canonical_inputs_unchanged", integrity.get("unchanged"), True, unit="boolean")

    def turn(self, dispatches, requests, calls, seed, case):
        """Independent byte/receipt/ledger audit over every actual scripted dispatch."""
        errors, cumulative, previous_work, previous_attempts = [], 0, 0, 0
        metrics = {"kernel_searches": len(calls), "request_bytes_max": 0, "kernel_search_ns": 0,
                   "kernel_work_ns": 0, "cumulative_memory_bytes": 0, "search_attempts": 0, "search_attempts_minus_kernel_searches": 0}
        def need(ok, text):
            if not ok:
                errors.append(text)
        need(bool(dispatches) and len(dispatches) == len(requests), "missing/mismatched dispatch/request set")
        need(len(dispatches) <= self.limits["reader_calls_per_turn_max"], "reader dispatch cap exceeded")
        need(len(calls) <= self.limits["kernel_searches_per_turn_max"], "actual Kernel search cap exceeded")
        for index, (dispatch, payload) in enumerate(zip(dispatches, requests), 1):
            raw = self.helper.go_json(payload)
            digest = hashlib.sha256(raw).hexdigest()
            snapshot = dispatch["snapshot"]
            need(dispatch["call"] == index, "dispatch order differs")
            need(len(raw) == dispatch["request_bytes"] == snapshot["serialized_bytes"], "exact request byte count differs")
            need(digest == dispatch["request_sha256"] == snapshot["request_sha256"], "exact request SHA256 differs")
            need(len(raw) <= self.limits["request_serialized_bytes_max"], "request byte cap exceeded")
            metrics["request_bytes_max"] = max(metrics["request_bytes_max"], len(raw))
            evidence = self.helper.wire_evidence(payload)
            if "messages" in payload:  # Scripted Chat Completions probes, when used by model-free validation.
                evidence = []
                for message in payload["messages"]:
                    if isinstance(message.get("content"), str) and message["content"].startswith("EVIE_MEMORY_DATA\n"):
                        evidence.extend(json.loads(message["content"].split("\n", 1)[1]).get("evidence") or [])
            need(evidence == (dispatch.get("evidence") or []), "wire evidence differs from dispatch")
            receipt = snapshot.get("memory") or {}
            need([self.helper.reference(item) for item in evidence] == (receipt.get("evidence") or []), "wire original references differ from receipt")
            errors.extend(dispatch.get("boundary_errors") or [])
            errors.extend(self.helper.forbidden_wire(payload, evidence, case, seed))
            errors.extend(str(item) for item in self.audit.audit_evidence(evidence, seed)["violations"])
            need(len(evidence) <= self.limits["held_evidence_items_max"], "held evidence cap exceeded")
            memory_bytes, replay_bytes = 0, 0
            charged = dispatch.get("charged_tool_call_ids") or []
            need(len(charged) == len(set(charged)), "duplicate charged tool IDs")
            for message in dispatch["composed_request"]["messages"]:
                size = len(self.helper.go_json(message))
                if (message.get("content") or "").startswith("EVIE_MEMORY_DATA\n"):
                    memory_bytes += size
                elif message.get("role") == "tool" and message.get("tool_call_id") in charged:
                    replay_bytes += size
            cumulative += memory_bytes + replay_bytes
            need(memory_bytes == dispatch["serialized_memory_message_bytes"] and replay_bytes == dispatch["serialized_outcome_replay_bytes"], "message or replay bytes differ")
            need(cumulative == dispatch["cumulative_memory_delivery_bytes"], "cumulative delivery bytes differ")
            need(cumulative <= self.limits["cumulative_memory_message_bytes_max"], "cumulative memory cap exceeded")
            accounting = receipt.get("investigation")
            if receipt:
                need(isinstance(accounting, dict), "memory receipt lacks runtime ledger")
            if accounting:
                work, attempts = accounting["kernel_work_ns"], accounting["search_attempts"]
                need(integer(work) and previous_work <= work <= self.limits["cumulative_kernel_work_ms_max"]*1000000, "runtime work missing/nonmonotonic/over cap")
                need(integer(attempts) and attempts >= previous_attempts, "search attempts missing/nonmonotonic")
                need(accounting["cumulative_memory_bytes"] == cumulative, "runtime cumulative bytes differ")
                previous_work, previous_attempts = work, attempts
        for call in calls:
            result = call["result"]
            result_bytes = len(self.helper.go_json(result))
            actual = result.get("evidence") or []
            need(not call.get("error") and not call.get("result_encoding_error"), "Kernel call or result encoding error")
            need(call.get("status") == result.get("status") and call.get("status") not in {"failed", "cancelled"}, "failed or inconsistent Kernel result")
            need(result_bytes == call["result_marshaled_bytes"] and result_bytes <= self.limits["search_result_bytes_max"], "Kernel serialized result size differs/over cap")
            need(call["result_reported_serialized_bytes"] == result["serialized_bytes"], "reported Kernel bytes differ")
            need(call["result_evidence_count"] == len(actual) <= self.limits["held_evidence_items_max"], "Kernel result count differs/over cap")
            need([self.helper.reference(item) for item in actual] == (call.get("evidence") or []), "Kernel result references differ")
            elapsed = call["elapsed_ns"]
            need(integer(elapsed), "Kernel elapsed measurement unavailable")
            if integer(elapsed):
                metrics["kernel_search_ns"] += elapsed
            query = call["query"]
            if query.get("kind") == "conversation_expansion":
                need(0 <= query.get("before", 0) <= self.limits["expansion_before_max"] and 0 <= query.get("after", 0) <= self.limits["expansion_after_max"], "expansion window cap exceeded")
                need(len(actual) <= self.limits["expansion_messages_max"], "expanded message cap exceeded")
                need(all(len(item["text"].encode()) <= self.limits["expansion_message_bytes_max"] for item in actual), "expanded source byte cap exceeded")
        metrics.update(kernel_work_ns=previous_work, cumulative_memory_bytes=cumulative,
                       search_attempts=previous_attempts, search_attempts_minus_kernel_searches=previous_attempts-len(calls))
        need(not calls or previous_attempts >= len(calls), "receipt attempts undercount actual Kernel calls")
        need(previous_work >= metrics["kernel_search_ns"], "runtime work undercounts delegated Kernel search time")
        return errors, metrics

    def local(self, directory):
        self.execution(directory, "local")
        summaries, total, all_errors = [], 0, []
        expected_files = {case["id"] + "-" + condition + "-local-traces.jsonl.gz" for case in self.cases for condition in self.conditions}
        self.gate("local:declared_trace_files", {path.name for path in Path(directory).glob("*-local-traces.jsonl.gz")}, expected_files, unit="set")
        repetitions = self.limits["local_repetitions_per_case_condition"]
        for case in self.cases:
            for condition in self.conditions:
                label = case["id"] + "-" + condition
                path = Path(directory) / (label + "-local-traces.jsonl.gz")
                first, whole, work, observed, issues = [], [], [], [], []
                seen = set()
                counters = {"kernel_searches": 0, "search_attempts": 0, "search_attempts_minus_kernel_searches": 0}
                statuses = {}
                try:
                    self.capture(path)
                    with gzip.open(path, "rt") as source:
                        for line in source:
                            row = json.loads(line)
                            repetition = row["repetition"]
                            if row.get("case_id") != case["id"] or row.get("condition") != condition or repetition in seen or repetition not in range(1, repetitions+1):
                                issues.append("duplicate/undeclared case, condition or repetition")
                            seen.add(repetition)
                            total += 1
                            errors, metrics = self.turn(row["dispatches"], row["encoded_requests"], row.get("kernel_calls") or [], self.seeds[case["id"]], case)
                            issues.extend(str(repetition) + ": " + error for error in errors)
                            for key in counters:
                                counters[key] += metrics[key]
                            for call in row.get("kernel_calls") or []:
                                statuses[call["status"]] = statuses.get(call["status"], 0) + 1
                            if row.get("error") != "<nil>" or row.get("semantic_revisions_unchanged") is not True or row.get("reader_model_calls") != 0:
                                issues.append(str(repetition) + ": failed turn, revision check or scripted reader declaration")
                            if row.get("kernel_searches") != metrics["kernel_searches"] or row.get("summed_observed_kernel_search_ns") != metrics["kernel_search_ns"] or row.get("runtime_kernel_work_ns") != metrics["kernel_work_ns"]:
                                issues.append(str(repetition) + ": captured Kernel counters differ from recomputation")
                            dispatches = row["dispatches"]
                            if not dispatches or row.get("first_dispatch_elapsed_ns") != dispatches[0]["elapsed_to_dispatch_ns"]:
                                issues.append(str(repetition) + ": first dispatch measurement differs")
                            values = [row.get("first_dispatch_elapsed_ns"), row.get("whole_turn_elapsed_ns"), row.get("runtime_kernel_work_ns"), row.get("summed_observed_kernel_search_ns")]
                            if not all(integer(value) for value in values) or values[0] <= 0 or values[1] < values[0]:
                                issues.append(str(repetition) + ": missing/invalid timing; no successful-only quantile")
                            else:
                                for group, value in zip((first, whole, work, observed), values):
                                    group.append(value)
                except (OSError, ValueError, KeyError, TypeError, EOFError, zlib.error) as error:
                    issues.append(str(error))
                complete = seen == set(range(1, repetitions+1)) and len(first) == repetitions
                summary = {"case_id": case["id"], "condition": condition, "observed_repetitions": sorted(seen),
                           "complete": complete, "call_counters": counters, "kernel_status_counts": statuses, "first_dispatch": quantiles(first), "whole_turn": quantiles(whole),
                           "runtime_work": quantiles(work), "observed_kernel_search": quantiles(observed), "violations": issues}
                summaries.append(summary)
                self.gate("local:" + label + ":samples", len(seen) if complete else None, repetitions)
                self.gate("local:" + label + ":boundaries", len(issues), 0, details=issues)
                self.gate("local:" + label + ":initial_p95", summary["first_dispatch"]["p95_ns"] if complete else None, self.limits["local_initial_request_p95_ms_max"]*1000000, "<=", "nanoseconds")
                self.gate("local:" + label + ":work_p95", summary["runtime_work"]["p95_ns"] if complete else None, self.limits["local_retrieval_work_p95_ms_max"]*1000000, "<=", "nanoseconds")
                all_errors.extend(issues)
        self.gate("local:total_samples", total, len(self.cases)*len(self.conditions)*repetitions)
        self.summaries["local"] = {"samples": total, "groups": summaries, "model_calls": 0,
                                   "cold_warm_policy": "first observed sample and later warm-sequence samples; no guaranteed cold model or excluded outliers",
                                   "attempt_count_limit": "Actual Kernel calls are gated at eight. SearchAttempts also includes refused/reused attempts; its difference is reported separately and is not falsely labeled all refusals."}

    def index(self, directory):
        self.execution(directory, "index")
        summaries = []
        expected_paths = {"index-" + case["id"] + ".json" for case in self.cases}
        self.gate("index:declared_case_files", {p.name for p in Path(directory).glob("index-*.json")}, expected_paths, unit="set")
        for case in self.cases:
            label = "index:" + case["id"]
            def evaluate():
                result = self.json(Path(directory) / ("index-" + case["id"] + ".json"))
                seed = self.seeds[case["id"]]
                issues = list(result.get("failures") or [])
                def need(ok, text):
                    if not ok:
                        issues.append(text)
                need(result.get("case_id") == case["id"] and result.get("version") == "integrated-index-v1", "index case/version differs")
                need(result.get("test_failed") is False, "index test failed or incomplete")
                need(result["seed_database_sha256"] == seed["seed_database_sha256"] and result["source_records"] == len(case["records"]), "index canonical seed/record count differs")
                need(result["original_seed_index_elapsed_ns"] == seed["index_elapsed_ns"] and result["original_seed_index_batches"] == seed["index_batches"], "original build observation differs from seed")
                need(result["original_seed_index_coverage"] == seed["index_coverage"], "original seed coverage differs")
                def active(value):
                    return isinstance(value, dict) and value.get("state") == "active" and value.get("pending") == 0 and integer(value.get("indexed")) and bool(value.get("generation"))
                need(active(result["original_seed_index_coverage"]), "original seed coverage incomplete")
                for stage in ("baseline", "reopened", "rebuilt", "before_append", "incremental"):
                    need(active(result["coverage"].get(stage)), stage + " coverage is not active/zero-pending")
                    storage = result["storage"][stage]
                    db, wal = storage["database_bytes"], storage["wal_bytes"]
                    need(integer(db) and db > 0 and integer(wal) and not storage.get("error"), stage + " file measurement missing/invalid")
                    if storage.get("derived_page_bytes") is not None:
                        derived = storage["derived_page_bytes"]
                        need(integer(derived) and storage["method"] == "dbstat-derived-pages-plus-all-WAL-upper-bound" and not storage.get("dbstat_error"), stage + " invalid declared dbstat accounting")
                        bound = derived + wal
                    else:
                        need(bool(storage.get("dbstat_error")) and storage["method"] == "dbstat-unavailable-conservative-whole-database-plus-WAL-upper-bound", stage + " missing explicit conservative-storage limitation")
                        bound = db + wal
                    need(storage["derived_storage_upper_bound_bytes"] == bound, stage + " storage bound arithmetic differs")
                    self.gate(label + ":storage:" + stage, bound, self.limits["derived_index_database_and_wal_growth_bytes_max"], "<=", "bytes")
                need(isinstance(result["coverage"].get("after_append"), dict) and integer(result["coverage"]["after_append"].get("pending")), "incremental pending observation missing")
                for stage in ("rebuild", "incremental_refresh"):
                    maintenance = result[stage]
                    need(not maintenance.get("error") and active(maintenance.get("coverage")), stage + " maintenance failed")
                    need(integer(maintenance["elapsed_ns"]) and maintenance["refresh_batches"] > 0 and bool(maintenance.get("progress")), stage + " missing measured progress")
                    need(maintenance["progress"][-1] == maintenance["coverage"], stage + " final progress differs")
                self.gate(label + ":initial_build", result["original_seed_index_elapsed_ns"], self.limits["index_build_ms_max"]*1000000, "<=", "nanoseconds")
                self.gate(label + ":rebuild", result["rebuild"]["elapsed_ns"], self.limits["index_rebuild_ms_max"]*1000000, "<=", "nanoseconds")
                need(integer(result["original_seed_index_elapsed_ns"]) and integer(result["public_append_elapsed_ns"]) and result["public_append_elapsed_ns"] > 0, "missing positive preparation/append observation")
                before, after = result["warm_incremental_rss_before"], result["warm_incremental_rss_after"]
                rss_available = all(integer(sample.get("bytes")) and sample["bytes"] > 0 and not sample.get("error") for sample in (before, after))
                delta = after["bytes"] - before["bytes"] if rss_available else None
                need(rss_available and before["pid"] == after["pid"] and before["method"] == after["method"], "Go worker RSS missing or process/method differs")
                need(delta is not None and result.get("warm_incremental_rss_delta_bytes") == delta, "worker RSS delta differs")
                self.gate(label + ":worker_rss_growth", max(delta, 0) if delta is not None else None, self.limits["warm_incremental_worker_rss_growth_bytes_max"], "<=", "bytes")
                original_query, search_query, canonical = None, None, None
                for stage in ("baseline", "reopened", "rebuilt", "incremental_turn"):
                    turn = result[stage]
                    need(not turn.get("error") and turn.get("accepted_revisions_unchanged") is True and turn.get("delivered_sources_match_current_inspection") is True, stage + " public turn or current inspection failed")
                    need(len(turn["dispatches"]) == 2, stage + " incomplete public turn")
                    actual_seed = copy.deepcopy(seed)
                    if stage == "incremental_turn":
                        actual_seed.setdefault("all_public_sources", []).append(result["incremental_original_source"])
                    errors, _ = self.turn(turn["dispatches"], turn["encoded_requests"], turn.get("kernel_calls") or [], actual_seed, case)
                    issues.extend(stage + ": " + error for error in errors)
                    refs = copy.deepcopy((turn["dispatches"][-1]["snapshot"].get("memory") or {}).get("evidence") or [])
                    for ref in refs:
                        if not ref.get("as_known_at_constrained"):
                            ref["as_known_at"] = "0001-01-01T00:00:00Z"
                        if not ref.get("valid_at_constrained"):
                            ref["valid_at"] = "0001-01-01T00:00:00Z"
                        ref.pop("retrieval_generation", None)
                    need(refs == (turn.get("canonical_references") or []), stage + " canonical reference audit differs")
                    if stage == "baseline":
                        original_query, search_query, canonical = turn["query"], turn["bounded_search_query"], refs
                        need(original_query == seed["rendered_question"], "public index question differs from frozen seed")
                    elif stage != "incremental_turn":
                        need(turn["query"] == original_query and turn["bounded_search_query"] == search_query, stage + " recovery query differs")
                        need(refs == canonical, stage + " exact reference order/source agreement failed")
                need(result.get("restart_agreement") is True and result.get("rebuild_agreement") is True, "reported recovery agreement failed")
                incremental = result["incremental_turn"]
                source = result["incremental_original_source"]
                if case["memory_mode"] == "unavailable":
                    need(result.get("incremental_source_withheld_by_policy") is True and not incremental.get("kernel_calls"), "unavailable source was not withheld before Kernel")
                    need(all(source["event_id"] not in self.helper.go_json(request).decode() for request in incremental["encoded_requests"]), "unavailable incremental source ID reached provider")
                    last = incremental["dispatches"][-1]
                    need(not last["snapshot"].get("memory") and not last.get("evidence"), "unavailable probe fabricated a memory receipt")
                    denials = [message for message in last["composed_request"]["messages"] if message.get("role") == "tool" and message.get("tool_call_id") in {"index-accepted", "index-conversation"} and "model-facing memory reads require EVIE_REMOTE_MEMORY=on" in message.get("content", "")]
                    need(len(denials) == 2, "unavailable probe lacks both read-grant denials")
                else:
                    need(result.get("incremental_source_inspectable") is True, "incremental source inspection missing/failed")
                    delivered = [s for item in incremental["dispatches"][-1].get("evidence") or [] for s in item.get("sources") or []]
                    need(any(s["event_id"] == source["event_id"] and s["evidence"] == source["evidence"] and s["locator_value"] == source["locator_value"] and s["evidence_sha256"] == source["evidence_sha256"] and s["authority"] == "owner_statement" for s in delivered), "incremental exact original source not delivered")
                self.gate(label + ":boundaries", len(issues), 0, details=issues)
                summaries.append({"case_id": case["id"], "records": result["source_records"], "build_ns": result["original_seed_index_elapsed_ns"],
                                  "rebuild_ns": result["rebuild"]["elapsed_ns"], "incremental_refresh_ns": result["incremental_refresh"]["elapsed_ns"],
                                  "worker_rss_delta_bytes": delta, "endpoint_rss_before": result["endpoint_rss_before"], "endpoint_rss_after": result["endpoint_rss_after"],
                                  "endpoint_observation": result["endpoint_observation"], "storage": result["storage"], "violations": issues})
            self.stage(label, evaluate)
        self.gate("index:complete_cases", len(summaries), len(self.cases))
        self.summaries["index"] = {"cases": summaries, "source_records_total": sum(row["records"] for row in summaries),
                                   "isolated_databases": True, "model_process_rss_in_worker_gate": False}

    def operating(self, directory):
        self.execution(directory, "operating")
        root = Path(directory) / "measurements"
        config = self.json(root / "configuration.json")
        planned = self.limits["operating_repetitions"]
        bound = self.limits["cancellation_tail_ms_max"]*1000000
        self.gate("operating:configuration", config.get("repetitions_per_condition") == planned and config.get("conditions") == list(MODES) and config.get("max_tail_ns") == bound, True, unit="boolean")
        source = self.capture(root / "samples.ndjson")
        rows = [json.loads(line) for line in source.read_text().splitlines() if line.strip()]
        seen, issues, tails = set(), [], {mode: [] for mode in MODES}
        def need(ok, message):
            if not ok:
                issues.append(message)
        for sample in rows:
            identity = (sample.get("mode"), sample.get("repetition"))
            need(identity not in seen and identity[0] in MODES and identity[1] in range(planned), "duplicate/unknown operating sample")
            seen.add(identity)
            label = str(identity) + ": "
            need(sample.get("complete") is True and not sample.get("failures"), label + "failed/incomplete boundary")
            need(sample.get("error_classification") == identity[0] and bool(sample.get("send_error")), label + "terminal classification differs")
            tail = sample.get("tail_ns")
            # Monotonic duration is authoritative; RFC3339 timestamps cannot
            # reproduce it exactly. Validate ordering and retain both originals.
            need(integer(tail) and tail <= bound, label + "tail missing/negative/over bound")
            if integer(tail) and identity[0] in tails:
                tails[identity[0]].append(tail)
            need(self.audit._instant(sample["returned_at"]) >= self.audit._instant(sample["boundary_at"]), label + "recorded wall-clock boundary ordering differs")
            need(sample.get("approval_calls") == 0 and len(sample.get("requests") or []) == 2, label + "late approval/provider effect")
            requests = sample.get("requests") or []
            for request in requests:
                raw = request["encoded_json"].encode()
                need(len(raw) == request["bytes"] and hashlib.sha256(raw).hexdigest() == request["sha256"], label + "request byte/hash differs")
            def protected(events):
                return [event for event in events if event["Type"] == "context_snapshot" or event["Type"].startswith("tool_")]
            need(protected(sample["events_before_boundary"]) == protected(sample["events_after_return"]), label + "new/changed receipt or tool effect after boundary")
            need(sample["accepted_revisions_before"] == sample["accepted_revisions_after"], label + "accepted revisions changed")
            if identity[0] == "lease_replacement":
                need(bool(sample.get("replacement_lease")) and sample["replacement_lease"] == sample.get("lease_after_return"), label + "replacement lease changed")
            receipts = [event["Payload"] for event in sample["events_after_return"] if event["Type"] == "context_snapshot" and (event.get("Payload") or {}).get("memory")]
            need(len(receipts) == 1, label + "original memory receipt count differs")
            if len(receipts) == 1 and len(requests) == 2:
                receipt = receipts[0]
                need(receipt["request_sha256"] == requests[1]["sha256"] and receipt["serialized_bytes"] == requests[1]["bytes"] and receipt["memory"]["evidence"] == sample["evidence_before_boundary"] and receipt["memory"]["investigation"]["search_attempts"] == 1, label + "original supplied evidence/receipt differs")
        expected = {(mode, repetition) for mode in MODES for repetition in range(planned)}
        self.gate("operating:sample_identities", seen, expected, unit="set")
        self.gate("operating:sample_count", len(rows), len(expected))
        self.gate("operating:boundaries", len(issues), 0, details=issues)
        summaries = {}
        for mode in MODES:
            summary = quantiles(tails[mode])
            complete = len(tails[mode]) == planned and seen == expected and len(rows) == len(expected)
            self.gate("operating:" + mode + ":maximum_tail", summary["max_ns"] if complete else None, bound, "<=", "nanoseconds")
            summaries[mode] = {**summary, "complete": complete}
        self.summaries["operating"] = summaries

    def quality(self, evidence_path, quality_path):
        evidence = self.json(evidence_path)
        quality = self.json(quality_path)
        self.execution(evidence["results_path"], "reader")
        for field, expected in (("freeze_sha256", self.freeze_hash), ("scorer_sha256", self.freeze["scorer_sha256"]), ("auditor_sha256", self.freeze["auditor_sha256"])):
            self.gate("evidence:" + field, evidence.get(field), expected, unit="SHA256")
        self.gate("evidence:partition", evidence.get("partition"), self.freeze["partition"], unit="label")
        manifest_path = Path(evidence_path).parent / "results-manifest.json"
        manifest = self.json(manifest_path)
        self.gate("evidence:results_manifest", sha(manifest_path), evidence.get("results_manifest_sha256"), unit="SHA256")
        actual = {str(path.relative_to(evidence["results_path"])): sha(path) for path in Path(evidence["results_path"]).rglob("*") if path.is_file()}
        self.gate("evidence:retained_result_hashes", actual, manifest, unit="manifest")
        expected = {(case["id"], condition) for case in self.cases for condition in self.conditions}
        rows = {(row["case_id"], row["condition"]): row for row in evidence["cases"]}
        self.gate("evidence:case_identities", set(rows), expected, unit="set")
        self.gate("evidence:row_count", len(evidence["cases"]), len(expected))
        issues = list(evidence.get("violations") or [])
        for case in self.cases:
            required = set(case["gold"]["support_sets"][0])
            for condition in self.conditions:
                row = rows[(case["id"], condition)]
                prefix = case["id"] + "/" + condition + ": "
                if set(row["required"]) != required or row["source_denominator"] != len(required):
                    issues.append(prefix + "source denominator differs from frozen gold")
                for group, key in (("first", "initial"), ("final", "final"), ("union", "union")):
                    if row[key + "_source_hits"] != len(required & set(row[group])):
                        issues.append(prefix + key + " source-hit arithmetic differs")
                if row["complete_final_support"] != (required <= set(row["final"]) if required else None):
                    issues.append(prefix + "complete support arithmetic differs")
                issues.extend(prefix + str(error) for error in row.get("violations") or [])
                if not integer(row.get("whole_turn_elapsed_ns")):
                    issues.append(prefix + "reader latency missing")
        self.gate("evidence:all_boundaries_and_denominators", len(issues), 0, details=issues)
        summaries = {}
        for condition in self.conditions:
            cohort = [rows[(case["id"], condition)] for case in self.cases]
            elapsed = [row["whole_turn_elapsed_ns"] for row in cohort if integer(row.get("whole_turn_elapsed_ns"))]
            timing = quantiles(elapsed)
            self.gate("reader:" + condition + ":latency_complete", len(elapsed), len(self.cases))
            self.gate("reader:" + condition + ":whole_turn_p95", timing["p95_ns"] if len(elapsed) == len(cohort) else None, self.limits["reader_whole_turn_p95_ms_max"]*1000000, "<=", "nanoseconds")
            summaries[condition] = timing
        for field, expected_hash in (("freeze_sha256", self.freeze_hash), ("evidence_report_sha256", sha(evidence_path)), ("grader_sha256", self.freeze["grader_sha256"])):
            self.gate("quality:" + field, quality.get(field), expected_hash, unit="SHA256")
        # Replay the frozen deterministic validator/arithmetic against every
        # independently hashed explicit semantic assessment. No model judging.
        grader = load_module(self.freeze["grader_path"], "integrated_frozen_grader")
        regenerated = grader.build_report(self.freeze_path, Path(evidence_path).resolve(), Path(quality["assessment_directory"]).resolve())
        self.gate("quality:exact_recomputation", regenerated == quality, True, unit="boolean")
        self.gate("quality:all_assessments_valid", len(quality.get("validation_errors") or []), 0, details=quality.get("validation_errors"))
        self.gate("quality:all_quality_gates_pass", quality.get("all_quality_gates_pass"), True, unit="boolean")
        self.gate("quality:row_count", len(quality["cases"]), len(expected))
        qrows = {(row["case_id"], row["condition"]): row for row in quality["cases"]}
        self.gate("quality:case_identities", set(qrows), expected, unit="set")
        denominators = []
        for case in self.cases:
            supports = case["gold"]["support_sets"][0]
            components = len(case["gold"].get("expected_answer_components") or []) if supports else 0
            for condition in self.conditions:
                row = qrows[(case["id"], condition)]
                if row["component_denominator"] != components or row["citation_denominator"] != len(supports) or row.get("assessment_valid") is not True:
                    denominators.append(case["id"] + "/" + condition + ": missing assessment or altered denominator")
        self.gate("quality:required_denominators", len(denominators), 0, details=denominators)
        for check in quality["quality_gates"]:
            self.gate("quality:" + str(check.get("condition")) + ":" + check["name"], check["observed"], check["threshold"], check["comparison"], "declared frozen metric")
        for key in HARD:
            self.gate("quality:hard_zero:" + key, quality["hard_violation_totals"].get(key), self.gates["hard_boundaries"][key + "_max"], "<=")
        self.summaries["reader_latency"] = summaries
        self.summaries["quality"] = {"conditions": quality["conditions"], "hard_violation_totals": quality["hard_violation_totals"],
                                     "semantic_assessment_limit": quality.get("assessment_limit")}

    def deterministic(self, path):
        attestation = self.json(path)
        self.gate("deterministic:freeze_binding", attestation.get("freeze_sha256"), self.freeze_hash, unit="SHA256")
        self.gate("deterministic:matrix_binding", attestation.get("matrix_sha256"), self.freeze["matrix_sha256"], unit="SHA256")
        required_go, required_ui = set(), set()
        for row in self.matrix["rows"]:
            for test in row.get("tests") or []:
                if test["seam"] == "focused_ui":
                    for name in test["names"]:
                        required_ui.add((test["path"], name))
                else:
                    package = "github.com/davidadel66/evie/" + str(Path(test["path"]).parent)
                    required_go.add((package, test["name"]))
        seen_go, seen_ui, commands, issues = set(), set(), [], []
        full_verify = 0
        for entry in attestation["commands"]:
            identifier = entry["id"]
            command = entry["command"]
            log_path = self.capture(entry["log_path"])
            log = log_path.read_text()
            problems = []
            if sha(log_path) != entry["log_sha256"] or entry.get("exit_code") != 0:
                problems.append("command log hash or successful exit missing")
            if not isinstance(command, list) or not command or not all(isinstance(item, str) for item in command) or not Path(entry["cwd"]).is_absolute():
                problems.append("exact argv or absolute working directory missing")
            if entry["kind"] == "go_matrix":
                if len(command) < 3 or command[:2] != ["go", "test"] or "-json" not in command or "-count=1" not in command:
                    problems.append("Go matrix requires exact uncached go test -json command")
                events = [json.loads(line) for line in log.splitlines() if line.strip()]
                passed = {(event["Package"], event["Test"]) for event in events if event.get("Action") == "pass" and event.get("Test")}
                if any(event.get("Action") == "fail" for event in events):
                    problems.append("Go matrix log contains failures")
                claimed = {(item["package"], item["name"]) for item in entry.get("matched_tests") or []}
                if claimed != passed & required_go:
                    problems.append("claimed Go matched tests differ from structured log")
                if not problems:
                    seen_go.update(passed)
            elif entry["kind"] == "ui_matrix":
                result_path = self.capture(entry["structured_results_path"])
                result = read(result_path)
                if sha(result_path) != entry["structured_results_sha256"] or result.get("success") is not True or result.get("numFailedTests") != 0:
                    problems.append("Vitest structured results missing, changed or failed")
                if not any("vitest" in item for item in command) or "run" not in command:
                    problems.append("UI matrix command is not a Vitest run")
                passed = set()
                for suite in result.get("testResults") or []:
                    suite_path = str(suite["name"]).replace("\\", "/")
                    for assertion in suite.get("assertionResults") or []:
                        if assertion.get("status") == "passed":
                            for test_path, name in required_ui:
                                if suite_path.endswith(test_path) and assertion.get("title") == name:
                                    passed.add((test_path, name))
                claimed = {(item["path"], item["name"]) for item in entry.get("matched_tests") or []}
                if claimed != passed:
                    problems.append("claimed UI matched tests differ from Vitest JSON")
                if not problems:
                    seen_ui.update(passed)
            elif entry["kind"] == "verify_change":
                if command not in (["./scripts/verify-change.sh"], ["bash", "./scripts/verify-change.sh"]):
                    problems.append("full verification exact command differs")
                if not log.strip():
                    problems.append("full verification log is empty")
                if not problems:
                    full_verify += 1
            else:
                problems.append("unknown deterministic command kind")
            issues.extend(identifier + ": " + error for error in problems)
            commands.append({"id": identifier, "command": command, "cwd": entry["cwd"], "log_path": str(log_path), "log_sha256": sha(log_path), "exit_code": entry["exit_code"], "violations": problems})
        self.gate("deterministic:commands", len(issues), 0, details=issues)
        self.gate("deterministic:go_matrix", len(required_go & seen_go), len(required_go), details=[str(item) for item in sorted(required_go-seen_go)])
        self.gate("deterministic:ui_matrix", len(required_ui & seen_ui), len(required_ui), details=[str(item) for item in sorted(required_ui-seen_ui)])
        self.gate("deterministic:full_verify", full_verify, 1, ">=")
        self.summaries["deterministic"] = {"go_required": len(required_go), "ui_required": len(required_ui), "commands": commands,
                                          "attestation_limit": "Exact local commands, captured structured results and exit records supplied by the implementation owner; not a claim of an independent human review."}

    def finish(self):
        failures = [check for check in self.checks if check["status"] != "pass"]
        return {"schema_version": 1, "freeze_sha256": self.freeze_hash, "partition": self.freeze.get("partition"),
                "resource_aggregator_sha256": sha(__file__), "all_required_gates_pass": bool(self.checks) and not failures,
                "release_ready": bool(self.checks) and not failures and self.freeze.get("partition") == "heldout",
                "complete": bool(self.checks) and all(check["status"] != "incomplete" for check in self.checks),
                "gates": self.checks, "failures": failures, "summaries": self.summaries,
                "input_sha256": self.inputs,
                "limitations": ["Semantic answer labels remain explicit judgments under the frozen rubric.",
                                "Go worker RSS growth is separate from observed out-of-process embedding-family RSS.",
                                "Resource repetitions are not independent answer-quality repetitions.",
                                "No missing, failed or partial attempt is removed to improve a quantile or gate."]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("freeze", "evidence-report", "quality-report", "local", "index", "operating", "deterministic", "output"):
        parser.add_argument("--" + name, required=True)
    args = parser.parse_args()
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=False)
    try:
        report = Report(args.freeze)
        report.stage("freeze", report.setup)
        if report.helper is not None:
            report.stage("quality", lambda: report.quality(args.evidence_report, args.quality_report))
            report.stage("local", lambda: report.local(args.local))
            report.stage("index", lambda: report.index(args.index))
            report.stage("operating", lambda: report.operating(args.operating))
            report.stage("deterministic", lambda: report.deterministic(args.deterministic))
        result = report.finish()
    except (OSError, ValueError, KeyError, TypeError) as error:
        result = {"schema_version": 1, "all_required_gates_pass": False, "release_ready": False, "complete": False,
                  "gates": [{"id": "freeze:readable", "status": "incomplete", "details": [str(error)]}],
                  "failures": [str(error)]}
    with (output / "report.json").open("x") as stream:
        json.dump(result, stream, indent=2, sort_keys=True, allow_nan=False,
                  default=lambda value: sorted(value) if isinstance(value, set) else str(value))
        stream.write("\n")
    print(json.dumps({"all_required_gates_pass": result["all_required_gates_pass"], "release_ready": result["release_ready"], "complete": result["complete"], "failed_gates": len(result["failures"])}))
    return 0 if result["all_required_gates_pass"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
