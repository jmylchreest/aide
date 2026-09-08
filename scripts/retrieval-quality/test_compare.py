import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from compare import build_comparison
from report import _read_json, write_report


def fixture():
    original = {"id": "one_ordinary", "task": "one", "treatment": "ordinary",
                "text_measurement_verified": True, "provider_usage_verified": True,
                "source_calls": 3, "model_responses": 4,
                "runtime": {"model": "pilot-model", "effort": "high",
                            "model_provider": "pilot-provider", "cli_version": "1.2.3"}}
    row = {"id": "one_ordinary", "task": "one", "treatment": "ordinary",
           "outcome": "pass", "task_pass": True, "issues": [], "failure": None,
           "observed_text_bytes": 100, "provider_usage": {"input_tokens": 200},
           "wall_time_seconds": 2.5, "original": original}
    return {"schema": 1, "kind": "retrieval_quality_pilot_report", "complete": True,
            "expected_trials": 1, "recorded_trials": 1,
            "missing_ids": [], "duplicate_ids": [], "trials": [row]}


class ComparisonTests(unittest.TestCase):
    def test_signed_deltas_and_no_mutation(self):
        before, after = fixture(), fixture()
        after["trials"][0]["observed_text_bytes"] = 80
        after["trials"][0]["provider_usage"]["input_tokens"] = 240
        saved = copy.deepcopy((before, after))
        row = build_comparison(before, after)["pairs"][0]
        self.assertEqual(row["metrics"]["observed_text_bytes"],
                         {"before": 100, "after": 80, "delta": -20})
        self.assertEqual(row["metrics"]["provider_usage"]["input_tokens"]["delta"], 40)
        self.assertEqual(row["metrics"]["source_calls"]["delta"], 0)
        self.assertEqual(row["metrics"]["model_responses"]["delta"], 0)
        self.assertTrue(row["comparison_conditions"]["runtime_metadata_matches"])
        self.assertEqual(row["comparison_conditions"]["reasons"], [])
        self.assertEqual((before, after), saved)

    def test_runtime_mismatch_suppresses_only_runtime_deltas(self):
        for field in ("model", "effort", "model_provider", "cli_version"):
            with self.subTest(field=field):
                before, after = fixture(), fixture()
                after["trials"][0]["original"]["runtime"][field] = "different"
                pair = build_comparison(before, after)["pairs"][0]
                conditions = pair["comparison_conditions"]
                self.assertFalse(conditions["runtime_metadata_matches"])
                self.assertIn(f"runtime.{field} differs between before and after", conditions["reasons"])
                self.assertEqual(conditions["runtime"][field]["after"], "different")
                metrics = pair["metrics"]
                self.assertIsNone(metrics["provider_usage"]["input_tokens"]["delta"])
                self.assertEqual(metrics["provider_usage"]["input_tokens"]["after"], 200)
                for key in ("model_responses", "wall_time_seconds"):
                    self.assertIsNone(metrics[key]["delta"])
                    self.assertIsNotNone(metrics[key]["after"])
                for key in ("source_calls", "observed_text_bytes"):
                    self.assertEqual(metrics[key]["delta"], 0)

    def test_missing_or_empty_runtime_does_not_establish_equivalence(self):
        for runtime in (None, {}, {"model": ""}, {"model": "   "}, {"model": 1}):
            with self.subTest(runtime=runtime):
                before, after = fixture(), fixture()
                for report in (before, after):
                    report["trials"][0]["original"]["runtime"] = runtime
                pair = build_comparison(before, after)["pairs"][0]
                self.assertFalse(pair["comparison_conditions"]["runtime_metadata_matches"])
                self.assertTrue(pair["comparison_conditions"]["reasons"])
                self.assertIsNone(pair["metrics"]["provider_usage"]["input_tokens"]["delta"])
                self.assertIsNone(pair["metrics"]["model_responses"]["delta"])
                self.assertIsNone(pair["metrics"]["wall_time_seconds"]["delta"])

    def test_unknown_metrics_stay_unknown_and_keep_known_side(self):
        before, after = fixture(), fixture()
        after["trials"][0].update(observed_text_bytes=None, provider_usage=None)
        metrics = build_comparison(before, after)["pairs"][0]["metrics"]
        self.assertEqual(metrics["observed_text_bytes"], {"before": 100, "after": None, "delta": None})
        self.assertEqual(metrics["provider_usage"]["input_tokens"],
                         {"before": 200, "after": None, "delta": None})

    def test_failed_unverified_and_incorrect_rows_retained_without_deltas(self):
        for outcome in ("infrastructure_failure", "unverified", "incorrect", "invalid"):
            with self.subTest(outcome=outcome):
                before, after = fixture(), fixture()
                after["trials"][0].update(outcome=outcome, task_pass=None, failure="retained")
                pair = build_comparison(before, after)["pairs"][0]
                self.assertEqual(pair["after"]["failure"], "retained")
                self.assertEqual(pair["after"]["outcome"], outcome)
                self.assertEqual(pair["metrics"]["observed_text_bytes"]["after"], 100)
                self.assertIsNone(pair["metrics"]["observed_text_bytes"]["delta"])
                self.assertIsNone(pair["metrics"]["wall_time_seconds"]["delta"])

    def test_measurements_need_attestation(self):
        before, after = fixture(), fixture()
        after["trials"][0]["original"].update(text_measurement_verified=False, provider_usage_verified=False)
        metrics = build_comparison(before, after)["pairs"][0]["metrics"]
        self.assertIsNone(metrics["observed_text_bytes"]["delta"])
        self.assertIsNone(metrics["provider_usage"]["input_tokens"]["delta"])
        self.assertIsNone(metrics["model_responses"]["delta"])

    def test_bad_numeric_values_cannot_produce_deltas(self):
        for bad in (-1, True, "2", float("nan"), float("inf")):
            with self.subTest(bad=bad):
                before, after = fixture(), fixture()
                row = after["trials"][0]
                row.update(observed_text_bytes=bad, wall_time_seconds=bad, provider_usage={"input_tokens": bad})
                row["original"].update(source_calls=bad, model_responses=bad)
                metrics = build_comparison(before, after)["pairs"][0]["metrics"]
                for key in ("observed_text_bytes", "wall_time_seconds", "source_calls", "model_responses"):
                    self.assertIsNone(metrics[key]["after"])
                    self.assertIsNone(metrics[key]["delta"])
                self.assertIsNone(metrics["provider_usage"]["input_tokens"]["delta"])

    def test_duplicate_and_identity_mismatch_rejected(self):
        after = fixture()
        after["trials"].append(copy.deepcopy(after["trials"][0]))
        with self.assertRaises(ValueError):
            build_comparison(fixture(), after)
        for container in ("row", "original"):
            after = fixture()
            row = after["trials"][0]
            (row if container == "row" else row["original"])["id"] = "other"
            with self.assertRaisesRegex(ValueError, "identity"):
                build_comparison(fixture(), after)

    def test_missing_peer_or_incomplete_report_rejected(self):
        after = fixture()
        after["trials"] = []
        with self.assertRaises(ValueError):
            build_comparison(fixture(), after)
        after = fixture()
        after.update(complete=False, missing_ids=["missing"])
        with self.assertRaises(ValueError):
            build_comparison(fixture(), after)

    def test_pairing_uses_task_and_treatment_not_order(self):
        before = fixture()
        row = copy.deepcopy(before["trials"][0])
        for target in (row, row["original"]):
            target.update(id="one_assisted", treatment="assisted")
        before["trials"].append(row)
        before.update(expected_trials=2, recorded_trials=2)
        after = copy.deepcopy(before)
        after["trials"].reverse()
        self.assertEqual(len(build_comparison(before, after)["pairs"]), 2)

    def test_strict_json_and_exclusive_cli_output(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            bad = path / "bad.json"
            for content in ('{"schema":1,"schema":1}', '{"n":NaN}'):
                bad.write_text(content)
                with self.assertRaises(ValueError):
                    _read_json(bad)
            before, after, output = (path / name for name in ("before.json", "after.json", "output.json"))
            write_report(before, fixture())
            write_report(after, fixture())
            command = [sys.executable, "-B", str(Path(__file__).with_name("compare.py")),
                       "--before", str(before), "--after", str(after), "--output", str(output)]
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 0)
            content = output.read_bytes()
            self.assertEqual(json.loads(content)["kind"], "retrieval_quality_pilot_comparison")
            self.assertNotEqual(subprocess.run(command, capture_output=True).returncode, 0)
            self.assertEqual(output.read_bytes(), content)


if __name__ == "__main__":
    unittest.main()
