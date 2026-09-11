#!/usr/bin/env python3
"""Plan, copy and verify immutable Stage 5 development-v2 evidence; stdlib only.

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
    "raw-reader.tar.gz": ["integrated-development-v2-reader"],
    "raw-source-audit.tar.gz": ["integrated-development-v2-evidence"],
    "raw-assessments.tar.gz": ["integrated-development-v2-assessments"],
    "raw-local.tar.gz": ["integrated-development-v2-local"],
    "raw-index.tar.gz": ["integrated-development-v2-index"],
    "raw-operating.tar.gz": ["integrated-development-v2-operating"],
    "raw-verification.tar.gz": ["integrated-development-v2-checks"],
}
VERIFICATION_FILES = [
    "run-integrated-development-v2.py", "grade-and-audit-development-v2.py",
    "167-verify-development-v2.py", "167-resources-v2-controls.py", "167-resources-v2-controls.log",
    "167-citation-grade-controls.py", "167-citation-grade-controls.log",
    "167-resources-citation-api-controls.py", "167-resources-citation-api-red.log",
    "167-resources-citation-api-green.log", "167-resources-citation-api-regression.log",
    "167-seal-protocol-controls.py", "167-seal-protocol-controls.log",
    "architecture-assessment-check-v2.json", "experiment-assessment-check-v2.json",
    "integrated-assessment-helper.py", "integrated-assessment-helper-notes.md",
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
                if path.stat().st_size != item["bytes"] or digest(path) != item["sha256"]:
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


def make_plan(root):
    root = Path(root).resolve()
    frozen = root / "integrated-development-v2"
    freeze = read(frozen / "freeze.json")
    freeze_sha = digest(frozen / "freeze.json")
    if freeze["version"] != "integrated-development-v2" or freeze["partition"] != "development":
        raise ValueError("this tool preserves only the completed development-v2 run")
    if set(freeze.get("worker_environment") or {}) - {"GODEBUG", "GOGC", "GOMAXPROCS", "GOMEMLIMIT"}:
        raise ValueError("unexpected worker environment field; no environment will be exported")
    for name, manifest, field, hash_field in (
        ("compiled-source.tar.gz", "compiled-source-manifest.json", "compiled_source_archive_sha256", "compiled_source_manifest_sha256"),
        ("prepared-inputs.tar.gz", "input-manifest.json", "prepared_inputs_archive_sha256", "input_manifest_sha256")):
        if digest(frozen / manifest) != freeze[hash_field]:
            raise ValueError("original manifest differs from freeze")
        validate_original_archive(frozen / name, frozen / manifest, freeze[field])
    verify_map(frozen / "source", read(frozen / "compiled-source-manifest.json"))
    verify_map(frozen / "inputs", read(frozen / "input-manifest.json"))
    for key, value in freeze.items():
        if key.endswith("_path") and key[:-5] + "_sha256" in freeze:
            if digest(Path(value)) != freeze[key[:-5] + "_sha256"]:
                raise ValueError("frozen file hash mismatch: " + key)
    if digest(frozen / "run.py") != freeze["runner_sha256"]:
        raise ValueError("frozen runner hash mismatch")
    for mode in ["reader", "local", "index", "operating"]:
        execution = read(root / ("integrated-development-v2-" + mode) / "execution.json")
        if not execution.get("finished_at_utc") or execution.get("execution_exception"):
            raise ValueError("run has not closed: " + mode)
    for mode in ["grade", "resource"]:
        execution = read(root / ("integrated-development-v2-" + mode + "-execution.json"))
        if not execution.get("finished_at_utc") or execution.get("execution_exception"):
            raise ValueError("post-run audit has not closed: " + mode)
    if not (root / "integrated-development-v2-resource/report.json").is_file():
        raise ValueError("resource audit report missing")
    evidence = read(root / "integrated-development-v2-evidence/evidence-report.json")
    if evidence["freeze_sha256"] != freeze_sha:
        raise ValueError("evidence report freeze identity mismatch")
    verify_map(root / "integrated-development-v2-reader", read(root / "integrated-development-v2-evidence/results-manifest.json"))
    assessments = read(root / "integrated-development-v2-assessment-manifest.json")
    if len(assessments) != 144:
        raise ValueError("144 assessment files required")
    verify_map(root / "integrated-development-v2-assessments", assessments)
    for path in (root / "integrated-development-v2-assessments").glob("*.json"):
        item = read(path)
        if not item.get("review_complete") or item["freeze_sha256"] != freeze_sha:
            raise ValueError("assessment is not complete for this freeze")
    direct, archives = {}, {}
    for path in sorted(frozen.rglob("*")):
        relative = path.relative_to(frozen)
        if not path.is_file() or relative.parts[0] in {"source", "inputs"} or relative == Path("integrated.test"):
            continue
        direct["frozen/" + str(relative)] = describe(path)
    reports = {
        "reports/deterministic.json": "integrated-development-v2-deterministic.json",
        "reports/evidence-report.json": "integrated-development-v2-evidence/evidence-report.json",
        "reports/assessment-manifest.json": "integrated-development-v2-assessment-manifest.json",
        "reports/quality-report.json": "integrated-development-v2-quality/quality-report.json",
        "reports/resource-report.json": "integrated-development-v2-resource/report.json",
    }
    for name, original in reports.items():
        direct[name] = describe(root / original)
    for path in sorted(root.glob("integrated-development-v2*")):
        if path.is_file() and path.name not in set(reports.values()):
            direct["commands/" + path.name] = describe(path)
    for name in ["integrated-freeze-v2-command.json", "integrated-freeze-v2-console.log"]:
        direct["commands/" + name] = describe(root / name)
    direct["preserve-integrated-v2.py"] = describe(Path(__file__).resolve())
    for name, directories in DIRECTORIES.items():
        members = {}
        for directory in directories:
            original = root / directory
            for path in sorted(original.rglob("*")):
                relative = path.relative_to(original)
                if path.is_file() and relative.parts[0] not in {"source", "inputs"} and relative != Path("integrated.test"):
                    members[str(path.relative_to(root))] = describe(path)
        if name == "raw-verification.tar.gz":
            for relative in VERIFICATION_FILES:
                members[relative] = describe(root / relative)
        if name == "raw-assessments.tar.gz":
            members["integrated-development-v2-assessment-manifest.json"] = describe(root / "integrated-development-v2-assessment-manifest.json")
        counter = CountHash()
        archive_to(counter, members)
        archives[name] = {"members": members, "bytes": counter.bytes, "sha256": counter.sha256.hexdigest(),
                          "original_bytes": sum(item["bytes"] for item in members.values())}
    return {"schema_version": 2, "source_root": str(root), "freeze_sha256": freeze_sha,
            "tool_sha256": digest(__file__), "files": direct, "archives": archives,
            "planned_file_count": len(direct) + sum(len(item["members"]) for item in archives.values()),
            "original_selected_bytes": sum(item["bytes"] for item in direct.values()) + sum(item["original_bytes"] for item in archives.values()),
            "planned_payload_bytes": sum(item["bytes"] for item in direct.values()) + sum(item["bytes"] for item in archives.values()),
            "excluded": [{"source": str(frozen / "integrated.test"), "bytes": (frozen / "integrated.test").stat().st_size,
                          "sha256": freeze["binary_sha256"], "reason": "compiled executable excluded; exact source archive/build command retained"}]
                        + [{"source": str(frozen / name), "reason": "already preserved losslessly in frozen/" + archive}
                           for name, archive in [("source", "compiled-source.tar.gz"), ("inputs", "prepared-inputs.tar.gz")]]
                        + [{"source": str(root / ("integrated-development-v2-assessments-" + owner)),
                            "reason": "identical final assessment bytes are in the combined 144-file assessment set"}
                           for owner in ["architecture", "experiment", "requirements"]],
            "completed_run": {"reader_turns": 144, "reader_http_calls": 181, "local_turns": 2880,
                "index_cases": 24, "operating_turns": 60, "semantic_assessments": 144,
                "quality_grader_exit_code": read(root / "integrated-development-v2-grade-execution.json")["exit_code"],
                "resource_grader_exit_code": read(root / "integrated-development-v2-resource-execution.json")["exit_code"],
                "release_ready": read(root / "integrated-development-v2-resource/report.json")["release_ready"]},
            "reproduction": {"original_paths_timestamps_and_contents_unchanged": True,
                "source_archive": "frozen/compiled-source.tar.gz", "source_manifest": "frozen/compiled-source-manifest.json",
                "input_archive": "frozen/prepared-inputs.tar.gz", "input_manifest": "frozen/input-manifest.json",
                "runner": "frozen/run.py", "original_build_command": read(frozen / "build.json")["command"],
                "go_version": freeze["go_version"], "original_source_root": freeze["source_root"],
                "original_inputs_path": freeze["inputs_path"], "original_binary_path": freeze["binary_path"],
                "original_binary_sha256": freeze["binary_sha256"],
                "source_head_before_issue_fix_folding": freeze["source_head_before_issue_fix_folding"],
                "model_and_context_metadata": "frozen/freeze.json", "binary_rebuild_attempted_by_preservation_tool": False,
                "limits": ["This is development-v2 evidence, never held-out or release evidence.",
                           "Frozen quality/resource failures and assessor/auditor disagreements remain unchanged.",
                           "Compiled binary and model weights are excluded; acquire the pinned model using the existing component instructions.",
                           "Historical absolute paths are retained exactly. Relocation requires a separate reproduction record, not edits to the measured freeze.",
                           "Archive member bytes and original file modification times are preserved; new gzip wrapper time is zero."]}}


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
