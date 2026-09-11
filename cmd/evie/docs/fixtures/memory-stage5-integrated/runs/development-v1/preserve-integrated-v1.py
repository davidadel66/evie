#!/usr/bin/env python3
"""Plan, copy and verify immutable Stage 5 development-v1 evidence; stdlib only.

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


DIRECTORIES = {
    "raw-local.tar.gz": ["integrated-development-v1-local"],
    "raw-index.tar.gz": ["integrated-development-v1-index"],
    "raw-operating.tar.gz": ["integrated-development-v1-operating"],
    "raw-diagnostics.tar.gz": ["167-context-repro-v1", "166-dev01-cold-warm-diagnostic-v1", "162-corrected-context-replay-v1"],
    "raw-verification.tar.gz": ["integrated-preflight-metadata-v1", "integrated-runner-work"],
}
VERIFICATION_FILES = [
    "integrated-freeze-v1-command.json", "integrated-freeze-v1-console.log",
    "integrated-development-v1-local-console.log", "integrated-development-v1-index-console.log",
    "integrated-development-v1-operating-console.log", "integrated-development-v1-partial-resource-audit-console.log",
    "167-mapped-go-v1.command.json", "167-mapped-go-v1.started.json", "167-mapped-go-v1.results.json", "167-mapped-go-v1.log",
    "167-mapped-ui-command.json", "167-mapped-ui-results.json", "167-mapped-ui.log",
    "167-verify-pre-pilot-v2.command.json", "167-verify-pre-pilot-v2.log", "167-verify-pre-pilot.log",
    "167-ablation-red.log", "167-ablation-green.log", "167-ablation-regression.log",
    "167-index-compile.log", "167-index-historical-red.log", "167-index-unavailable-red.log",
    "167-index-model-free-tracer.log", "167-index-model-free-green.log", "167-index-model-free-final.log", "167-index-vet.log",
    "167-shared-probe-model-free.log", "167-resources-controls.py", "167-resources-controls.log",
    "audit-source-sanity.py", "analyze-format-sanity.py",
    "162-corrected-context-replay.py",
]


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
    return {"source": str(path), "bytes": path.stat().st_size, "sha256": digest(path)}


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
    # Deterministic wrapper metadata; exact member bytes are never rewritten.
    with gzip.GzipFile(filename="", mode="wb", fileobj=sink, mtime=0, compresslevel=9) as compressed:
        with tarfile.open(fileobj=compressed, mode="w|", format=tarfile.PAX_FORMAT) as archive:
            for name, item in sorted(members.items()):
                path = Path(item["source"])
                if path.stat().st_size != item["bytes"] or digest(path) != item["sha256"]:
                    raise ValueError("original changed after plan: " + str(path))
                info = tarfile.TarInfo(name)
                info.size, info.mode, info.mtime = item["bytes"], 0o600, 0
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


def make_plan(root):
    root = Path(root).resolve()
    frozen = root / "integrated-development-v1"
    freeze = read(frozen / "freeze.json")
    if freeze["version"] != "integrated-development-v1" or freeze["partition"] != "development":
        raise ValueError("this bounded preservation tool only selects development v1")
    if set(freeze.get("worker_environment") or {}) - {"GODEBUG", "GOGC", "GOMAXPROCS", "GOMEMLIMIT"}:
        raise ValueError("unexpected worker environment field; inspect without exporting environment values")
    for name, manifest, field in (("compiled-source.tar.gz", "compiled-source-manifest.json", "compiled_source_archive_sha256"),
                                  ("prepared-inputs.tar.gz", "input-manifest.json", "prepared_inputs_archive_sha256")):
        validate_original_archive(frozen / name, frozen / manifest, freeze[field])
    runner = root / "integrated-runner-v1.py"
    if digest(runner) != freeze["runner_sha256"]:
        raise ValueError("preserved runner does not match the original frozen runner")
    corrected = root / "162-corrected-context-replay-v1"
    corrected_freeze = read(corrected / "freeze.json")
    if corrected_freeze["original_development_freeze_sha256"] != digest(frozen / "freeze.json"):
        raise ValueError("corrected diagnostic does not identify this exact original freeze")
    validate_original_archive(corrected / "compiled-source.tar.gz", corrected / "compiled-source-manifest.json",
                              corrected_freeze["compiled_source_archive_sha256"])
    if digest(corrected / "input-manifest.json") != freeze["input_manifest_sha256"]:
        raise ValueError("corrected diagnostic inputs differ from original v1")
    direct, archives = {}, {}
    for path in sorted(frozen.rglob("*")):
        relative = path.relative_to(frozen)
        if not path.is_file() or relative.parts[0] in {"source", "inputs"} or relative == Path("integrated.test"):
            continue
        direct["frozen/" + str(relative)] = describe(path)
    direct["frozen/run.py"] = describe(runner)
    for name, relative in (("reports/deterministic-v1.json", "integrated-development-v1-deterministic.json"),
                           ("reports/partial-resource-audit-v1.json", "integrated-development-v1-partial-resource-audit/report.json")):
        direct[name] = describe(root / relative)
    direct["preserve-integrated-v1.py"] = describe(Path(__file__).resolve())
    for name, directories in DIRECTORIES.items():
        members = {}
        for directory in directories:
            original = root / directory
            if not original.is_dir():
                raise ValueError("missing completed evidence directory: " + str(original))
            for path in sorted(original.rglob("*")):
                relative = path.relative_to(original)
                if path.is_file() and relative.parts[0] not in {"source", "inputs"} and relative != Path("integrated.test"):
                    members[str(path.relative_to(root))] = describe(path)
        if name == "raw-verification.tar.gz":
            for relative in VERIFICATION_FILES:
                members[relative] = describe(root / relative)
        counter = CountHash()
        archive_to(counter, members)
        archives[name] = {"members": members, "bytes": counter.bytes, "sha256": counter.sha256.hexdigest(),
                          "original_bytes": sum(item["bytes"] for item in members.values())}
    binaries = [{"source": str(frozen / "integrated.test"), "bytes": (frozen / "integrated.test").stat().st_size,
                 "sha256": freeze["binary_sha256"], "reason": "compiled executable deliberately excluded; preserved source and build metadata remain"}]
    binaries.append({"source": str(corrected / "integrated.test"), "bytes": (corrected / "integrated.test").stat().st_size,
                     "sha256": corrected_freeze["binary_sha256"], "reason": "corrected diagnostic executable excluded; its exact source archive is retained in raw-diagnostics.tar.gz"})
    return {"schema_version": 1, "source_root": str(root), "freeze_sha256": digest(frozen / "freeze.json"),
            "tool_sha256": digest(__file__), "files": direct, "archives": archives,
            "planned_file_count": len(direct) + sum(len(item["members"]) for item in archives.values()),
            "original_selected_bytes": sum(item["bytes"] for item in direct.values()) + sum(item["original_bytes"] for item in archives.values()),
            "planned_payload_bytes": sum(item["bytes"] for item in direct.values()) + sum(item["bytes"] for item in archives.values()),
            "excluded": binaries + [{"source": str(frozen / name), "reason": "already preserved losslessly in the frozen " + archive}
                                    for name, archive in (("source", "compiled-source.tar.gz"), ("inputs", "prepared-inputs.tar.gz"))]
                        + [{"source": str(corrected / "source"), "reason": "corrected diagnostic source is already preserved in its compiled-source.tar.gz"}],
            "reproduction": {"original_paths_in_raw_metadata_remain_unchanged": True,
                "source_archive": "frozen/compiled-source.tar.gz", "source_manifest": "frozen/compiled-source-manifest.json",
                "input_archive": "frozen/prepared-inputs.tar.gz", "input_manifest": "frozen/input-manifest.json",
                "runner": "frozen/run.py", "original_build_command": read(frozen / "build.json")["command"],
                "go_version": freeze["go_version"], "original_source_root": freeze["source_root"],
                "original_inputs_path": freeze["inputs_path"], "original_binary_path": freeze["binary_path"],
                "original_binary_sha256": freeze["binary_sha256"], "source_head_before_issue_fix_folding": freeze["source_head_before_issue_fix_folding"],
                "model_and_context_metadata": "frozen/freeze.json", "binary_rebuild_attempted_by_preservation_tool": False,
                "corrected_diagnostic_source_archive": "raw-diagnostics.tar.gz:162-corrected-context-replay-v1/compiled-source.tar.gz",
                "corrected_diagnostic_freeze_sha256": digest(corrected / "freeze.json"),
                "limits": ["Compiled binary and model weights are not repository artifacts.",
                           "Exact original freezes retain their historical absolute paths; relocation requires a separate reproduction record, never editing the measured freeze.",
                           "The source archive covers the 391 declared Go compilation inputs; deterministic UI verification remains separately logged.",
                           "Fast reproduction contains one selected repetition, despite the original generic summary declaring twenty planned samples.",
                           "Failed-v1 runtime accounting fields are preserved exactly; missing or stale final receipts cannot establish final runtime totals.",
                           "No reader-quality output or held-out evaluation is represented by this failed local/index run."]}}


def copy_plan(plan_path, destination):
    plan = read(plan_path)
    if plan["tool_sha256"] != digest(__file__):
        raise ValueError("preservation tool changed after planning")
    destination = Path(destination).resolve()
    source_root = Path(plan["source_root"])
    if destination == source_root or destination.is_relative_to(source_root):
        raise ValueError("copy destination must be outside original evidence directory")
    destination.mkdir(parents=True, exist_ok=False)
    outputs = {}
    for name, item in plan["files"].items():
        path = Path(item["source"])
        if digest(path) != item["sha256"] or path.stat().st_size != item["bytes"]:
            raise ValueError("original changed after plan: " + str(path))
        target = destination / name
        target.parent.mkdir(parents=True, exist_ok=True)
        with path.open("rb") as source, target.open("xb") as output:
            shutil.copyfileobj(source, output)
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
                seen.add(member.name)
        if seen != set(archive_info["members"]):
            raise ValueError("preserved archive lacks original members")
        count += len(seen)
    for name, map_name, field in (("compiled-source.tar.gz", "compiled-source-manifest.json", "compiled_source_archive_sha256"),
                                  ("prepared-inputs.tar.gz", "input-manifest.json", "prepared_inputs_archive_sha256")):
        freeze = read(destination / "frozen/freeze.json")
        validate_original_archive(destination / "frozen" / name, destination / "frozen" / map_name, freeze[field])
    return {"verified_original_files": count, "output_files": len(actual), "payload_bytes": manifest["payload_bytes"],
            "freeze_sha256": manifest["freeze_sha256"], "model_calls": 0, "binary_retained": False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="mode", required=True)
    planning = commands.add_parser("plan")
    planning.add_argument("--source", default="/tmp/evie-memory-stage5")
    planning.add_argument("--output", required=True)
    copying = commands.add_parser("copy")
    copying.add_argument("--plan", required=True)
    copying.add_argument("--output", required=True)
    checking = commands.add_parser("verify")
    checking.add_argument("--directory", required=True)
    args = parser.parse_args()
    if args.mode == "plan":
        result = make_plan(args.source)
        write_new(Path(args.output).resolve(), result)
        print(json.dumps({key: result[key] for key in ("planned_file_count", "original_selected_bytes", "planned_payload_bytes", "freeze_sha256")}))
    elif args.mode == "copy":
        print(json.dumps(copy_plan(args.plan, args.output)))
    else:
        print(json.dumps(verify_copy(args.directory)))


if __name__ == "__main__":
    main()
