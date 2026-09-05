#!/usr/bin/env python3
"""Regression tests for accidentally approving incomplete or stale conformance."""

import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("conformance", Path(__file__).with_name("memory-stage-4-conformance.py"))
RUNNER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RUNNER)


class EvidenceBoundaries(unittest.TestCase):
    def setUp(self):
        self.contract = {"version": "v1", "packages": {"real/package": ["TestReal"]}, "scenarios": {"real": "TestReal"}}
        self.receipt = {"version": "v1", "scenario": "real", "details": {"boundary": True}}

    def events(self, receipt=None, child_outcome="pass", package="real/package"):
        body = "prefix STAGE4_EVIDENCE " + json.dumps(receipt or self.receipt) + "\n"
        # test2json may split an evidence line across multiple Output events.
        return [{"Action": "output", "Package": package, "Test": "TestReal/child", "Output": body[:25]},
                {"Action": "output", "Package": package, "Test": "TestReal/child", "Output": body[25:]},
                {"Action": child_outcome, "Package": package, "Test": "TestReal/child"},
                {"Action": "pass", "Package": package, "Test": "TestReal"},
                {"Action": "pass", "Package": package}]

    def inspect(self, events):
        return RUNNER.inspect_go_events("\n".join(json.dumps(event) for event in events), self.contract)

    def test_split_receipt_requires_passing_test_and_package(self):
        result = self.inspect(self.events())
        self.assertEqual(result["failures"], [])
        self.assertEqual(result["scenarios"]["real"]["receipt"], self.receipt)
        for outcome in ("fail", "skip"):
            with self.subTest(outcome=outcome):
                self.assertTrue(self.inspect(self.events(child_outcome=outcome))["failures"])
        self.assertTrue(self.inspect(self.events()[:-1])["failures"])

    def test_receipt_cannot_replace_tests_or_cross_packages(self):
        self.assertTrue(self.inspect(self.events()[:2])["failures"])
        self.assertTrue(self.inspect(self.events(package="other/package"))["failures"])
        self.assertTrue(self.inspect(self.events()[2:])["failures"])
        self.assertTrue(self.inspect(self.events() + self.events()[:2])["failures"])
        for receipt in ({"version": "other", "scenario": "real"}, {"version": "v1", "scenario": []}):
            with self.subTest(receipt=receipt):
                self.assertTrue(self.inspect(self.events(receipt=receipt))["failures"])

    def test_failed_test_cannot_be_erased_by_later_pass(self):
        events = [{"Action": "fail", "Package": "real/package", "Test": "TestReal"}] + self.events()
        self.assertTrue(self.inspect(events)["failures"])

    def test_invalid_go_output_fails_closed(self):
        self.assertTrue(RUNNER.inspect_go_events("not test JSON", self.contract)["failures"])


class BrowserBoundaries(unittest.TestCase):
    def setUp(self):
        self.contract = {"browser": {"version": "browser-v1", "cases": ["global", "private"], "checks": ["scope", "resolution"]}}
        self.receipt = {"version": "browser-v1", "status": "passed", "source_sha256": "source-a",
                        "cases": [{"name": name, "checks": {"scope": True, "resolution": True}} for name in ("global", "private")],
                        "provenance": {"web_interactions": "actual preview and approval", "store_checks": "exact replay"}}

    def test_rejects_stale_incomplete_and_false_evidence(self):
        self.assertEqual(RUNNER.validate_browser(self.receipt, self.contract, "source-a"), [])
        self.assertTrue(RUNNER.validate_browser(self.receipt, self.contract, "source-b"))
        for field, value in (("cases", self.receipt["cases"][:1]), ("cases", [None]), ("cases", [{"name": None}]), ("cases", [self.receipt["cases"][0]] * 2), ("status", "incomplete"), ("provenance", {})):
            receipt = copy.deepcopy(self.receipt)
            receipt[field] = value
            with self.subTest(field=field, value=value):
                self.assertTrue(RUNNER.validate_browser(receipt, self.contract, "source-a"))
        receipt = copy.deepcopy(self.receipt)
        receipt["cases"][1]["checks"]["scope"] = False
        self.assertTrue(RUNNER.validate_browser(receipt, self.contract, "source-a"))


class SourceBinding(unittest.TestCase):
    def test_untracked_changes_deletions_and_generated_results(self):
        with tempfile.TemporaryDirectory() as directory:
            # An alternate index/worktree may be verifying the caller. Bind all
            # repository paths here so this fixture cannot operate on that repo.
            with patch.dict(os.environ, {"GIT_DIR": str(Path(directory) / ".git"),
                                         "GIT_COMMON_DIR": str(Path(directory) / ".git"),
                                         "GIT_WORK_TREE": directory,
                                         "GIT_INDEX_FILE": str(Path(directory) / ".git" / "index")}):
                self.check_source_binding(Path(directory))

    def check_source_binding(self, root):
        subprocess.run(["git", "init", "-q", str(root)], check=True)
        (root / "internal").mkdir()
        source = root / "internal" / "behavior.go"
        source.write_text("original")
        subprocess.run(["git", "add", "internal/behavior.go"], cwd=root, check=True)
        original = RUNNER.source_fingerprint(root)["sha256"]
        (root / ".scratch").mkdir()
        (root / ".scratch" / "result.json").write_text("generated")
        self.assertEqual(RUNNER.source_fingerprint(root)["sha256"], original)
        untracked = root / "internal" / "new_test.go"
        untracked.write_text("new coverage")
        self.assertNotEqual(RUNNER.source_fingerprint(root)["sha256"], original)
        untracked.unlink()
        source.write_text("edited")
        self.assertNotEqual(RUNNER.source_fingerprint(root)["sha256"], original)
        source.unlink()
        deleted = RUNNER.source_fingerprint(root)
        self.assertNotEqual(deleted["sha256"], original)
        self.assertEqual(deleted["files"], [{"path": "internal/behavior.go", "deleted": True}])


if __name__ == "__main__":
    unittest.main()
