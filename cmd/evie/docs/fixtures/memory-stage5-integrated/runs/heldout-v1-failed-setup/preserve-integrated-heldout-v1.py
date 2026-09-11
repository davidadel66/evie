#!/usr/bin/env python3
"""Plan, copy and verify immutable Stage 5 heldout-v1 evidence; stdlib only.

Planning reads originals and writes only a NEW plan file. Copy is an explicit
later command. Neither command edits original freezes, inputs, logs or reports.
No process environment, model runtime or network is read or invoked.
"""

import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import shutil
import tarfile


RUN = "integrated-heldout-v1"
DEVELOPMENT = "integrated-development-v4"
DIRECTORIES = {
    "raw-reader.tar.gz": [RUN + "-reader"],
    "raw-source-audit.tar.gz": [RUN + "-evidence"],
    "raw-assessments.tar.gz": [RUN + "-assessments"],
    "raw-local.tar.gz": [RUN + "-local"],
    "raw-index.tar.gz": [RUN + "-index"],
    "raw-operating.tar.gz": [RUN + "-operating"],
    "raw-verification.tar.gz": [RUN + "-final-checks"],
    "superseded-v3-checks.tar.gz": [RUN + "-checks"],
}
VERIFICATION_FILES = ["run-integrated-heldout-v1.py", "168-verify-heldout-v1-final.py",
                      "168-seal-heldout-v1-command-prepared.json"]
OPTIONAL_PATTERNS = ["*heldout-v1*.py", "*heldout-v1*.json", "*heldout-v1*.log", "*heldout-v1*.md",
                     "168-*.json", "168-*.log", "168-*.py", "architecture-curation-metadata-exposure.json"]
REPO = Path("/Users/davidboktor/code/evie-memory-stage-5")
REPO_REGRESSION_DIR = Path("cmd/evie/docs/fixtures/memory-stage5-integrated/development/v4")
REPO_REGRESSION_DIRS = [REPO_REGRESSION_DIR, Path("cmd/evie/docs/fixtures/memory-stage5-integrated/development/v3")]
CURATION_DIR = Path("cmd/evie/docs/fixtures/memory-stage5-integrated/heldout/v1")

def repo_references():
    return {str(path.relative_to(REPO)): describe(path)
            for directory in REPO_REGRESSION_DIRS for path in sorted((REPO / directory).rglob("*")) if path.is_file()}


def read(path):
    return json.loads(Path(path).read_text())


def digest(path):
    result = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for data in iter(lambda: stream.read(1 << 20), b""):
            result.update(data)
    return result.hexdigest()


def write_new(path, value):
    with Path(path).open("x") as stream:
        json.dump(value, stream, indent=2, sort_keys=True)
        stream.write("\n")


def regular(path):
    if path.is_symlink() or not path.is_file():
        raise ValueError("expected regular original file: " + str(path))
    with path.open("rb") as stream:
        magic = stream.read(4)
    if magic in (b"\x7fELF", b"\xcf\xfa\xed\xfe", b"\xfe\xed\xfa\xcf", b"\xca\xfe\xba\xbe"):
        raise ValueError("compiled executable must not enter preservation payload: " + str(path))


def describe(path):
    regular(path)
    return {"source": str(path), "bytes": path.stat().st_size, "sha256": digest(path), "mtime_ns": path.stat().st_mtime_ns, "mode": path.stat().st_mode & 0o777}


class CountHash(io.RawIOBase):
    def __init__(self):
        self.bytes, self.sha256 = 0, hashlib.sha256()

    def writable(self):
        return True

    def write(self, value):
        self.bytes += len(value)
        self.sha256.update(value)
        return len(value)


def archive_to(sink, members):
    # New wrapper metadata is deterministic; original member bytes and times are retained.
    with gzip.GzipFile(filename="", mode="wb", fileobj=sink, mtime=0, compresslevel=9) as compressed:
        with tarfile.open(fileobj=compressed, mode="w|", format=tarfile.PAX_FORMAT) as archive:
            for name, item in sorted(members.items()):
                path = Path(item["source"])
                if path.stat().st_size != item["bytes"] or digest(path) != item["sha256"] or path.stat().st_mtime_ns != item["mtime_ns"]:
                    raise ValueError("original changed after plan: " + str(path))
                info = tarfile.TarInfo(name)
                info.size, info.mode = item["bytes"], item["mode"]
                info.mtime = item["mtime_ns"] // 1_000_000_000
                info.pax_headers["mtime"] = str(item["mtime_ns"] // 1_000_000_000) + "." + str(item["mtime_ns"] % 1_000_000_000).zfill(9)
                with path.open("rb") as source:
                    archive.addfile(info, source)


def validate_original_archive(archive_path, manifest_path, expected_sha):
    if digest(archive_path) != expected_sha:
        raise ValueError("original archive differs from freeze")
    manifest = read(manifest_path)
    seen = set()
    with tarfile.open(archive_path, "r:gz") as archive:
        for member in archive:
            if not member.isfile() or member.name not in manifest or member.name in seen:
                raise ValueError("unexpected original archive member")
            if hashlib.sha256(archive.extractfile(member).read()).hexdigest() != manifest[member.name]:
                raise ValueError("original archive member hash differs")
            seen.add(member.name)
    if seen != set(manifest):
        raise ValueError("original archive lacks a manifest member")


def verify_map(base, manifest):
    for name, expected in manifest.items():
        path = base / name
        regular(path)
        if digest(path) != expected:
            raise ValueError("original differs from source manifest: " + str(path))


def closed_execution(path):
    item = read(path)
    if not item.get("finished_at_utc"):
        raise ValueError("execution is not closed: " + str(path))
    # Failed exit codes and recorded exceptions are preserved, never converted to success.
    return {"path": str(path), "exit_code": item.get("exit_code"),
            "execution_exception": item.get("execution_exception"), "finished_at_utc": item["finished_at_utc"]}


def measured_counts(root, frozen, freeze, evidence, assessments):
    # Invoked only during an authorized post-evaluation plan. No source/question text is returned.
    workload = read(frozen / "workload.json")
    if workload.get("partition") != "heldout":
        raise ValueError("sealed workload partition mismatch")
    cases = [case["id"] for case in workload["cases"]]
    conditions = list(freeze["configuration"]["conditions"])
    repeats = freeze["configuration"]["reader_repetitions_per_case_condition"]
    expected_pairs = {(case, condition) for case in cases for condition in conditions}
    actual_pairs = [(row["case_id"], row["condition"]) for row in evidence.get("cases", [])]
    issues = []
    if repeats != 1:
        issues.append("unexpected reader repetition shape: preserve the actual artifact inventory without inventing missing samples")
    if len(actual_pairs) != len(set(actual_pairs)) or set(actual_pairs) != expected_pairs:
        issues.append("reader case/condition coverage incomplete or duplicated")
    expected_assessments = {case + "-" + condition + ".json" for case, condition in expected_pairs}
    if set(assessments) != expected_assessments:
        issues.append("manual assessment coverage differs from frozen case/condition inventory")
    incomplete = []
    for path in sorted((root / (RUN + "-assessments")).glob("*.json")):
        item = read(path)
        if item.get("freeze_sha256") != digest(frozen / "freeze.json"):
            raise ValueError("assessment belongs to another freeze")
        if not item.get("review_complete"):
            incomplete.append(path.name)
    if incomplete:
        issues.append("incomplete manual assessment records remain preserved")
    actual_calls = sum(row.get("observed_model_calls", 0) for row in evidence.get("conditions", {}).values())
    case_calls = sum(row.get("model_calls", 0) for row in evidence.get("cases", []))
    encoded_calls = len(list((root / (RUN + "-reader")).glob("*-encoded-request.json")))
    wire_calls = len(list((root / (RUN + "-reader")).glob("*-wire-request.json")))
    if not actual_calls == case_calls == encoded_calls == wire_calls:
        issues.append("actual call totals disagree across audit, case rows and retained encoded/wire files")
    local_rows = 0
    local_pairs = set()
    local_failed = 0
    for path in sorted((root / (RUN + "-local")).glob("*-local-traces.jsonl.gz")):
        with gzip.open(path, "rt") as stream:
            for line in stream:
                if not line.strip():
                    continue
                item = json.loads(line)
                local_rows += 1
                local_pairs.add((item["case_id"], item["condition"]))
                local_failed += item.get("error") not in (None, "", "<nil>")
    local_expected = len(expected_pairs) * freeze["configuration"]["resources"]["local_repetitions_per_case_condition"]
    if local_rows != local_expected or local_pairs != expected_pairs:
        issues.append("local sample coverage incomplete")
    index_cases = [read(path)["case_id"] for path in sorted((root / (RUN + "-index")).glob("index-*.json"))]
    if len(index_cases) != len(set(index_cases)) or set(index_cases) != set(cases):
        issues.append("index case coverage incomplete or duplicated")
    operating_root = root / (RUN + "-operating") / "measurements"
    operating_config = read(operating_root / "configuration.json")
    operating_rows = 0
    with (operating_root / "samples.ndjson").open() as stream:
        operating_rows = sum(1 for line in stream if line.strip())
    operating_expected = len(operating_config["conditions"]) * operating_config["repetitions_per_condition"]
    if operating_rows != operating_expected:
        issues.append("operating sample coverage incomplete")
    return {"reader_turns_observed": len(actual_pairs), "reader_turns_planned": len(expected_pairs) * repeats,
            "case_count_from_sealed_workload": len(cases), "condition_count_from_freeze": len(conditions),
            "reader_http_calls_reported": actual_calls, "reader_calls_in_case_rows": case_calls,
            "encoded_request_files": encoded_calls, "wire_request_files": wire_calls,
            "local_turns_observed": local_rows, "local_turns_planned": local_expected,
            "local_failed_turns_retained": local_failed, "index_cases_observed": len(index_cases),
            "index_cases_planned": len(cases), "operating_turns_observed": operating_rows,
            "operating_turns_planned": operating_expected, "semantic_assessments_observed": len(assessments),
            "incomplete_assessments": incomplete, "coverage_or_accounting_gaps": issues}


def make_plan(root):
    root = Path(root).resolve()
    frozen = root / RUN
    freeze = read(frozen / "freeze.json")
    freeze_sha = digest(frozen / "freeze.json")
    if freeze["version"] != RUN or freeze["partition"] != "heldout":
        raise ValueError("this tool preserves only integrated-heldout-v1")
    if set(freeze.get("worker_environment") or {}) - {"GODEBUG", "GOGC", "GOMAXPROCS", "GOMEMLIMIT"}:
        raise ValueError("unexpected worker environment field; no environment will be exported")
    for name, manifest, field, hash_field in (
        ("compiled-source.tar.gz", "compiled-source-manifest.json", "compiled_source_archive_sha256", "compiled_source_manifest_sha256"),
        ("prepared-inputs.tar.gz", "input-manifest.json", "prepared_inputs_archive_sha256", "input_manifest_sha256")):
        if digest(frozen / manifest) != freeze[hash_field]:
            raise ValueError("original manifest differs from freeze")
        validate_original_archive(frozen / name, frozen / manifest, freeze[field])
    # Release sealing retains the original V4 source_root/binary_path and copies archives only.
    verify_map(Path(freeze["source_root"]), read(frozen / "compiled-source-manifest.json"))
    verify_map(Path(freeze["inputs_path"]), read(frozen / "input-manifest.json"))
    for key, value in freeze.items():
        if key.endswith("_path") and key[:-5] + "_sha256" in freeze:
            if digest(Path(value)) != freeze[key[:-5] + "_sha256"]:
                raise ValueError("frozen file hash mismatch: " + key)
    if digest(frozen / "run.py") != freeze["runner_sha256"]:
        raise ValueError("frozen runner hash mismatch")
    if freeze.get("same_development_executable") is not True:
        raise ValueError("release is not bound to the original executable")
    development = root / DEVELOPMENT
    if digest(development / "freeze.json") != freeze["development_freeze_sha256"]:
        raise ValueError("development freeze identity mismatch")
    development_report = root / (DEVELOPMENT + "-resource") / "report.json"
    if digest(development_report) != freeze["development_report_sha256"]:
        raise ValueError("development resource identity mismatch")
    executions = {}
    for mode in ["reader", "local", "index", "operating"]:
        executions[mode] = closed_execution(root / (RUN + "-" + mode) / "execution.json")
    for mode in ["evidence", "grade", "resource"]:
        executions[mode] = closed_execution(root / (RUN + "-" + mode + "-execution.json"))
    resource = read(root / (RUN + "-resource/report.json"))
    quality = read(root / (RUN + "-quality/quality-report.json"))
    evidence = read(root / (RUN + "-evidence/evidence-report.json"))
    for report in [resource, quality, evidence]:
        if report["freeze_sha256"] != freeze_sha:
            raise ValueError("post-run report belongs to another freeze")
    verify_map(root / (RUN + "-reader"), read(root / (RUN + "-evidence/results-manifest.json")))
    assessments = read(root / (RUN + "-assessment-manifest.json"))
    verify_map(root / (RUN + "-assessments"), assessments)
    counts = measured_counts(root, frozen, freeze, evidence, assessments)
    # Exact reports remain untouched. Preservation never promotes readiness over an observed failure.
    failed = bool(counts["coverage_or_accounting_gaps"] or any(e["exit_code"] != 0 or e["execution_exception"] for e in executions.values())
                  or resource.get("failures") or resource.get("complete") is not True or resource.get("all_required_gates_pass") is not True)
    derived_ready = resource.get("release_ready") is True and not failed
    direct, archives = {}, {}
    for path in sorted(frozen.rglob("*")):
        relative = path.relative_to(frozen)
        if not path.is_file() or relative.parts[0] in {"source", "inputs"} or relative == Path("integrated.test"):
            continue
        direct["frozen/" + str(relative)] = describe(path)
    for name, source in {"development-freeze.json": development / "freeze.json", "development-build.json": development / "build.json",
                         "development-resource-report.json": development_report}.items():
        direct["provenance/" + name] = describe(source)
    reports = {"reports/deterministic.json": RUN + "-deterministic.json",
               "reports/evidence-report.json": RUN + "-evidence/evidence-report.json",
               "reports/assessment-manifest.json": RUN + "-assessment-manifest.json",
               "reports/quality-report.json": RUN + "-quality/quality-report.json",
               "reports/resource-report.json": RUN + "-resource/report.json"}
    for name, original in reports.items():
        direct[name] = describe(root / original)
    for path in sorted(root.glob(RUN + "*")):
        if path.is_file() and path.name not in set(reports.values()):
            direct["commands/" + path.name] = describe(path)
    direct["preserve-integrated-heldout-v1.py"] = describe(Path(__file__).resolve())
    directories = dict(DIRECTORIES)
    directories["raw-assessment-groups.tar.gz"] = [p.name for p in sorted(root.glob(RUN + "-assessments-*")) if p.is_dir()]
    for name, directory_names in directories.items():
        members = {}
        for directory in directory_names:
            original = root / directory
            if not original.is_dir():
                raise ValueError("expected artifact directory missing: " + directory)
            for path in sorted(original.rglob("*")):
                relative = path.relative_to(original)
                if path.is_file() and relative.parts[0] not in {"source", "inputs"} and relative != Path("integrated.test"):
                    members[str(path.relative_to(root))] = describe(path)
        if name == "raw-verification.tar.gz":
            for relative in VERIFICATION_FILES:
                members[relative] = describe(root / relative)
            for pattern in OPTIONAL_PATTERNS:
                for path in sorted(root.glob(pattern)):
                    if path.is_file() and path.name != "168-verify-heldout-v1.py":
                        members[path.name] = describe(path)
            for optional in ["integrated-assessment-helper.py", "integrated-assessment-helper-notes.md"]:
                if (root / optional).is_file():
                    members[optional] = describe(root / optional)
        if name == "superseded-v3-checks.tar.gz":
            members["168-verify-heldout-v1.py"] = describe(root / "168-verify-heldout-v1.py")
        if name == "raw-assessments.tar.gz":
            members[RUN + "-assessment-manifest.json"] = describe(root / (RUN + "-assessment-manifest.json"))
        counter = CountHash(); archive_to(counter, members)
        archives[name] = {"members": members, "bytes": counter.bytes, "sha256": counter.sha256.hexdigest(),
                          "original_bytes": sum(item["bytes"] for item in members.values())}
    # Byte-level provenance capture only: the curation record includes source text and is never printed.
    curation_members = {str(path.relative_to(REPO)): describe(path) for path in sorted((REPO / CURATION_DIR).rglob("*")) if path.is_file()}
    if not curation_members:
        raise ValueError("independent curation provenance is missing")
    counter = CountHash(); archive_to(counter, curation_members)
    archives["curation-provenance.tar.gz"] = {"members": curation_members, "bytes": counter.bytes,
        "sha256": counter.sha256.hexdigest(), "original_bytes": sum(item["bytes"] for item in curation_members.values())}
    return {"schema_version": 3, "source_root": str(root), "freeze_sha256": freeze_sha,
            "tool_sha256": digest(__file__), "files": direct, "archives": archives, "repository_references": repo_references(),
            "repository_reference_policy": "Original approved v3/v4 apparatus regression records are references only; sealed heldout curation provenance is separately byte-preserved.",
            "planned_file_count": len(direct) + sum(len(item["members"]) for item in archives.values()),
            "original_selected_bytes": sum(item["bytes"] for item in direct.values()) + sum(item["original_bytes"] for item in archives.values()),
            "planned_payload_bytes": sum(item["bytes"] for item in direct.values()) + sum(item["bytes"] for item in archives.values()),
            "excluded": [{"source": freeze["binary_path"], "bytes": Path(freeze["binary_path"]).stat().st_size,
                          "sha256": freeze["binary_sha256"], "reason": "original development executable reused; source archive/build command retained"},
                         {"source": freeze["source_root"], "reason": "source bytes already in frozen/compiled-source.tar.gz"},
                         {"source": freeze["inputs_path"], "reason": "canonical inputs already in frozen/prepared-inputs.tar.gz"}],
            "superseded_checks": {"archive": "superseded-v3-checks.tar.gz", "release_binding": False,
                "reason": "These earlier checks exercised the superseded V3-only operating matrix. They are historical evidence, never release deterministic proof."},
            "completed_run": dict(counts, executions=executions, resource_report_release_ready=resource.get("release_ready"),
                preservation_observed_release_ready=derived_ready, resource_gate_count=len(resource.get("gates", [])),
                resource_failures=resource.get("failures"), qualification="Raw curation provenance and exposure records are preserved; use the separately authored qualification prose, not an inferred claim of universal blindness."),
            "reproduction": {"original_paths_timestamps_and_contents_unchanged": True,
                "source_archive": "frozen/compiled-source.tar.gz", "source_manifest": "frozen/compiled-source-manifest.json",
                "input_archive": "frozen/prepared-inputs.tar.gz", "input_manifest": "frozen/input-manifest.json",
                "runner": "frozen/run.py", "original_build_command": read(development / "build.json")["command"],
                "go_version": freeze["go_version"], "original_source_root": freeze["source_root"],
                "original_inputs_path": freeze["inputs_path"], "original_binary_path": freeze["binary_path"],
                "original_binary_sha256": freeze["binary_sha256"], "source_issue167_commit_at_seal": freeze["source_issue167_commit"],
                "development_freeze_sha256": freeze["development_freeze_sha256"],
                "model_and_context_metadata": "frozen/freeze.json", "binary_rebuild_attempted_by_preservation_tool": False,
                "limits": ["No program, corpus, threshold or output is modified by preservation.",
                           "All actual failures and incomplete cohorts remain in the retained reports and counts.",
                           "Compiled executable and model weights are excluded; original source inputs and acquisition instructions remain available.",
                           "Historical absolute paths and timestamps are unchanged; relocation requires a new reproduction record.",
                           "New gzip wrapper time is zero; original member bytes, mode and nanosecond modification times are retained."]}}


def make_failed_setup_plan(root):
    """Preserve an unsealed attempt only after preparation and its outer seal command close."""
    root = Path(root).resolve()
    attempted = root / RUN
    if (attempted / "freeze.json").exists():
        raise ValueError("attempt has a freeze; use the sealed-run plan command instead")
    development = root / DEVELOPMENT
    frozen = read(development / "freeze.json")
    preparation = closed_execution(attempted / "prepare-execution.json")
    seal = closed_execution(root / (RUN + "-seal-execution.json"))
    if preparation["exit_code"] == 0 or seal["exit_code"] == 0:
        raise ValueError("failed-setup mode requires the observed failed preparation and seal")
    unexpected_outputs = [mode for mode in ["index", "local", "operating", "reader"]
                          if (root / (RUN + "-" + mode)).exists()]
    if unexpected_outputs:
        raise ValueError("unsealed attempt unexpectedly has evaluation outputs: " + str(unexpected_outputs))
    validate_original_archive(attempted / "compiled-source.tar.gz", attempted / "compiled-source-manifest.json",
                              frozen["compiled_source_archive_sha256"])
    if digest(attempted / "compiled-source-manifest.json") != frozen["compiled_source_manifest_sha256"]:
        raise ValueError("attempted source manifest differs from exact V4 executable inputs")
    if digest(Path(frozen["binary_path"])) != frozen["binary_sha256"]:
        raise ValueError("original V4 executable changed")
    verify_map(Path(frozen["source_root"]), read(attempted / "compiled-source-manifest.json"))
    if digest(attempted / "run.py") != frozen["runner_sha256"]:
        raise ValueError("attempted runner differs from exact V4")
    for field in ["gates", "rubric", "operating", "matrix", "index", "scorer", "auditor", "grader", "assessment", "procedure", "resources"]:
        name = ("audit_sources" if field == "auditor" else field) + Path(frozen[field + "_path"]).suffix
        if digest(attempted / name) != frozen[field + "_sha256"]:
            raise ValueError("attempted immutable program/document differs from V4: " + field)
    direct, archives = {}, {}
    for path in sorted(attempted.rglob("*")):
        relative = path.relative_to(attempted)
        if not path.is_file() or relative.parts[0] in {"source", "inputs"} or relative == Path("integrated.test"):
            continue
        direct["attempt/" + str(relative)] = describe(path)
    for name in ["freeze.json", "build.json"]:
        direct["provenance/development-" + name] = describe(development / name)
    direct["provenance/development-resource-report.json"] = describe(root / (DEVELOPMENT + "-resource/report.json"))
    for path in sorted(root.glob(RUN + "*")):
        if path.is_file():
            direct["commands/" + path.name] = describe(path)
    for name in VERIFICATION_FILES:
        direct["commands/" + name] = describe(root / name)
    direct["preserve-integrated-heldout-v1.py"] = describe(Path(__file__).resolve())
    sources = {
        "partial-prepared-inputs.tar.gz": attempted / "inputs",
        "preparation-final-checks.tar.gz": root / (RUN + "-final-checks"),
        "superseded-v3-checks.tar.gz": root / (RUN + "-checks"),
        "curation-provenance.tar.gz": REPO / CURATION_DIR,
    }
    for name, directory in sources.items():
        if not directory.is_dir():
            raise ValueError("expected failed-setup capture directory missing: " + str(directory))
        members = {str(path.relative_to(directory)): describe(path) for path in sorted(directory.rglob("*")) if path.is_file()}
        if name == "superseded-v3-checks.tar.gz":
            members["168-verify-heldout-v1.py"] = describe(root / "168-verify-heldout-v1.py")
        if name == "preparation-final-checks.tar.gz":
            for pattern in OPTIONAL_PATTERNS:
                for path in sorted(root.glob(pattern)):
                    if path.is_file() and path.name != "168-verify-heldout-v1.py":
                        members[path.name] = describe(path)
        counter = CountHash(); archive_to(counter, members)
        archives[name] = {"members": members, "bytes": counter.bytes, "sha256": counter.sha256.hexdigest(),
                          "original_bytes": sum(item["bytes"] for item in members.values())}
    partial_files = archives["partial-prepared-inputs.tar.gz"]["members"]
    return {"schema_version": 3, "kind": "failed_setup_unsealed", "source_root": str(root),
            "freeze_sha256": None, "tool_sha256": digest(__file__), "files": direct, "archives": archives,
            "repository_references": repo_references(),
            "repository_reference_policy": "Prior v3/v4 apparatus references only; this preparation attempt produced no heldout freeze or assessment.",
            "planned_file_count": len(direct) + sum(len(item["members"]) for item in archives.values()),
            "original_selected_bytes": sum(item["bytes"] for item in direct.values()) + sum(item["original_bytes"] for item in archives.values()),
            "planned_payload_bytes": sum(item["bytes"] for item in direct.values()) + sum(item["bytes"] for item in archives.values()),
            "failed_setup": {"sealed": False, "release_ready": False, "preparation": preparation, "outer_seal": seal,
                "reader_generations": 0, "index_local_operating_evaluations_started": False,
                "partial_input_files": len(partial_files),
                "partial_seed_database_files": sum(name.endswith("/seed.db") for name in partial_files),
                "partial_seed_mapping_files": sum(name.endswith("/seed.json") for name in partial_files),
                "counts_are_observed_files_not_completed_cases": True,
                "failure_detail_source": "attempt/prepare-execution.log",
                "meaning": "Canonical preparation failed before freeze.json. Partial input files are losslessly retained; they are not sealed evaluation inputs, measured retrieval results, or semantic assessments."},
            "superseded_checks": {"archive": "superseded-v3-checks.tar.gz", "release_binding": False,
                                  "reason": "Earlier V3-only checks are historical evidence and cannot certify release."},
            "reproduction": {"development_freeze_sha256": digest(development / "freeze.json"),
                "original_binary_sha256": frozen["binary_sha256"], "original_binary_path": frozen["binary_path"],
                "original_source_root": frozen["source_root"], "source_archive": "attempt/compiled-source.tar.gz",
                "source_manifest": "attempt/compiled-source-manifest.json", "build": "provenance/development-build.json",
                "binary_rebuild_attempted_by_preservation_tool": False,
                "all_original_bytes_paths_and_timestamps_unchanged": True},
            "excluded": [{"source": frozen["binary_path"], "sha256": frozen["binary_sha256"],
                          "reason": "Exact V4 executable identity retained; executable excluded from source-only evidence archive"},
                         {"source": frozen["source_root"], "reason": "Already retained in original compiled-source archive"}]}


def copy_plan(plan_path, destination):
    plan = read(plan_path)
    if plan["tool_sha256"] != digest(__file__):
        raise ValueError("preservation tool changed after planning")
    destination = Path(destination).resolve()
    source_root = Path(plan["source_root"])
    if destination == source_root or destination.is_relative_to(source_root):
        raise ValueError("copy destination must be outside original evidence directory")
    for relative, item in plan.get("repository_references", {}).items():
        if digest(REPO / relative) != item["sha256"]:
            raise ValueError("referenced repository record changed after plan: " + relative)
    destination.mkdir(parents=True, exist_ok=False)
    outputs = {}
    for name, item in plan["files"].items():
        path = Path(item["source"])
        if digest(path) != item["sha256"] or path.stat().st_size != item["bytes"] or path.stat().st_mtime_ns != item["mtime_ns"]:
            raise ValueError("original changed after plan: " + str(path))
        target = destination / name
        target.parent.mkdir(parents=True, exist_ok=True)
        with path.open("rb") as source, target.open("xb") as output:
            shutil.copyfileobj(source, output)
        shutil.copystat(path, target)
        outputs[name] = {"bytes": target.stat().st_size, "sha256": digest(target)}
        if outputs[name] != {key: item[key] for key in ("bytes", "sha256")}:
            raise ValueError("copied original changed")
    for name, item in plan["archives"].items():
        target = destination / name
        with target.open("xb") as stream:
            archive_to(stream, item["members"])
        outputs[name] = {"bytes": target.stat().st_size, "sha256": digest(target)}
        if outputs[name] != {key: item[key] for key in ("bytes", "sha256")}:
            raise ValueError("archive differs from planned exact members")
    write_new(destination / "preservation-plan.json", plan)
    outputs["preservation-plan.json"] = {"bytes": (destination / "preservation-plan.json").stat().st_size,
                                        "sha256": digest(destination / "preservation-plan.json")}
    write_new(destination / "preservation-manifest.json", {"schema_version": 1, "freeze_sha256": plan["freeze_sha256"],
              "files": outputs, "payload_bytes": sum(item["bytes"] for item in outputs.values()),
              "original_files": plan["planned_file_count"], "all_copy_hashes_verified": True})
    return verify_copy(destination)


def verify_copy(destination):
    destination = Path(destination).resolve()
    manifest = read(destination / "preservation-manifest.json")
    plan = read(destination / "preservation-plan.json")
    actual = {str(path.relative_to(destination)) for path in destination.rglob("*") if path.is_file()}
    if actual != set(manifest["files"]) | {"preservation-manifest.json"}:
        raise ValueError("preserved output has missing or unexpected files")
    for name, item in manifest["files"].items():
        path = destination / name
        regular(path)
        if digest(path) != item["sha256"] or path.stat().st_size != item["bytes"]:
            raise ValueError("preserved file differs from manifest: " + name)
    for name, item in plan["files"].items():
        path = destination / name
        if path.stat().st_mtime_ns != item["mtime_ns"] or (path.stat().st_mode & 0o777) != item["mode"]:
            raise ValueError("preserved direct file time/mode differs: " + name)
    reference_root = next((parent for parent in destination.parents if (parent / REPO_REGRESSION_DIR).is_dir()), None)
    if plan.get("repository_references") and reference_root is None:
        raise ValueError("repository regression references are required; verify from the repository artifact directory")
    for relative, item in plan.get("repository_references", {}).items():
        if digest(reference_root / relative) != item["sha256"]:
            raise ValueError("referenced repository record hash differs: " + relative)
    count = len(plan["files"])
    for name, archive_info in plan["archives"].items():
        seen = set()
        with tarfile.open(destination / name, "r:gz") as archive:
            for member in archive:
                if not member.isfile() or member.name not in archive_info["members"] or member.name in seen:
                    raise ValueError("unexpected preserved archive member")
                wanted = archive_info["members"][member.name]
                if member.size != wanted["bytes"] or hashlib.sha256(archive.extractfile(member).read()).hexdigest() != wanted["sha256"]:
                    raise ValueError("preserved raw archive member differs: " + member.name)
                expected_mtime = str(wanted["mtime_ns"] // 1_000_000_000) + "." + str(wanted["mtime_ns"] % 1_000_000_000).zfill(9)
                if member.pax_headers.get("mtime") != expected_mtime or member.mode != wanted["mode"]:
                    raise ValueError("preserved raw archive member time/mode differs: " + member.name)
                seen.add(member.name)
        if seen != set(archive_info["members"]):
            raise ValueError("preserved archive lacks original members")
        count += len(seen)
    if plan.get("kind") == "failed_setup_unsealed":
        freeze = read(destination / "provenance/development-freeze.json")
        validate_original_archive(destination / "attempt/compiled-source.tar.gz", destination / "attempt/compiled-source-manifest.json", freeze["compiled_source_archive_sha256"])
    else:
        for name, map_name, field in (("compiled-source.tar.gz", "compiled-source-manifest.json", "compiled_source_archive_sha256"),
                                      ("prepared-inputs.tar.gz", "input-manifest.json", "prepared_inputs_archive_sha256")):
            freeze = read(destination / "frozen/freeze.json")
            validate_original_archive(destination / "frozen" / name, destination / "frozen" / map_name, freeze[field])
    return {"verified_original_files": count, "output_files": len(actual), "payload_bytes": manifest["payload_bytes"],
            "freeze_sha256": manifest["freeze_sha256"], "model_calls": 0, "binary_retained": False, "all_member_hashes_times_modes_verified": True, "repository_records_verified": len(plan.get("repository_references", {}))}


def inventory(root):
    root = Path(root).resolve()
    expected = [RUN + "/freeze.json", RUN + "/run.py", RUN + "/compiled-source.tar.gz",
                RUN + "/compiled-source-manifest.json", RUN + "/prepared-inputs.tar.gz", RUN + "/input-manifest.json",
                RUN + "-deterministic.json", RUN + "-assessment-manifest.json", RUN + "-evidence/evidence-report.json",
                RUN + "-quality/quality-report.json", RUN + "-resource/report.json"] + VERIFICATION_FILES
    expected += [RUN + "-" + mode + "/execution.json" for mode in ["reader", "local", "index", "operating"]]
    expected += [RUN + "-" + mode + "-execution.json" for mode in ["evidence", "grade", "resource"]]
    present, missing = [], []
    for name in sorted(set(expected)):
        (present if (root / name).is_file() else missing).append(name)
    return {"schema_version": 1, "source_root": str(root), "status": "metadata-only inventory; no workload or answer content read",
            "directories": DIRECTORIES, "required_files_present": present, "expected_files_missing": missing,
            "optional_patterns": OPTIONAL_PATTERNS, "curation_directory_byte_capture": str(CURATION_DIR),
            "exclusions": ["compiled executables", "unpacked source and canonical inputs already archived"],
            "copy_preconditions": ["all measurement and final source/grade/resource executions closed",
                                   "original frozen source/input archives and metadata hashes intact"],
            "count_policy": "Later authorized planning derives frozen planned and actual observed cohorts separately; missing or failed samples are never normalized to success.",
            "superseded_policy": "integrated-heldout-v1-checks and its old script are preserved in a separate historical archive, never release proof."}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="mode", required=True)
    listing = commands.add_parser("inventory")
    listing.add_argument("--source", default="/tmp/evie-memory-stage5")
    listing.add_argument("--output", required=True)
    planning = commands.add_parser("plan")
    planning.add_argument("--source", default="/tmp/evie-memory-stage5")
    planning.add_argument("--output", required=True)
    failed = commands.add_parser("failed-setup-plan")
    failed.add_argument("--source", default="/tmp/evie-memory-stage5")
    failed.add_argument("--output", required=True)
    copying = commands.add_parser("copy")
    copying.add_argument("--plan", required=True)
    copying.add_argument("--output", required=True)
    checking = commands.add_parser("verify")
    checking.add_argument("--directory", required=True)
    args = parser.parse_args()
    if args.mode == "inventory":
        result = inventory(args.source)
        write_new(Path(args.output).resolve(), result)
        print(json.dumps({"present": len(result["required_files_present"]), "missing": result["expected_files_missing"]}))
    elif args.mode == "plan":
        result = make_plan(args.source)
        write_new(Path(args.output).resolve(), result)
        print(json.dumps({key: result[key] for key in ("planned_file_count", "original_selected_bytes", "planned_payload_bytes", "freeze_sha256")}))
    elif args.mode == "failed-setup-plan":
        result = make_failed_setup_plan(args.source)
        write_new(Path(args.output).resolve(), result)
        print(json.dumps({key: result[key] for key in ("planned_file_count", "original_selected_bytes", "planned_payload_bytes", "freeze_sha256")}))
    elif args.mode == "copy":
        print(json.dumps(copy_plan(args.plan, args.output)))
    else:
        print(json.dumps(verify_copy(args.directory)))


if __name__ == "__main__":
    main()
