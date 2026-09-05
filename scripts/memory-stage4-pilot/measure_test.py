import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("pilot_measure", Path(__file__).with_name("measure.py"))
measure = importlib.util.module_from_spec(spec)
spec.loader.exec_module(measure)


class PilotReportTest(unittest.TestCase):
    def test_independent_factors_and_retained_levels(self):
        variants = measure.variants()
        base = variants[0][1]
        for _, variant in variants[1:]:
            self.assertEqual(sum(variant[k] != base[k] for k in base), 1)
        self.assertEqual({v["retained-events"] for _, v in variants}, {10000, 100000, 1000000})

    def test_counts_and_missing_measurements_are_not_zero(self):
        self.assertEqual(measure.distribution([]), {"n": 0, "p50": None, "p95": None, "max": None})
        self.assertEqual(measure.distribution([3, 1, 2]), {"n": 3, "p50": 2, "p95": 3, "max": 3})

    def test_incomplete_conformance_and_changed_kernel_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "conformance.json"
            path.write_text(json.dumps({"version": "memory-stage-4-conformance-v1", "status": "incomplete"}))
            with self.assertRaises(ValueError):
                measure.conformance(path)
        receipt = {"source": {"files": [{"path": "internal/eviedb/db.go", "sha256": "old"}]}}
        with self.assertRaises(ValueError):
            measure.validate_baseline_source(receipt, {"files": {"internal/eviedb/db.go": "new"}})

    def test_added_kernel_file_cannot_borrow_conformance(self):
        receipt = {"source": {"files": [{"path": "internal/eviedb/db.go", "sha256": "same"}]}}
        original = {"files": {"internal/eviedb/db.go": "same", "scripts/memory-stage4-pilot/run.go": "new tooling"}}
        measure.validate_baseline_source(receipt, original)
        for actual in [
            {"internal/eviedb/db.go": "same", "internal/eviedb/extra.go": "new"},
            {"internal/eviedb/db.go": "same", "cmd/evie/extra.go": "new"},
            {},
        ]:
            with self.assertRaises(ValueError):
                measure.validate_baseline_source(receipt, {"files": actual})

    def test_missing_and_malformed_trial_receipts_are_preserved_as_failures(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "trial.json"
            missing = measure.trial_observation(path, 86)
            self.assertEqual(missing["status"], "failed")
            self.assertIsNone(missing["report_sha256"])
            for content in [b"", b"{interrupted", b"\xff", b"[]", b'{}']:
                path.write_bytes(content)
                observed = measure.trial_observation(path, 0)
                self.assertEqual(observed["status"], "failed")
                self.assertEqual(observed["report_sha256"], measure.digest(path))
                self.assertEqual(path.read_bytes(), content)

    def test_failed_trials_still_finish_the_versioned_matrix_report(self):
        class FailedProcess:
            returncode = 86
            pid = 1

            def __init__(self, command, **_):
                path = Path(command[command.index("--output") + 1])
                # Missing in one mode, malformed in another, valid JSON with
                # an explicit infrastructure failure in the third.
                mode = command[command.index("--mode") + 1]
                if mode == "new":
                    path.write_text("{interrupted")
                elif mode == "history":
                    path.write_text('{"error":"owned worker failed"}')

            def poll(self):
                return self.returncode

        def build(command, **_):
            Path(command[command.index("-o") + 1]).write_bytes(b"fixture executable")

        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "matrix"
            identity = {"files": {"internal/eviedb/db.go": "same"}, "sha256": "same"}
            baseline = {"source": {"files": [{"path": "internal/eviedb/db.go", "sha256": "same"}]}}
            argv = ["measure", "--conformance", "fixture", "--output", str(output), "--only", "baseline", "--repetitions", "2"]
            with (
                mock.patch.object(measure.sys, "argv", argv),
                mock.patch.object(measure, "conformance", return_value=baseline),
                mock.patch.object(measure, "source_identity", return_value=identity),
                mock.patch.object(measure.platform, "platform", return_value="fixture OS"),
                mock.patch.object(measure.subprocess, "run", side_effect=build),
                mock.patch.object(measure.subprocess, "Popen", FailedProcess),
            ):
                self.assertTrue(measure.main())
            report = json.loads((output / "report.json").read_text())
            self.assertEqual(report["version"], "memory-stage4-pilot-matrix-v1")
            self.assertEqual(report["infrastructure_status"], "failed")
            self.assertEqual(report["pilot_status"], "incomplete")
            self.assertEqual(len(report["runs"]), 6)
            self.assertEqual(len(report["failures"]), 6)
            self.assertFalse(report["paired_deltas"])
            self.assertTrue(all(run["status"] == "failed" for run in report["runs"]))


if __name__ == "__main__":
    unittest.main()
