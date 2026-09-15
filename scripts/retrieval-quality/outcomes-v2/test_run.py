import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location("pilot_run", Path(__file__).with_name("run.py"))
pilot = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(pilot)


class PilotTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.package = self.root / "package"
        (self.package / "navigation/template").mkdir(parents=True)
        (self.package / "navigation/template/source.py").write_text("original\n")
        (self.package / "navigation/prompt.md").write_text("Find the source.")
        self.protocol = {
            "shared_instructions": "Work inside ROOT.",
            "treatments": {"ordinary": "Ordinary tools.", "available": "Optional aide."},
            "trial_order": [dict(id=f"t{i}", task="navigation", treatment=t, repetition=1)
                            for i, t in enumerate(("ordinary", "available"), 1)],
        }
        (self.package / "protocol.json").write_text(json.dumps(self.protocol))
        self.freeze()

    def freeze(self):
        files = {str(p.relative_to(self.package)): hashlib.sha256(p.read_bytes()).hexdigest()
                 for p in self.package.rglob("*") if p.is_file() and p.name != "manifest.json"}
        (self.package / "manifest.json").write_text(json.dumps({"files": files}))

    def capture(self, actor="actor-1", malformed=False):
        path = self.root / f"{actor}.jsonl"
        counts = {key: 0 for key in pilot.collector.COUNTERS}
        counts.update(input_tokens=100, output_tokens=20, total_tokens=120)
        rows = [
            {"type": "session_meta", "payload": {"id": actor, "session_id": "session",
                "agent_path": "/root/trial", "cli_version": "1", "model_provider": "provider"}},
            {"type": "turn_context", "payload": {"model": "model", "effort": "high"}},
            {"type": "token_usage_record", "payload": {"thread_id": actor, "response_id": "r1",
                "usage": counts, "thread_token_usage": counts}},
            {"type": "event_msg", "payload": {"type": "task_complete", "started_at": 1,
                "completed_at": 2, "duration_ms": 1000}},
            {"type": "response_item", "payload": {"type": "message", "role": "assistant",
                "phase": "final_answer", "content": [{"type": "output_text", "text": "Done"}]}},
        ]
        path.write_text("broken" if malformed else "\n".join(json.dumps(row) for row in rows))
        return str(path)

    def review(self, **overrides):
        row = dict(self.protocol["trial_order"][0], agent_id="actor-1", log_path=self.capture(),
                   status="completed", scope_verified=True, trace_reviewed=True,
                   quality_verified=True, quality={"passed": True}, aide_operations=0,
                   aide_uptake=False)
        row.update(overrides)
        return row

    def test_report_provenance_rejects_changed_protocol_or_collector(self):
        self.protocol["collector_sha256"] = hashlib.sha256(Path(pilot.collector.__file__).read_bytes()).hexdigest()
        path = self.package / "protocol.json"
        path.write_text(json.dumps(self.protocol))
        self.freeze()
        ledger = {"protocol_sha256": hashlib.sha256(path.read_bytes()).hexdigest(), "trials": []}
        self.assertEqual(pilot.verified_report(path, ledger)["provenance"]["protocol_sha256"], ledger["protocol_sha256"])
        with self.assertRaises(ValueError):
            pilot.verified_report(path, dict(ledger, protocol_sha256="0" * 64))
        self.protocol["collector_sha256"] = "0" * 64
        path.write_text(json.dumps(self.protocol))
        self.freeze()
        ledger["protocol_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        with self.assertRaises(ValueError):
            pilot.verified_report(path, ledger)

    def test_malformed_ledger_rejected(self):
        for ledger in ({}, {"trials": "bad"}, {"trials": [42]}, {"trials": [{"id": []}]},
                       {"trials": [{"id": "unknown"}]}):
            with self.subTest(ledger=ledger), self.assertRaises(ValueError):
                pilot.report(self.protocol, ledger)

    def test_missing_and_failed_rows_retained(self):
        result = pilot.report(self.protocol, {"trials": [self.review(status="failed")]})
        self.assertEqual(len(result["trials"]), 2)
        self.assertEqual(result["trials"][0]["status"], "failed")
        self.assertEqual(result["trials"][1]["status"], "missing")
        self.assertIsNone(result["trials"][1]["runtime_usage"])
        self.assertEqual(result["comparisons"], [])

    def test_identity_must_match(self):
        result = pilot.report(self.protocol, {"trials": [self.review(agent_id="other")]})
        row = result["trials"][0]
        self.assertFalse(row["protocol_valid"])
        self.assertIn("agent_identity_mismatch", row["unknown_reasons"])
        self.assertIsNone(row["runtime_usage"])

    def test_zero_aide_uptake_is_valid(self):
        result = pilot.report(self.protocol, {"trials": [self.review()]})
        self.assertTrue(result["trials"][0]["protocol_valid"])
        self.assertEqual(result["trials"][0]["aide_operations"], 0)

    def test_malformed_capture_unknown(self):
        reviewed = self.review()
        reviewed["log_path"] = self.capture(malformed=True)
        result = pilot.report(self.protocol, {"trials": [reviewed]})
        self.assertIsNone(result["trials"][0]["runtime_usage"])
        self.assertIn("capture_unreadable_or_malformed", result["trials"][0]["unknown_reasons"])

    def test_prepare_and_no_overwrite(self):
        dest = self.root / "trials"
        pilot.prepare(self.package, dest)
        self.assertEqual((dest / "t1/root/source.py").read_text(), "original\n")
        self.assertIn(str(dest / "t1/root"), (dest / "t1/participant-prompt.md").read_text())
        with self.assertRaises(FileExistsError):
            pilot.prepare(self.package, dest)

    def test_manifest_mutation_rejected_before_destination_created(self):
        (self.package / "navigation/template/source.py").write_text("changed")
        dest = self.root / "trials"
        with self.assertRaises(ValueError):
            pilot.prepare(self.package, dest)
        self.assertFalse(dest.exists())

    def test_manifest_must_cover_every_template_file(self):
        (self.package / "navigation/template/extra.py").write_text("unfrozen")
        with self.assertRaises(ValueError):
            pilot.prepare(self.package, self.root / "trials")

    def test_comparisons_require_pass_and_matching_runtime(self):
        first = self.review()
        second = dict(first, **self.protocol["trial_order"][1], agent_id="actor-2",
                      log_path=self.capture(actor="actor-2"))
        self.assertEqual(len(pilot.report(self.protocol, {"trials": [first, second]})["comparisons"]), 1)
        second["quality"] = {"passed": False}
        result = pilot.report(self.protocol, {"trials": [first, second]})
        self.assertEqual(result["comparisons"], [])
        self.assertIsNotNone(result["trials"][1]["runtime_usage"])
        second["quality"] = {"passed": True}
        path = Path(second["log_path"])
        path.write_text(path.read_text().replace('"model": "model"', '"model": "different"'))
        self.assertEqual(pilot.report(self.protocol, {"trials": [first, second]})["comparisons"], [])

    def test_separate_runtime_identity_and_malformed_review_remain_unknown(self):
        reviewed = self.review(agent_id="/root/trial", runtime_session_id="actor-1",
                               aide_operations=True, aide_uptake="yes", aide_duration_ms=-1)
        result = pilot.report(self.protocol, {"trials": [reviewed]})["trials"][0]
        self.assertTrue(result["protocol_valid"])
        self.assertIsNone(result["aide_operations"])
        self.assertIsNone(result["aide_uptake"])
        self.assertIsNone(result["aide_duration_ms"])

    def test_reviewed_deviation_and_reused_actor_prevent_comparison(self):
        first = self.review(protocol_violations=["forbidden_tool"])
        row = pilot.report(self.protocol, {"trials": [first]})["trials"][0]
        self.assertFalse(row["protocol_valid"])
        self.assertIsNotNone(row["runtime_usage"])
        first["protocol_violations"] = []
        second = dict(first, **self.protocol["trial_order"][1])
        result = pilot.report(self.protocol, {"trials": [first, second]})
        self.assertEqual(result["comparisons"], [])
        self.assertTrue(all("reused_runtime_identity" in row["unknown_reasons"] for row in result["trials"]))


if __name__ == "__main__":
    unittest.main()
