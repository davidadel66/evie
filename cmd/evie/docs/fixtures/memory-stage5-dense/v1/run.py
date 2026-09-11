#!/usr/bin/env python3
"""Freeze and run the actual #166 integration regression with stdlib only."""

import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
from datetime import datetime, timezone
import urllib.request
import urllib.parse


def digest(path):
    value = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            value.update(block)
    return value.hexdigest()


def write(path, value):
    with Path(path).open("x") as stream:
        json.dump(value, stream, indent=2, sort_keys=True)
        stream.write("\n")


def source_hashes(root):
    paths = list(root.rglob("*.go")) + [root / "go.mod", root / "go.sum"]
    return {str(path.relative_to(root)): digest(path) for path in sorted(paths)
            if ".git" not in path.parts and "node_modules" not in path.parts}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise RuntimeError("local metadata redirects are forbidden")


def get_metadata(endpoint, path):
    parsed = urllib.parse.urlsplit(endpoint)
    if (parsed.scheme != "http" or parsed.username or parsed.password
            or parsed.path or parsed.query or parsed.fragment
            or not ipaddress.ip_address(parsed.hostname).is_loopback):
        raise RuntimeError("metadata requires literal loopback HTTP")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    with opener.open(endpoint + path, timeout=10) as response:
        if response.status != 200:
            raise RuntimeError("local metadata unavailable")
        data = response.read((1 << 20) + 1)
        if len(data) > 1 << 20:
            raise RuntimeError("local metadata oversized")
        return json.loads(data)


def freeze(args):
    root = Path(args.source).resolve()
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=False)
    binary = Path(args.binary).resolve()
    if binary.exists():
        raise RuntimeError("refuse to overwrite an existing frozen executable")
    fixture_root = root / "docs/experiments/memory-retrieval-spike/v1"
    fixtures = {name: digest(fixture_root / name)
                for name in ["corpus.json", "development.json", "heldout.json"]}
    before = source_hashes(root)
    build = subprocess.run(["go", "test", "-c", "./internal/agent", "-o", str(binary)],
                           cwd=root, text=True, capture_output=True)
    write(output / "build.json", {"command": ["go", "test", "-c", "./internal/agent", "-o", str(binary)],
                                  "exit_code": build.returncode,
                                  "stdout": build.stdout, "stderr": build.stderr})
    if build.returncode:
        raise RuntimeError("experiment compilation failed; build artifact retained")
    after = source_hashes(root)
    write(output / "compiled-sources-before.json", before)
    write(output / "compiled-sources-after.json", after)
    if before != after:
        raise RuntimeError("source inputs changed while compiling; freeze aborted")
    models = Path(args.models).resolve()
    manifest_path = models / "manifests/registry.ollama.ai/library/all-minilm/22m"
    manifest = json.loads(manifest_path.read_text())
    manifest_digest = digest(manifest_path)
    if manifest_digest != "1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef":
        raise RuntimeError("local model manifest differs from selected #165 digest")
    layers = []
    for layer in manifest["layers"]:
        path = models / "blobs" / layer["digest"].replace(":", "-")
        measured = digest(path)
        if measured != layer["digest"].split(":", 1)[1]:
            raise RuntimeError("local model blob differs from manifest")
        layers.append({"media_type": layer["mediaType"], "sha256": measured,
                       "bytes": path.stat().st_size})
    if not any(layer["sha256"] == "797b70c4edf85907fe0a49eb85811256f65fa0f7bf52166b147fd16be2be4662"
               for layer in layers):
        raise RuntimeError("selected actual model weights are absent")
    tags = get_metadata(args.endpoint, "/api/tags")
    if not any(model["name"] == "all-minilm:22m" and
               model["digest"].removeprefix("sha256:") == manifest_digest
               for model in tags.get("models", [])):
        raise RuntimeError("served model tag does not match selected manifest")
    version = get_metadata(args.endpoint, "/api/version")
    if version.get("version") != "0.6.3":
        raise RuntimeError("runtime version differs from selected #165 runtime")
    runtime_binary = Path(args.runtime).resolve()
    runtime_digest = digest(runtime_binary)
    if runtime_digest != "2794d30cf43308d6ce368d2827f09b2b6b2e804960e9f5768b3b603f1ad93a8f":
        raise RuntimeError("runtime executable differs from selected #165 digest")
    hardware = {"platform": platform.platform(), "machine": platform.machine(),
                "logical_cpu_count": os.cpu_count()}
    for name in ["hw.model", "hw.memsize", "hw.physicalcpu"]:
        hardware[name] = subprocess.check_output(["sysctl", "-n", name], text=True).strip()
    write(output / "freeze.json", {
        "version": "memory-stage5-dense-integration-v1",
        "frozen_at_utc": datetime.now(timezone.utc).isoformat(),
        "source_root": str(root), "binary_path": str(binary), "binary_sha256": digest(binary),
        "fixture_root": str(fixture_root), "fixture_sha256": fixtures,
        "compiled_sources_before_sha256": digest(output / "compiled-sources-before.json"),
        "compiled_sources_after_sha256": digest(output / "compiled-sources-after.json"),
        "runner_sha256": digest(__file__), "endpoint": args.endpoint,
        "runtime_version": version, "runtime_binary_sha256": runtime_digest,
        "model_manifest_sha256": manifest_digest, "model_manifest": manifest,
        "model_layers": layers, "hardware": hardware,
        "go_version": subprocess.check_output(["go", "version"], text=True).strip(),
        "configuration": {"model": "all-minilm:22m", "dimensions": 384,
            "normalization": "L2 float32", "minimum_cosine": 0.25, "rrf_k": 60,
            "conditions": ["lexical endpoint unset", "hybrid actual local endpoint"],
            "condition_order": "rotate by (case_index + repetition) modulo two",
            "repetitions": 3, "result_limit": 8, "result_bytes": 12288, "turn_bytes": 36864,
            "whole_turn_p95_ns_max": 250000000, "paraphrase_recall_min": 0.85,
            "paraphrase_improvement_min": 0.10, "zero_forbidden_or_secret": True,
            "automatic_recall": False, "provider": "scripted complete Session.Send",
            "context_profile": {"hard": 300000, "working": 262144, "output": 16384},
            "maintenance_batch_rows": 256, "maintenance_max_batches": 10000,
            "maintenance_embedding_input_budget": 16, "dense_chunk_bytes": 240,
            "dense_chunk_overlap_bytes": 48, "dense_vector_scan_limit": 4096,
            "query_dense_subdeadline_ms_max": 250,
            "embedding_num_thread": 4, "embedding_truncate": False,
            "embedding_keep_alive": "30s", "embedding_operation_deadline_seconds": 10,
            "HTTP_observer": "unchanged body forwarding; direct loopback, no proxy or redirects; overhead included",
            "corpus_adapter": "f.remember prefixes accepted source with 'Remember my retrieval marker: '; literal unchanged; predicate retrieval_marker; f.converse uses unchanged original plus scripted Noted acknowledgement",
            "source_policy": "all 389 corpus records authored and fully indexed before selected questions decoded",
            "timing_scope": "Session.Send including durable history, tool dispatch, actual query embedding, retrieval/fusion, revalidation and full provider request composition; excludes fixture setup, maintenance, session creation and artifact serialization",
            "comparison_limit": "#165 used four results/8192-byte evidence-array budget and Store broker boundary; this run measures the full production turn with eight results/12288-byte result and 36864-byte cumulative memory budget",
            "partition_policy": "Reuse previously evaluated #165 development and heldout partitions solely as integration regression; never #167/#168 untouched heldout",
        },
    })
    print(json.dumps({"freeze": str(output / "freeze.json"), "binary_sha256": digest(binary)}))


def run(args):
    path = Path(args.freeze).resolve()
    freeze = json.loads(path.read_text())
    binary = Path(freeze["binary_path"])
    if digest(binary) != freeze["binary_sha256"]:
        raise RuntimeError("frozen executable changed")
    if digest(__file__) != freeze["runner_sha256"]:
        raise RuntimeError("runner changed after freeze")
    root = Path(freeze["source_root"])
    before_path = path.parent / "compiled-sources-before.json"
    if digest(before_path) != freeze["compiled_sources_before_sha256"]:
        raise RuntimeError("compiled source manifest changed")
    if source_hashes(root) != json.loads(before_path.read_text()):
        raise RuntimeError("isolated source snapshot changed")
    output = Path(args.output).resolve()
    if output.exists():
        raise RuntimeError("refuse to overwrite an existing partition attempt")
    environment = os.environ.copy()
    environment.update({"EVIE_DENSE_EXPERIMENT_FREEZE": str(path),
                        "EVIE_DENSE_EXPERIMENT_PARTITION": args.partition,
                        "EVIE_DENSE_EXPERIMENT_OUTPUT": str(output)})
    command = [str(binary), "-test.run=^TestMemoryStage5DenseExperiment$", "-test.v", "-test.count=1", "-test.timeout=20m"]
    started = datetime.now(timezone.utc).isoformat()
    result = subprocess.run(command, cwd=root, env=environment, text=True, capture_output=True)
    if not output.exists():
        output.mkdir()
    with (output / "execution.log").open("x") as stream:
        stream.write(result.stdout)
        stream.write(result.stderr)
    write(output / "execution.json", {"command": command, "started_at_utc": started,
                                     "finished_at_utc": datetime.now(timezone.utc).isoformat(),
                                     "exit_code": result.returncode,
                                     "freeze_sha256": digest(path)})
    print(json.dumps({"partition": args.partition, "exit_code": result.returncode, "output": str(output)}))
    return result.returncode


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="command", required=True)
    freezing = sub.add_parser("freeze")
    for option in ["source", "output", "binary", "models", "runtime"]:
        freezing.add_argument("--" + option, required=True)
    freezing.add_argument("--endpoint", default="http://127.0.0.1:11565")
    running = sub.add_parser("run")
    running.add_argument("--freeze", required=True)
    running.add_argument("--output", required=True)
    running.add_argument("--partition", required=True, choices=["development", "heldout"])
    args = parser.parse_args()
    sys.exit(freeze(args) if args.command == "freeze" else run(args))
