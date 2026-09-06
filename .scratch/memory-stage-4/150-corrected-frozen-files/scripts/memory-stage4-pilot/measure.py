#!/usr/bin/env python3
"""Reproducible, sequential infrastructure experiments; no quality inference."""

import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import shutil
import signal
import subprocess
import sys
import time


def digest(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def write(path, value):
    with Path(path).open("x") as stream:
        json.dump(value, stream, indent=2, sort_keys=True)
        stream.write("\n")


def distribution(values):
    values = sorted(values)
    if not values:
        return {"n": 0, "p50": None, "p95": None, "max": None}
    return {"n": len(values), "p50": values[math.ceil(len(values) * .5)-1],
            "p95": values[math.ceil(len(values) * .95)-1], "max": values[-1]}


def variants():
    base = {"retained-events": 10000, "source-bytes": 256, "graph-claims": 1,
            "scopes": 1, "delay-ms": 25, "processes": 1, "turns": 16, "backfill-roots": 16}
    out = [("baseline", base)]
    for factor, values in [("retained-events", [100000, 1000000]),
                           ("source-bytes", [4096, 12000]),
                           ("graph-claims", [100, 1000]), ("scopes", [16]),
                           ("delay-ms", [0, 250]), ("processes", [2])]:
        for value in values:
            out.append((f"{factor}-{value}", {**base, factor: value}))
    return out


def process_sample(root_pid):
    # ps contains only pid/ppid/resource counters, never environment or arguments.
    result = subprocess.run(["ps", "-axo", "pid=,ppid=,rss=,%cpu="],
                            text=True, capture_output=True, check=True)
    rows = {}
    for line in result.stdout.splitlines():
        fields = line.split()
        if len(fields) == 4:
            rows[int(fields[0])] = (int(fields[1]), int(fields[2]), float(fields[3]))
    selected = {root_pid}
    while True:
        added = {pid for pid, row in rows.items() if row[0] in selected}
        if added <= selected:
            break
        selected |= added
    return {"monotonic_ns": time.monotonic_ns(), "processes": [
        {"pid": pid, "rss_kib": rows[pid][1], "cpu_percent": rows[pid][2]}
        for pid in sorted(selected) if pid in rows]}


def stop_process_tree(process):
    # Popen starts a new session, so this group contains only this owned trial.
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    try:
        process.wait(timeout=25)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait()


def conformance(path):
    report = json.loads(Path(path).read_text())
    if report.get("version") != "memory-stage-4-conformance-v1" or report.get("status") != "passed" or report.get("failures") or report.get("skipped_checks"):
        raise ValueError("a complete passing #149 conformance receipt is required before workload experiments")
    return {"path": str(Path(path).resolve()), "sha256": digest(path),
            "version": report["version"], "status": report["status"], "source": report["source"]}


def source_identity(root):
    paths = [root / "go.mod", root / "go.sum"]
    for directory in ["cmd", "internal", "scripts/memory-stage4-pilot"]:
        paths.extend(p for p in (root / directory).rglob("*")
                     if p.is_file() and p.suffix in {".go", ".py"} and "node_modules" not in p.parts)
    files = {str(path.relative_to(root)): digest(path) for path in sorted(set(paths))}
    canonical = json.dumps(files, sort_keys=True, separators=(",", ":")).encode()
    return {"files": files, "sha256": hashlib.sha256(canonical).hexdigest()}


def validate_baseline_source(baseline, identity):
    expected = baseline["source"].get("files", [])
    if not expected:
        raise ValueError("conformance omits its source inventory")
    def dependency(name):
        return ((name.startswith("internal/") or name.startswith("cmd/")) and name.endswith(".go")) or name in {"go.mod", "go.sum"}
    expected_files = {item["path"]: item["sha256"] for item in expected if dependency(item["path"])}
    actual_files = {name: sha for name, sha in identity["files"].items() if dependency(name)}
    if expected_files.keys() != actual_files.keys():
        added = sorted(actual_files.keys() - expected_files.keys())
        missing = sorted(expected_files.keys() - actual_files.keys())
        raise ValueError(f"conformance source path set differs: added={added}, missing={missing}")
    for name, sha in expected_files.items():
        if actual_files[name] != sha:
            raise ValueError(f"conformance source differs from workload source: {name}")


def summarize(raw):
    metrics = {}
    for name in ["terminal_commit_nanos", "response_finalization_nanos"]:
        metrics[name] = distribution([m[name] for m in raw["foreground"] if m.get(name) is not None])
    for name in ["queue_wait_nanos", "inference_nanos", "validation_nanos", "database_completion_nanos"]:
        metrics[name] = distribution([m[name] for j in raw["jobs"] for m in j["measurements"] if m.get(name) is not None])
    for name in ["publication_nanos", "candidate_freshness_nanos"]:
        metrics[name] = distribution([j[name] for j in raw["jobs"] if j.get(name) is not None])
    metrics["scripted_resolution_nanos"] = distribution(raw["scripted_resolution_nanos"])
    elapsed = raw["observed_nanos"] / 1e9
    foreground_elapsed = raw["counts"]["foreground_elapsed_nanos"] / 1e9
    metrics["observed_completed_events_per_second"] = raw["counts"].get("completed_events", 0) / elapsed
    metrics["offered_foreground_events_per_second"] = raw["counts"]["foreground_persisted_events"] / foreground_elapsed
    metrics["observed_candidate_arrivals_per_second"] = len(raw["candidates"]) / elapsed
    # The observation window includes worker startup and finite catch-up, so
    # every completed job belongs to its denominator. This is a workload rate,
    # not saturated model service capacity.
    metrics["active_human_review_candidates_per_second"] = None
    metrics["source_arrival_capacity_viable"] = None
    metrics["candidate_arrival_review_capacity_viable"] = None
    metrics["oldest_unresolved_age_ms_before_scripted_review"] = None
    # Candidate age uses the actual latest diagnostic clock, never active time.
    published = [c["published_at_unix_ms"] for c in raw["candidates"]
                 if c.get("published_at_unix_ms") is not None and c["review_state"] == "unresolved"]
    if published:
        metrics["oldest_unresolved_age_ms_before_scripted_review"] = max(0, raw["diagnostics_as_of_unix_ms"] - min(published))
    return metrics


def trial_observation(path, exit_code):
    """A failed/missing receipt is evidence, not an aggregation exception."""
    observation = {"report_sha256": None, "status": "failed"}
    try:
        observation["report_sha256"] = digest(path)
        raw = json.loads(Path(path).read_text())
        if not isinstance(raw, dict):
            raise ValueError("trial receipt must be a JSON object")
        if exit_code != 0:
            raise ValueError(raw.get("error") or f"trial process exited with status {exit_code}")
        if raw.get("error"):
            raise ValueError(raw["error"])
        if raw.get("version") != "memory-stage4-pilot-infrastructure-v1" or raw.get("kind") != "scripted_infrastructure_only" or raw.get("release_eligible") is not False:
            raise ValueError("trial receipt has an invalid infrastructure identity")
        if raw.get("disposable_database_removed") is not True:
            raise ValueError("worker did not confirm disposable database cleanup")
        observation["metrics"] = summarize(raw)
        observation["status"] = "passed"
    except (OSError, UnicodeError, ValueError, TypeError, KeyError, ZeroDivisionError) as error:
        observation["error"] = f"incomplete trial receipt: {error}"
    return observation


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--conformance", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--repetitions", type=int, default=3)
    parser.add_argument("--only", help="comma-separated declared variant IDs")
    args = parser.parse_args()
    if args.repetitions < 2 or args.repetitions > 5:
        parser.error("use 2–5 paired repetitions; one run does not quantify repetition")
    baseline = conformance(args.conformance)
    root = Path(__file__).resolve().parents[2]
    identity = source_identity(root)
    validate_baseline_source(baseline, identity)
    args.output.mkdir(parents=True, exist_ok=False)
    binary = args.output / "memory-stage4-pilot"
    subprocess.run(["go", "build", "-o", str(binary.resolve()), "./scripts/memory-stage4-pilot"], cwd=root, check=True)
    selected = variants()
    if args.only:
        ids = set(args.only.split(","))
        selected = [v for v in selected if v[0] in ids]
        if {v[0] for v in selected} != ids:
            parser.error("unknown variant")
    corpus = root / "cmd/evie/docs/fixtures/memory-stage-4-spike/v1"
    metadata = {"version": "memory-stage4-pilot-matrix-v1", "kind": "scripted_infrastructure_only",
                "release_eligible": False, "conformance": baseline, "source": identity,
                "binary_sha256": digest(binary), "environment": {"platform": platform.platform(),
                "machine": platform.machine(), "python": platform.python_version(), "cpu_count": os.cpu_count()},
                "spike_contract": {name: digest(corpus / name) for name in ["development.json", "development.gold.json", "holdout-custody.md"]},
                "repetitions": args.repetitions, "variant_ids": [v[0] for v in selected],
                "quality": {"status": "pending_human_output_adjudication_and_selected_model", "supported_useful_precision": None,
                            "required_memory_recall": None, "raw_proposals_graded": None, "retained_proposals_graded": None},
                "human_review": {"status": "pending_actual_David_sessions", "active_seconds": None, "capacity": None},
                "chosen_configuration": None, "numerical_release_gates": None, "model_server_resources": None,
                "final_holdout": {"created": False, "exposed": False, "run": False}, "runs": [], "paired_deltas": [],
                "limitations": ["Scripted infrastructure measurements cannot establish model quality or runtime fitness.",
                "Modes rotate across repetitions; each run has a new database and identical declared fixtures, with fresh OS-cache state uncontrolled.",
                "Each variant changes one factor from baseline; factors are not combined into capacity promises.",
                "ps samples whole pilot process trees at 100ms including setup; RSS is sampled, not an exact peak. CPU percentages are ps interval estimates.",
                "Archived rows vary retained database size, not the size of an active foreground conversation.",
                "Source and candidate throughput are observed workload rates, not sustained compilation or human review capacity.",
                "No quality thresholds are frozen without selected-model and actual-human pilot observations."]}
    write(args.output / "plan.json", {k:v for k,v in metadata.items() if k not in {"runs", "paired_deltas"}})
    failures = []
    for variant_id, variant in selected:
        for repetition in range(args.repetitions):
            modes = ["disabled", "new", "history"]
            modes = modes[repetition % 3:] + modes[:repetition % 3]
            pair = {}
            for mode in modes:
                if shutil.disk_usage(args.output).free < 3 * 1024**3:
                    raise RuntimeError("less than 3 GiB free; preserved completed reports without another fixture")
                run_id = f"{variant_id}-r{repetition+1}-{mode}"
                report_path = args.output / f"{run_id}.json"
                command = [str(binary.resolve()), "run", "--mode", mode, "--output", str(report_path.resolve())]
                for key, value in variant.items():
                    command.extend([f"--{key}", str(value)])
                samples = []
                started = time.monotonic()
                with (args.output / f"{run_id}.stderr").open("x") as stderr:
                    process = subprocess.Popen(command, cwd=root, stderr=stderr, stdout=stderr, start_new_session=True)
                    try:
                        while process.poll() is None:
                            samples.append(process_sample(process.pid))
                            if time.monotonic() - started > 660:
                                stop_process_tree(process)
                                break
                            time.sleep(.1)
                    except BaseException:
                        stop_process_tree(process)
                        raise
                resources = {"samples": samples, "sampled_peak_process_tree_rss_kib": max(
                    (sum(p["rss_kib"] for p in s["processes"]) for s in samples), default=None)}
                resource_path = args.output / f"{run_id}.resources.json"
                write(resource_path, resources)
                entry = {"id": run_id, "variant": variant_id, "repetition": repetition+1, "mode": mode,
                         "command": command, "exit_code": process.returncode,
                         "resources_sha256": digest(resource_path), "sampled_peak_process_tree_rss_kib": resources["sampled_peak_process_tree_rss_kib"]}
                entry.update(trial_observation(report_path, process.returncode))
                if entry["status"] != "passed":
                    failures.append(run_id)
                else:
                    pair[mode] = entry
                metadata["runs"].append(entry)
                print(f"{run_id}: {entry['status']}", flush=True)
            for mode in ["new", "history"]:
                if "disabled" in pair and mode in pair:
                    for metric in ["terminal_commit_nanos", "response_finalization_nanos"]:
                        left, right = pair["disabled"]["metrics"][metric], pair[mode]["metrics"][metric]
                        metadata["paired_deltas"].append({"variant": variant_id, "repetition": repetition+1,
                            "mode": mode, "metric": metric, "baseline_n": left["n"], "current_n": right["n"],
                            "p50_delta": right["p50"]-left["p50"], "p95_delta": right["p95"]-left["p95"]})
    metadata["source_after_sha256"] = source_identity(root)["sha256"]
    if metadata["source_after_sha256"] != identity["sha256"]:
        failures.append("source_changed_during_measurement")
    metadata["failures"] = failures
    metadata["infrastructure_status"] = "failed" if failures else "passed"
    metadata["pilot_status"] = "incomplete"
    write(args.output / "report.json", metadata)
    return bool(failures)


if __name__ == "__main__":
    sys.exit(main())
