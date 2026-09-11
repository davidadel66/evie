#!/usr/bin/env python3
"""Preserve and run an integrated development/release evaluation with stdlib only.

Freeze is completed before any measured reader query. All destinations are new;
failed attempts remain available and cannot be overwritten by this runner.
"""

import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import ipaddress
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import urllib.parse
import urllib.request


RETRIEVAL_CONFIGURATION = {
    "reader_guide": "memory-retrieval-v2",
    "automatic_planner": "bounded-reference-lexical-v2",
    "store_query_deadline_ms": 500,
    "agent_read_deadline_ms": 750,
    "query_bytes_max": 1024,
    "query_terms_max": 32,
    "initial_results_per_kind": 2,
    "initial_result_bytes_per_kind": 6144,
    "current_request_bytes_examined": 512,
    "earlier_owner_roots_examined": 16,
    "earlier_root_bytes_max": 384,
    "earlier_roots_selected": 2,
    "compaction_continuity_bytes_max": 512,
    "explicit_identity_selectors_max": 2,
    "explicit_identity_selector_bytes_max": 128,
    "shared_candidate_limit": 64,
    "exact_candidate_limit": 16,
    "lexical_candidate_limit": 24,
    "temporal_candidate_limit": 8,
    "graph_anchors": 4,
    "graph_width": 16,
    "graph_depth": 2,
    "graph_paths_per_claim": 2,
    "rrf_k": 60,
    "dense_model": "all-minilm:22m",
    "dense_dimensions": 384,
    "dense_vector_encoding": "L2 normalized float32",
    "dense_minimum_cosine": 0.25,
    "dense_candidates": 8,
    "dense_vector_scan_limit": 4096,
    "dense_chunk_bytes": 240,
    "dense_chunk_overlap_bytes": 48,
    "dense_query_deadline": "min(250ms, half remaining Store deadline)",
    "active_dense_raw_lexical_candidate_limit": 24,
    "inactive_dense_raw_lexical_candidate_limit": 64,
    "maintenance_rows_per_batch": 256,
    "maintenance_embedding_inputs_per_batch": 16,
    "embedding_context_tokens": 256,
    "embedding_threads": 4,
    "embedding_keep_alive": "30s",
    "embedding_truncate": False,
    "embedding_input_bytes_max": 1024,
    "embedding_response_bytes_max": 1048576,
    "embedding_operation_deadline_ms": 10000,
    "changed_conflict_peers_per_held_claim": 8,
    "new_owner_suggestions_per_held_claim": 8,
    "expansion_sequence_positions": 64,
    "semantic_authority": "canonical accepted state and exact original source policy; scores propose only",
}


WORKER_ENVIRONMENT = {"GOMAXPROCS": str(os.cpu_count()), "GOGC": "100",
                      "GOMEMLIMIT": "off", "GODEBUG": ""}


def worker_environment(frozen=None):
    environment = os.environ.copy()
    for name in ("EVIE_MEMORY_INTEGRATED_FREEZE", "EVIE_MEMORY_INTEGRATED_INPUTS",
                 "EVIE_MEMORY_INTEGRATED_WORKLOAD", "EVIE_MEMORY_READER_ARTIFACTS",
                 "EVIE_MEMORY_INTEGRATED_OPERATING_OUTPUT", "EVIE_RUN_PRODUCTION_READER_PREFLIGHT"):
        environment.pop(name, None)
    environment.update(frozen["worker_environment"] if frozen else WORKER_ENVIRONMENT)
    return environment


def sha256(path):
    result = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            result.update(block)
    return result.hexdigest()


def write_json(path, value):
    with Path(path).open("x") as stream:
        json.dump(value, stream, indent=2, sort_keys=True)
        stream.write("\n")


def tree_hashes(directory):
    return {str(path.relative_to(directory)): sha256(path)
            for path in sorted(Path(directory).rglob("*")) if path.is_file()}


def run_command(command, directory, output, environment=None):
    started = datetime.now(timezone.utc).isoformat()
    record = {"command": command, "working_directory": str(directory),
              "started_at_utc": started}
    write_json(Path(str(output) + "-started.json"), record)
    exit_code = None
    failure = None
    try:
        with Path(str(output) + ".log").open("x") as stream:
            result = subprocess.run(command, cwd=directory, env=environment,
                                    stdout=stream, stderr=subprocess.STDOUT)
        exit_code = result.returncode
    except BaseException as error:
        failure = {"type": type(error).__name__, "detail": str(error)}
        raise
    finally:
        write_json(Path(str(output) + ".json"), {
            **record, "finished_at_utc": datetime.now(timezone.utc).isoformat(),
            "exit_code": exit_code, "execution_exception": failure,
        })
    if exit_code:
        raise RuntimeError(f"command failed ({exit_code}); retained {output}.log")
    requested_tests = [arg.removeprefix("-test.run=^").removesuffix("$")
                       for arg in command if arg.startswith("-test.run=^")]
    log = Path(str(output) + ".log").read_text()
    if any("--- PASS: " + name + " " not in log for name in requested_tests):
        raise RuntimeError(f"requested evaluation did not pass or was skipped; retained {output}.log")


def compiled_paths(root):
    encoded = subprocess.check_output(
        ["go", "list", "-deps", "-test", "-json", "./internal/agent"],
        cwd=root, text=True)
    decoder = json.JSONDecoder()
    paths = {"go.mod", "go.sum"}
    fields = ("GoFiles", "CgoFiles", "TestGoFiles", "XTestGoFiles", "EmbedFiles",
              "TestEmbedFiles", "XTestEmbedFiles", "SFiles", "HFiles", "CFiles",
              "CXXFiles", "MFiles", "FFiles", "SysoFiles")
    while encoded.strip():
        package, consumed = decoder.raw_decode(encoded.lstrip())
        encoded = encoded.lstrip()[consumed:]
        directory = Path(package.get("Dir", "/"))
        if not directory.is_relative_to(root):
            continue
        for field in fields:
            for name in package.get(field, []):
                path = directory / name
                if path.is_relative_to(root) and path.is_file():
                    paths.add(str(path.relative_to(root)))
    return sorted(paths)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, file, code, message, headers, newurl):
        raise RuntimeError("local metadata redirects are forbidden")


def local_metadata(endpoint, suffix):
    parsed = urllib.parse.urlsplit(endpoint)
    if (parsed.scheme != "http" or parsed.username or parsed.password or parsed.path
            or parsed.query or parsed.fragment
            or not ipaddress.ip_address(parsed.hostname).is_loopback):
        raise RuntimeError("metadata requires literal loopback HTTP")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    with opener.open(endpoint + suffix, timeout=10) as response:
        data = response.read((1 << 20) + 1)
        if response.status != 200 or len(data) > 1 << 20:
            raise RuntimeError("invalid bounded local metadata response")
        return json.loads(data)


def model_identity(args):
    models = Path(args.models).resolve()
    manifest_path = models / "manifests/registry.ollama.ai/library/all-minilm/22m"
    manifest = json.loads(manifest_path.read_text())
    manifest_sha = sha256(manifest_path)
    if manifest_sha != "1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef":
        raise RuntimeError("selected MiniLM manifest changed")
    blobs = {}
    for item in [manifest["config"], *manifest["layers"]]:
        path = models / "blobs" / item["digest"].replace(":", "-")
        actual = sha256(path)
        if actual != item["digest"].split(":", 1)[1]:
            raise RuntimeError("local model blob does not match manifest")
        blobs[actual] = path.stat().st_size
    tags = local_metadata(args.endpoint, "/api/tags")
    if not any(model["name"] == "all-minilm:22m"
               and model["digest"].removeprefix("sha256:") == manifest_sha
               for model in tags.get("models", [])):
        raise RuntimeError("served MiniLM tag differs from selected manifest")
    version = local_metadata(args.endpoint, "/api/version")
    runtime_sha = sha256(args.runtime)
    if version.get("version") != "0.6.3" or runtime_sha != "2794d30cf43308d6ce368d2827f09b2b6b2e804960e9f5768b3b603f1ad93a8f":
        raise RuntimeError("selected Ollama runtime changed")
    return {"manifest_sha256": manifest_sha, "manifest": manifest,
            "blobs_sha256_bytes": blobs, "runtime": version,
            "runtime_path": str(Path(args.runtime).resolve()),
            "runtime_sha256": runtime_sha}


def archive(root, names, destination):
    with tarfile.open(destination, "x:gz") as output:
        for name in sorted(names):
            output.add(root / name, arcname=name, recursive=False)


def freeze(args):
    root = Path(args.repository).resolve()
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=False)
    source = output / "source"
    source.mkdir()
    shutil.copyfile(__file__, output / "run.py")
    documents = {name: Path(getattr(args, name)).resolve()
                 for name in ("workload", "gates", "rubric", "operating", "matrix", "index", "scorer", "auditor", "grader", "assessment", "procedure", "resources")}
    before = {name: sha256(root / name) for name in compiled_paths(root)}
    for name in before:
        target = source / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(root / name, target)
    for name, path in documents.items():
        shutil.copyfile(path, output / (("audit_sources" if name == "auditor" else name) + path.suffix))
    write_json(output / "compiled-source-manifest.json", before)
    binary = output / "integrated.test"
    run_command(["go", "test", "-c", "./internal/agent", "-o", str(binary)],
                source, output / "build", worker_environment())
    if {name: sha256(root / name) for name in before} != before:
        raise RuntimeError("source changed during freeze build; attempt retained")
    archive(source, before, output / "compiled-source.tar.gz")
    model = model_identity(args)
    inputs = output / "inputs"
    preflight = output / "preflight"
    preflight.mkdir()
    env = worker_environment()
    env.update({"EVIE_RUN_PRODUCTION_READER_PREFLIGHT": "1",
                "EVIE_MEMORY_READER_ARTIFACTS": str(preflight)})
    run_command([str(binary), "-test.run=^TestMemoryStage5ProductionReaderPreflight$",
                 "-test.count=1", "-test.v"], source, output / "preflight-execution", env)
    profile = json.loads((preflight / "preflight-context-profile.json").read_text())
    if (profile["CanonicalModel"] != "openai/gpt-6-astra-20260903"
            or profile["WorkingTokens"] != 24576 or profile["OutputReserveTokens"] != 768):
        raise RuntimeError("reader identity/profile differs from declared development configuration")
    env = worker_environment()
    env.update({"EVIE_MEMORY_INTEGRATED_INPUTS": str(inputs),
                "EVIE_MEMORY_INTEGRATED_WORKLOAD": str(output / "workload.json"),
                "EVIE_MEMORY_EMBEDDING_ENDPOINT": args.endpoint})
    write_json(output / "model-state-before-preparation.json", {
        "observed_at_utc": datetime.now(timezone.utc).isoformat(),
        "model_state": local_metadata(args.endpoint, "/api/ps")})
    run_command([str(binary), "-test.run=^TestMemoryStage5IntegratedPrepare$",
                 "-test.count=1", "-test.v", "-test.timeout=30m"],
                source, output / "prepare-execution", env)
    write_json(output / "model-state-after-preparation.json", {
        "observed_at_utc": datetime.now(timezone.utc).isoformat(),
        "model_state": local_metadata(args.endpoint, "/api/ps")})
    input_hashes = tree_hashes(inputs)
    if not input_hashes or not any(name.endswith("/seed.db") for name in input_hashes):
        raise RuntimeError("prepare did not retain canonical seed databases")
    write_json(output / "input-manifest.json", input_hashes)
    archive(inputs, input_hashes, output / "prepared-inputs.tar.gz")
    hardware = {"platform": platform.platform(), "machine": platform.machine(),
                "logical_cpus": os.cpu_count()}
    for key in ("hw.model", "hw.memsize", "hw.physicalcpu"):
        hardware[key] = subprocess.check_output(["sysctl", "-n", key], text=True).strip()
    configuration = json.loads((output / "gates.json").read_text())
    workload = json.loads((output / "workload.json").read_text())
    if workload["partition"] != args.partition or len(workload["cases"]) != configuration["reader_cases"]:
        raise RuntimeError("workload partition/count differs from declared gate cohort")
    if any(len(case["gold"]["support_sets"]) != 1 for case in workload["cases"]):
        raise RuntimeError("this frozen evaluation procedure requires one predeclared minimal support set per case")
    write_json(output / "freeze.json", {
        "version": args.version, "partition": args.partition,
        "frozen_at_utc": datetime.now(timezone.utc).isoformat(),
        "source_root": str(source), "binary_path": str(binary),
        "binary_sha256": sha256(binary), "runner_sha256": sha256(__file__),
        "compiled_source_manifest_sha256": sha256(output / "compiled-source-manifest.json"),
        "compiled_source_archive_sha256": sha256(output / "compiled-source.tar.gz"),
        "inputs_path": str(inputs), "input_manifest_sha256": sha256(output / "input-manifest.json"),
        "prepared_inputs_archive_sha256": sha256(output / "prepared-inputs.tar.gz"),
        "workload_path": str(output / "workload.json"), "workload_sha256": sha256(output / "workload.json"),
        "gates_path": str(output / "gates.json"), "gates_sha256": sha256(output / "gates.json"),
        "rubric_path": str(output / "rubric.md"), "rubric_sha256": sha256(output / "rubric.md"),
        "operating_path": str(output / "operating.md"), "operating_sha256": sha256(output / "operating.md"),
        "matrix_path": str(output / "matrix.json"), "matrix_sha256": sha256(output / "matrix.json"),
        "index_path": str(output / "index.md"), "index_sha256": sha256(output / "index.md"),
        "scorer_path": str(output / "scorer.py"), "scorer_sha256": sha256(output / "scorer.py"),
        "auditor_path": str(output / "audit_sources.py"), "auditor_sha256": sha256(output / "audit_sources.py"),
        **{field + suffix: value for field in ("grader", "assessment", "procedure", "resources")
           for suffix, value in (("_path", str(output / (field + documents[field].suffix))),
                                 ("_sha256", sha256(output / (field + documents[field].suffix))))},
        "profile": profile, "profile_sha256": sha256(preflight / "preflight-context-profile.json"),
        "model": model, "endpoint": args.endpoint, "hardware": hardware,
        "worker_environment": WORKER_ENVIRONMENT,
        "go_version": subprocess.check_output(["go", "version"], text=True).strip(),
        "configuration": configuration,
        "retrieval_configuration": RETRIEVAL_CONFIGURATION,
        "source_head_before_issue_fix_folding": subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=root, text=True).strip(),
        "condition_order": "rotate six conditions by zero-based case index",
        "measurement_order": ["index", "local", "operating", "reader"],
        "provider": "actual production reader, low reasoning; model controls every non-oracle read from request one",
    })
    print(json.dumps({"freeze": str(output / "freeze.json"), "binary_sha256": sha256(binary)}))


def verify_freeze(path):
    path = Path(path).resolve()
    frozen = json.loads(path.read_text())
    base = path.parent
    checks = {Path(frozen["binary_path"]): frozen["binary_sha256"],
              Path(__file__): frozen["runner_sha256"],
              base / "compiled-source-manifest.json": frozen["compiled_source_manifest_sha256"],
              base / "input-manifest.json": frozen["input_manifest_sha256"],
              base / "compiled-source.tar.gz": frozen["compiled_source_archive_sha256"],
              base / "prepared-inputs.tar.gz": frozen["prepared_inputs_archive_sha256"]}
    for field in ("workload", "gates", "rubric", "operating", "matrix", "index", "scorer", "auditor", "grader", "assessment", "procedure", "resources"):
        checks[Path(frozen[field + "_path"])] = frozen[field + "_sha256"]
    for file, expected in checks.items():
        if sha256(file) != expected:
            raise RuntimeError(f"frozen input changed: {file}")
    source = Path(frozen["source_root"])
    sources = json.loads((base / "compiled-source-manifest.json").read_text())
    if {name: sha256(source / name) for name in sources} != sources:
        raise RuntimeError("compiled source snapshot changed")
    inputs = Path(frozen["inputs_path"])
    if tree_hashes(inputs) != json.loads((base / "input-manifest.json").read_text()):
        raise RuntimeError("prepared canonical inputs changed")
    if sha256(frozen["model"]["runtime_path"]) != frozen["model"]["runtime_sha256"]:
        raise RuntimeError("local runtime executable changed after freeze")
    if local_metadata(frozen["endpoint"], "/api/version") != frozen["model"]["runtime"]:
        raise RuntimeError("served runtime version changed after freeze")
    return frozen


def run(args):
    path = Path(args.freeze).resolve()
    frozen = verify_freeze(path)
    base = path.parent
    source = Path(frozen["source_root"])
    inputs = Path(frozen["inputs_path"])
    output = Path(args.output).resolve()
    if output.exists():
        raise RuntimeError("refuse to overwrite any previous attempt")
    output.mkdir(parents=True)
    env = worker_environment(frozen)
    env.update({"EVIE_MEMORY_INTEGRATED_INPUTS": str(inputs),
                "EVIE_MEMORY_INTEGRATED_FREEZE": str(path),
                "EVIE_MEMORY_READER_ARTIFACTS": str(output),
                "EVIE_MEMORY_EMBEDDING_ENDPOINT": frozen["endpoint"]})
    if args.mode == "operating":
        env["EVIE_MEMORY_INTEGRATED_OPERATING_OUTPUT"] = str(output / "measurements")
    test = {"reader": "TestMemoryStage5IntegratedReaderEvaluation",
            "local": "TestMemoryStage5IntegratedLocalEvaluation",
            "index": "TestMemoryStage5IntegratedIndexMeasurements",
            "operating": "TestMemoryStage5IntegratedOperatingMeasurements"}[args.mode]
    write_json(output / "execution-freeze.json", {
        "freeze_sha256": sha256(path), "mode": args.mode,
        "binary_sha256": frozen["binary_sha256"], "inputs_verified_before_run": True,
        "model_state_observed_at_utc": datetime.now(timezone.utc).isoformat(),
        "selected_model_state_before_run": local_metadata(frozen["endpoint"], "/api/ps")})
    try:
        run_command([frozen["binary_path"], "-test.run=^" + test + "$", "-test.count=1",
                     "-test.v", "-test.timeout=3h"], source, output / "execution", env)
    finally:
        unchanged = tree_hashes(inputs) == json.loads((base / "input-manifest.json").read_text())
        write_json(output / "input-integrity-after-run.json", {"unchanged": unchanged})
        if not unchanged:
            raise RuntimeError("run mutated frozen canonical inputs")
    print(json.dumps({"mode": args.mode, "output": str(output), "exit_code": 0}))


def verified_development_report(freeze_path, frozen, report_path):
    report_path = Path(report_path).resolve()
    report = json.loads(report_path.read_text())
    if (report.get("freeze_sha256") != sha256(freeze_path)
            or report.get("partition") != "development"
            or report.get("complete") is not True
            or report.get("all_required_gates_pass") is not True
            or report.get("release_ready") is not False
            or report.get("resource_aggregator_sha256") != frozen["resources_sha256"]
            or report.get("failures") != []
            or not report.get("gates")
            or any(gate.get("status") != "pass" for gate in report["gates"])):
        raise RuntimeError("complete passing development report must bind this exact freeze")
    inputs = report.get("measurement_inputs", {})
    required = {"freeze", "evidence_report", "quality_report", "local", "index", "operating", "deterministic"}
    if (set(inputs) != required
            or Path(inputs["freeze"]).resolve() != Path(freeze_path).resolve()
            or not report.get("input_sha256")):
        raise RuntimeError("development report lacks its exact measurement inputs")
    for path, expected in report["input_sha256"].items():
        if sha256(path) != expected:
            raise RuntimeError("development report input changed: " + path)
    if sha256(frozen["resources_path"]) != frozen["resources_sha256"]:
        raise RuntimeError("development resource auditor changed")
    spec = importlib.util.spec_from_file_location("verified_development_resources", frozen["resources_path"])
    resources = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(resources)
    if resources.build_report(**inputs) != report:
        raise RuntimeError("development report differs from independent frozen re-computation")
    return report


def seal_release(args):
    development_path = Path(args.development_freeze).resolve()
    frozen = verify_freeze(development_path)
    if frozen["partition"] != "development":
        raise RuntimeError("release must start from the verified development freeze")
    report_path = Path(args.development_report).resolve()
    verified_development_report(development_path, frozen, report_path)
    repository = Path(args.repository).resolve()
    subject = subprocess.check_output(["git", "log", "-1", "--format=%s"], cwd=repository, text=True)
    if "#167" not in subject:
        raise RuntimeError("commit the verified #167 pilot before sealing the release inputs")
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=False)
    shutil.copyfile(__file__, output / "run.py")
    workload = Path(args.workload).resolve()
    if json.loads(workload.read_text()).get("partition") != "heldout":
        raise RuntimeError("sealed release workload must declare heldout")
    shutil.copyfile(workload, output / "workload.json")
    for name in ("compiled-source-manifest.json", "compiled-source.tar.gz"):
        shutil.copyfile(development_path.parent / name, output / name)
    for field in ("gates", "rubric", "operating", "matrix", "index", "scorer", "auditor", "grader", "assessment", "procedure", "resources"):
        original = Path(frozen[field + "_path"])
        destination = output / (("audit_sources" if field == "auditor" else field) + original.suffix)
        shutil.copyfile(original, destination)
        frozen[field + "_path"] = str(destination)
    # Exact same executable, source snapshot, scorer, gates, ranking, budgets,
    # model identities and read procedure. Only separately authored source and
    # question data are prepared here; no reader is called during preparation.
    inputs = output / "inputs"
    environment = worker_environment(frozen)
    environment.update({"EVIE_MEMORY_INTEGRATED_INPUTS": str(inputs),
                        "EVIE_MEMORY_INTEGRATED_WORKLOAD": str(output / "workload.json"),
                        "EVIE_MEMORY_EMBEDDING_ENDPOINT": frozen["endpoint"]})
    write_json(output / "model-state-before-preparation.json", {
        "observed_at_utc": datetime.now(timezone.utc).isoformat(),
        "model_state": local_metadata(frozen["endpoint"], "/api/ps")})
    run_command([frozen["binary_path"], "-test.run=^TestMemoryStage5IntegratedPrepare$",
                 "-test.count=1", "-test.v", "-test.timeout=30m"],
                Path(frozen["source_root"]), output / "prepare-execution", environment)
    write_json(output / "model-state-after-preparation.json", {
        "observed_at_utc": datetime.now(timezone.utc).isoformat(),
        "model_state": local_metadata(frozen["endpoint"], "/api/ps")})
    input_hashes = tree_hashes(inputs)
    if not input_hashes or not any(name.endswith("/seed.db") for name in input_hashes):
        raise RuntimeError("release preparation did not retain canonical seed databases")
    write_json(output / "input-manifest.json", input_hashes)
    archive(inputs, input_hashes, output / "prepared-inputs.tar.gz")
    frozen.update({
        "version": args.version, "partition": "heldout",
        "frozen_at_utc": datetime.now(timezone.utc).isoformat(),
        "development_freeze_sha256": sha256(development_path),
        "development_report_sha256": sha256(report_path),
        "source_issue167_commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=repository, text=True).strip(),
        "workload_path": str(output / "workload.json"), "workload_sha256": sha256(output / "workload.json"),
        "inputs_path": str(inputs), "input_manifest_sha256": sha256(output / "input-manifest.json"),
        "prepared_inputs_archive_sha256": sha256(output / "prepared-inputs.tar.gz"),
        "same_development_executable": True,
    })
    write_json(output / "freeze.json", frozen)
    print(json.dumps({"freeze": str(output / "freeze.json"), "binary_sha256": frozen["binary_sha256"]}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="command", required=True)
    freezing = commands.add_parser("freeze")
    for name in ("repository", "output", "workload", "gates", "rubric", "operating", "matrix", "index", "scorer", "auditor", "grader", "assessment", "procedure", "resources", "models", "runtime", "version", "partition"):
        freezing.add_argument("--" + name, required=True)
    freezing.add_argument("--endpoint", default="http://127.0.0.1:11565")
    running = commands.add_parser("run")
    for name in ("freeze", "output"):
        running.add_argument("--" + name, required=True)
    running.add_argument("--mode", choices=("reader", "local", "index", "operating"), required=True)
    sealing = commands.add_parser("seal-release")
    for name in ("development-freeze", "development-report", "repository", "workload", "output", "version"):
        sealing.add_argument("--" + name, required=True)
    arguments = parser.parse_args()
    {"freeze": freeze, "run": run, "seal-release": seal_release}[arguments.command](arguments)
