import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location("pilot_report", Path(__file__).with_name("report.py"))
report = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(report)


class ReportTests(unittest.TestCase):
    def setUp(self):
        self.tasks = {"tasks": [{"id": "task"}], "treatments": {"ordinary": "", "assisted": ""},
                      "run_order": [{"id": "trial", "task": "task", "treatment": "ordinary"}]}
        self.grading = {"answers": {"task": {"flag": False, "count": 0, "name": "ok"}}}
        self.trial = {"id": "trial", "task": "task", "treatment": "ordinary",
                      "answers": {"flag": False, "count": 0, "name": "ok"},
                      "evidence": {key: "source:1 supports this" for key in ("flag", "count", "name")},
                      "source_validity": "valid", "trace_validity": "valid"}

    def grade(self, trial=None):
        return report.build_report([trial or self.trial], self.tasks, self.grading)["trials"][0]

    def test_exact_types_missing_and_null(self):
        self.trial["answers"] = {"flag": 0, "count": False, "name": None}
        row = self.grade()
        self.assertEqual(row["correct_fields"], 0)
        self.trial["answers"] = {}
        self.assertEqual(self.grade()["correct_fields"], 0)
        self.assertFalse(report.exact_json(0, 0.0))
        self.assertFalse(report.exact_json([False], [0]))

    def test_correctness_requires_separate_evidence_review(self):
        row = self.grade()
        self.assertTrue(row["automated_correct"])
        self.assertEqual(row["outcome"], "unverified")
        self.assertIsNone(row["task_pass"])
        self.trial["evidence_review"] = {key: "supported" for key in self.trial["answers"]}
        self.assertTrue(self.grade()["task_pass"])
        self.trial["evidence_review"]["name"] = "unsupported"
        self.assertEqual(self.grade()["outcome"], "incorrect")
        self.assertFalse(self.grade()["task_pass"])

    def test_review_without_citation_cannot_pass(self):
        self.trial["evidence_review"] = {key: "supported" for key in self.trial["answers"]}
        self.trial["evidence"] = {}
        self.assertEqual(self.grade()["outcome"], "unverified")

    def test_invalid_failure_unverified_and_original_preserved(self):
        original = copy.deepcopy(self.trial)
        result = report.build_report([self.trial], self.tasks, self.grading)
        self.assertEqual(self.trial, original)
        self.assertEqual(result["trials"][0]["original"], original)
        self.trial["source_validity"] = "invalid"
        self.trial["failure"] = "timeout"
        row = self.grade()
        self.assertEqual(row["outcome"], "invalid")
        self.assertEqual(row["failure"], "timeout")
        self.trial["source_validity"] = "valid"
        self.assertEqual(self.grade()["outcome"], "infrastructure_failure")
        del self.trial["failure"]
        self.trial["trace_validity"] = "unverified"
        self.assertEqual(self.grade()["outcome"], "unverified")

    def test_expected_set_duplicates_and_unknown_identity(self):
        duplicate = report.build_report([self.trial, self.trial], self.tasks, self.grading)
        self.assertEqual(duplicate["duplicate_ids"], ["trial"])
        self.assertTrue(all(r["outcome"] == "invalid" for r in duplicate["trials"]))
        self.assertFalse(duplicate["complete"])
        trial = dict(self.trial, task="unknown", treatment="unknown")
        result = report.build_report([trial], self.tasks, self.grading)
        self.assertEqual(result["missing_ids"], ["trial"])
        self.assertEqual(result["trials"][0]["outcome"], "invalid")
        self.assertEqual(report.build_report([], self.tasks, self.grading)["missing_ids"], ["trial"])

    def test_unknown_measurements_are_not_zero_or_self_reported(self):
        self.trial.update(observed_text_bytes=25, provider_usage={"input_tokens": 50})
        row = self.grade()
        self.assertIsNone(row["observed_text_bytes"])
        self.assertIsNone(row["provider_usage"])
        self.trial.update(observed_text_bytes=0, text_measurement_verified=True,
                          provider_usage_verified=True, wall_time_seconds=0)
        row = self.grade()
        self.assertEqual(row["observed_text_bytes"], 0)
        self.assertEqual(row["provider_usage"], {"input_tokens": 50})
        self.assertEqual(row["wall_time_seconds"], 0)
        self.trial["observed_text_bytes"] = False
        self.assertEqual(self.grade()["outcome"], "invalid")

    def test_malformed_records_retained(self):
        result = report.build_report([None, {"id": "trial", "answers": []}], self.tasks, self.grading)
        self.assertEqual(len(result["trials"]), 2)
        self.assertTrue(all(r["outcome"] == "invalid" for r in result["trials"]))

    def test_invalid_measurements_and_status_types(self):
        for updates in ({"observed_text_bytes": -1}, {"wall_time_seconds": float("inf")},
                        {"provider_usage": {"input_tokens": True}}, {"trace_validity": []},
                        {"text_measurement_verified": "true"}, {"answers": []},
                        {"evidence_review": {"flag": True}}, {"task": []}):
            with self.subTest(updates=updates):
                self.assertEqual(self.grade(dict(self.trial, **updates))["outcome"], "invalid")
        trial = dict(self.trial, observed_text_bytes=10 ** 400, text_measurement_verified=True)
        self.assertEqual(self.grade(trial)["observed_text_bytes"], 10 ** 400)

    def test_json_rejects_duplicate_keys_and_nonfinite_constants(self):
        with tempfile.TemporaryDirectory() as temp:
            source = Path(temp) / "input.json"
            for text in ('{"answers":{"a":0,"a":1}}', '{"value":NaN}', '{"value":Infinity}'):
                source.write_text(text)
                with self.assertRaises(ValueError):
                    report._read_json(source)

    def test_source_hash_and_output_protection(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = root / "source.txt"
            source.write_bytes(b"hello")
            manifest = {"files": [{"file": "source.txt", "bytes": 5,
                         "sha256": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"}]}
            self.assertTrue(report.verify_sources(root, manifest)["valid"])
            source.write_bytes(b"wrong")
            self.assertFalse(report.verify_sources(root, manifest)["valid"])
            manifest["files"][0]["file"] = "../escape"
            self.assertFalse(report.verify_sources(root, manifest)["valid"])
            destination = root / "result.json"
            report.write_report(destination, {"a": 1})
            with self.assertRaises(FileExistsError):
                report.write_report(destination, {"a": 2})
            self.assertEqual(json.loads(destination.read_text()), {"a": 1})


if __name__ == "__main__":
    unittest.main()
