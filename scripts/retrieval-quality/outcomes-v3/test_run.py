import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import shlex

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
            "shared_instructions": "Work inside {{ROOT}}. Literal ROOT stays literal.",
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

    def test_bridge_metrics_remain_separate_from_handler_duration(self):
        metrics = {"setup_index_ms": 25, "total_ms": 8, "guard_ms": 1,
                   "startup_ms": 2, "identity_ms": 1, "request_ms": 3,
                   "exit_ms": 1, "successful_calls": 1, "failed_calls": 0,
                   "tool_calls": 1, "emitted_text_bytes": 30}
        row = pilot.report(self.protocol, {"trials": [self.review(bridge_metrics=metrics)]})["trials"][0]
        self.assertEqual(row["bridge_metrics"], metrics)
        self.assertIsNone(row["aide_duration_ms"])
        missing = pilot.report(self.protocol, {"trials": [self.review(bridge_metrics=[])]})["trials"][0]
        self.assertIsNone(missing["bridge_metrics"])

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

    def bridge_package(self):
        self.binary = self.root / "frozen binary"
        self.binary.write_bytes(b"frozen-test-binary")
        (self.package / "bridge.py").write_text("# frozen bridge\n")
        self.protocol["retrieval_bridge"] = {
            "path": "bridge.py",
            "binary_sha256": hashlib.sha256(self.binary.read_bytes()).hexdigest(),
        }
        self.protocol["treatments"]["available"] = (
            "rtk proxy python3 {{BRIDGE}} call --root {{ROOT}} --binary {{BINARY}} "
            "--evidence-dir {{EVIDENCE}} --tool-json '{\"name\":\"code_search\",\"arguments\":{\"query\":\"x\"}}'"
        )
        (self.package / "navigation/prompt.md").write_text("Find source under {{ROOT}}.")
        (self.package / "protocol.json").write_text(json.dumps(self.protocol))
        self.freeze()

    def test_prepare_resolves_frozen_bridge_and_isolated_paths_in_all_prompt_parts(self):
        self.bridge_package()
        dest = self.root / "trials with spaces"
        ledger = pilot.prepare(self.package, dest, binary=self.binary)
        prompt = (dest / "t2/participant-prompt.md").read_text()
        for value in [self.package / "bridge.py", self.binary, dest / "t2/root", dest / "t2/bridge-evidence"]:
            self.assertIn(shlex.quote(str(value)), prompt)
        self.assertNotIn("{{", prompt)
        self.assertIn("Literal ROOT stays literal", prompt)
        self.assertEqual(ledger["trials"][1]["bridge_evidence_dir"], str(dest / "t2/bridge-evidence"))
        self.assertEqual(ledger["retrieval_bridge"]["binary_sha256"], self.protocol["retrieval_bridge"]["binary_sha256"])
        self.assertNotIn(str(self.binary), (dest / "t1/participant-prompt.md").read_text())

    def test_prepare_rejects_unfrozen_bridge_or_binary_before_creating_trials(self):
        self.bridge_package()
        for mode in ("missing_binary", "changed_binary", "uncovered_bridge"):
            with self.subTest(mode=mode):
                self.binary.write_bytes(b"frozen-test-binary")
                self.freeze()
                binary = self.binary
                if mode == "missing_binary":
                    binary = None
                elif mode == "changed_binary":
                    self.binary.write_bytes(b"changed")
                else:
                    manifest = json.loads((self.package / "manifest.json").read_text())
                    del manifest["files"]["bridge.py"]
                    (self.package / "manifest.json").write_text(json.dumps(manifest))
                dest = self.root / mode
                with self.assertRaises(ValueError):
                    pilot.prepare(self.package, dest, binary=binary)
                self.assertFalse(dest.exists())

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
