#!/usr/bin/env python3
"""Run the scripted Stage 4 contract and retain fail-closed, source-bound evidence."""

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parent.parent
CONTRACT_PATH = ROOT / "cmd/evie/docs/fixtures/memory-stage-4-conformance/v1/contract.json"
MARKER = "STAGE4_EVIDENCE "


def sha256(raw):
    return hashlib.sha256(raw).hexdigest()


def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def source_fingerprint(root=ROOT):
    """Hash tracked and untracked source, including deletions, but no run artifacts.

    The same routine runs before browser setup and before/after verification.
    Results belong outside source directories (normally .scratch/ or /tmp/).
    """
    paths = subprocess.check_output(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"], cwd=root
    ).decode().split("\0")
    records = []
    for name in sorted(set(paths)):
        parts = Path(name).parts
        if not parts or (parts[0] not in {"cmd", "internal", "scripts", "docs", ".github"}
                         and name not in {"go.mod", "go.sum", "AGENTS.md", "package.json", "package-lock.json"}):
            continue
        if any(part in {"node_modules", "dist", "__pycache__"} for part in parts):
            continue
        path = root / name
        if path.is_symlink():
            records.append({"path": name, "symlink": os.readlink(path)})
        elif path.is_file():
            records.append({"path": name, "sha256": sha256(path.read_bytes()), "executable": bool(path.stat().st_mode & 0o111)})
        elif not path.exists():
            records.append({"path": name, "deleted": True})
    if not records:
        raise ValueError("no source files in fingerprint")
    return {"sha256": sha256(encoded(records)), "files": records}


def inspect_go_events(raw, contract):
    """A receipt is evidence only when its emitting test and package passed."""
    outcomes, package_outcomes, receipts, fragments, failures, skips = {}, {}, [], {}, [], []
    for line in raw.splitlines():
        try:
            event = json.loads(line)
        except (ValueError, TypeError):
            failures.append("invalid go test JSON output")
            continue
        package, test, action = event.get("Package", ""), event.get("Test", ""), event.get("Action")
        key = (package, test)
        if action in {"pass", "fail", "skip"}:
            if test:
                outcomes[key] = action
            else:
                package_outcomes[package] = action
            if action == "fail":
                failures.append(f"{package}:{test or '(package)'} failed")
            if action == "skip":
                skips.append(f"{package}:{test or '(package)'}")
        if action == "output":
            fragments[key] = fragments.get(key, "") + event.get("Output", "")
            while "\n" in fragments[key]:
                output, fragments[key] = fragments[key].split("\n", 1)
                if MARKER in output:
                    try:
                        receipt = json.loads(output.split(MARKER, 1)[1])
                        if not isinstance(receipt, dict):
                            raise ValueError("not an object")
                        receipts.append({"package": package, "test": test, "receipt": receipt})
                    except (ValueError, TypeError):
                        failures.append(f"{package}:{test} emitted invalid evidence")
    for package, tests in contract["packages"].items():
        if package_outcomes.get(package) != "pass":
            failures.append(f"required package did not pass: {package}")
        for test in tests:
            if outcomes.get((package, test)) != "pass":
                failures.append(f"required test did not pass: {package}:{test}")
    found = {}
    for item in receipts:
        receipt, test, package = item["receipt"], item["test"], item["package"]
        scenario = receipt.get("scenario")
        parent = test.split("/", 1)[0]
        expected = contract["scenarios"].get(scenario) if isinstance(scenario, str) else None
        if receipt.get("version") != contract["version"] or expected != parent:
            failures.append(f"unbound evidence: {scenario}")
        elif parent not in contract["packages"].get(package, []):
            failures.append(f"wrong evidence package: {scenario}")
        elif outcomes.get((package, test)) != "pass" or outcomes.get((package, parent)) != "pass":
            failures.append(f"evidence test did not pass: {scenario}")
        elif scenario in found:
            failures.append(f"duplicate evidence: {scenario}")
        else:
            found[scenario] = item
    for scenario in contract["scenarios"]:
        if scenario not in found:
            failures.append(f"missing scenario evidence: {scenario}")
    if skips:
        failures.append("required focused run skipped tests")
    return {"failures": failures, "skips": skips, "scenarios": found,
            "tests": [{"package": p, "test": t, "outcome": a} for (p, t), a in sorted(outcomes.items())]}


def validate_browser(receipt, contract, source_sha256):
    failures = []
    if receipt.get("version") != contract["browser"]["version"]:
        failures.append("browser receipt version mismatch")
    if receipt.get("status") != "passed":
        failures.append("browser interaction did not pass")
    if receipt.get("source_sha256") != source_sha256:
        failures.append("browser receipt source fingerprint mismatch")
    cases = receipt.get("cases", [])
    if not isinstance(cases, list):
        return failures + ["browser cases must be a list"]
    names = [case.get("name") for case in cases if isinstance(case, dict)]
    if len(names) != len(cases) or any(not isinstance(name, str) for name in names) or sorted(names) != sorted(contract["browser"]["cases"]):
        failures.append("browser case set is incomplete or duplicated")
    for case in cases:
        if not isinstance(case, dict):
            failures.append("invalid browser case")
            continue
        checks = case.get("checks", {})
        for check in contract["browser"]["checks"]:
            if not isinstance(checks, dict) or checks.get(check) is not True:
                failures.append(f"browser {case.get('name')} did not prove {check}")
    # This explicitly distinguishes real UI interaction from public Store checks.
    if not isinstance(receipt.get("provenance"), dict) or not receipt["provenance"].get("web_interactions") or not receipt["provenance"].get("store_checks"):
        failures.append("browser receipt lacks UI/Store provenance boundaries")
    return failures


def run_command(name, argv, cwd, output, timeout=1200):
    path = output / (name + ".log")
    started = dt.datetime.now(dt.timezone.utc).isoformat()
    begin = time.monotonic()
    print(f"Running {name}: {' '.join(argv)}", flush=True)
    with path.open("wb") as log:
        try:
            # Process groups allow a timed-out Go command to stop its child tests.
            process = subprocess.Popen(argv, cwd=cwd, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            try:
                code = process.wait(timeout=timeout)
            except subprocess.TimeoutExpired:
                import signal
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
                log.write(b"\nCONFORMANCE COMMAND TIMED OUT\n")
                code = 124
        except OSError as error:
            log.write(str(error).encode())
            code = 127
    raw = path.read_bytes()
    warnings = [line for line in raw.decode(errors="replace").splitlines() if re.search(r"\bwarning\b", line, re.I)]
    print(f"{name}: exit {code} ({time.monotonic() - begin:.1f}s)", flush=True)
    return {"name": name, "argv": argv, "cwd": str(cwd.relative_to(ROOT)) or ".", "exit_code": code,
            "started_utc": started, "duration_seconds": round(time.monotonic() - begin, 3),
            "log": path.name, "log_sha256": sha256(raw), "warnings": warnings}, raw


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fingerprint", action="store_true", help="print only the current source SHA256 for browser evidence")
    parser.add_argument("--output-dir", type=Path, help="new directory outside source roots for immutable logs/report")
    parser.add_argument("--browser-receipt", type=Path, help="completed browser receipt from this exact source")
    parser.add_argument("--focused-only", action="store_true", help="development run; explicitly incomplete, never conformance")
    args = parser.parse_args(argv)
    source = source_fingerprint()
    if args.fingerprint:
        print(source["sha256"])
        return 0
    output = (args.output_dir or ROOT / ".scratch" / "memory-stage-4" / ("conformance-" + dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ"))).resolve()
    try:
        relative = output.relative_to(ROOT)
    except ValueError:
        relative = None
    if relative and relative.parts and relative.parts[0] in {"cmd", "internal", "scripts", "docs", ".github"}:
        parser.error("results must be outside source directories")
    output.mkdir(parents=True, exist_ok=False)
    contract = json.loads(CONTRACT_PATH.read_text())
    report = {"version": contract["version"], "kind": contract["kind"], "status": "incomplete",
              "source": source, "git_head": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT).decode().strip(),
              "environment": {"os": platform.platform(), "python": platform.python_version()},
              "commands": [], "failures": [], "warnings": [], "skipped_checks": [], "excluded_claims": contract["excluded_claims"]}
    for name, command in [("go", ["go", "version"]), ("node", ["node", "--version"]), ("npm", ["npm", "--version"]), ("sqlite_driver", ["go", "list", "-m", "-json", "modernc.org/sqlite"])]:
        record, raw = run_command("environment-" + name, command, ROOT, output, 60)
        report["commands"].append(record)
        report["environment"][name] = raw.decode(errors="replace").strip()
    record, _ = run_command("report-boundaries", [sys.executable, "scripts/memory_stage4_conformance_test.py"], ROOT, output, 60)
    report["commands"].append(record)
    tests = [test for package in contract["packages"].values() for test in package]
    pattern = "^(" + "|".join(re.escape(test) for test in tests) + ")$"
    for name, race in [("integrated", []), ("integrated-race", ["-race"])]:
        command = ["go", "test", *race, "-json", "./cmd/evie", "./internal/eviedb", "-run", pattern, "-count=1"]
        record, raw = run_command(name, command, ROOT, output)
        report["commands"].append(record)
        analysis = inspect_go_events(raw.decode(errors="replace"), contract)
        report[name] = analysis
        report["failures"].extend(name + ": " + error for error in analysis["failures"])
    if args.focused_only:
        report["skipped_checks"].extend([{"check": "repository verification", "reason": "explicit --focused-only development run"}, {"check": "UI tests", "reason": "explicit --focused-only development run"}])
    else:
        for name, command, cwd in [("repository-verification", ["./scripts/verify-change.sh"], ROOT), ("ui-tests", ["npx", "--no-install", "vitest", "run"], ROOT / "internal/web/ui")]:
            record, _ = run_command(name, command, cwd, output)
            report["commands"].append(record)
    if args.browser_receipt:
        try:
            raw = args.browser_receipt.read_bytes()
            browser = json.loads(raw)
            if not isinstance(browser, dict):
                raise ValueError("browser receipt must be an object")
            report["failures"].extend(validate_browser(browser, contract, source["sha256"]))
            (output / "browser-receipt.json").write_bytes(raw)
            report["browser"] = {"receipt": "browser-receipt.json", "sha256": sha256(raw), "result": browser}
        except (OSError, ValueError) as error:
            report["failures"].append("unreadable browser receipt: " + str(error))
    else:
        report["skipped_checks"].append({"check": "actual browser interactions", "reason": "no matching --browser-receipt supplied; conformance remains incomplete"})
    report["source_after_sha256"] = source_fingerprint()["sha256"]
    if report["source_after_sha256"] != source["sha256"]:
        report["failures"].append("source changed during verification")
    for command in report["commands"]:
        if command["exit_code"] != 0:
            report["failures"].append(command["name"] + " exited " + str(command["exit_code"]))
        report["warnings"].extend({"command": command["name"], "message": warning} for warning in command["warnings"])
    report["status"] = "failed" if report["failures"] else "incomplete" if report["skipped_checks"] else "passed"
    report["completed_utc"] = dt.datetime.now(dt.timezone.utc).isoformat()
    (output / "report.json").write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print(f"{report['status']}: {output / 'report.json'}", flush=True)
    return 1 if report["failures"] else 2 if report["skipped_checks"] else 0


if __name__ == "__main__":
    sys.exit(main())
