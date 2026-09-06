import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

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


if __name__ == "__main__":
    unittest.main()
