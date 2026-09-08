import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import subprocess
import sys
import unittest

SPEC = importlib.util.spec_from_file_location("collect_codex", Path(__file__).with_name("collect_codex.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)

COUNTERS = dict(input_tokens=100, cached_input_tokens=20, cache_write_input_tokens=0,
                output_tokens=30, reasoning_output_tokens=10, total_tokens=130)

def row(kind, payload):
    return {"timestamp": "2026-09-08T12:00:00Z", "type": kind, "payload": payload}

def fixture():
    return [
        row("session_meta", dict(id="actor-1", session_id="parent", agent_path="/root/trial", cli_version="1", model_provider="provider", instructions="PRIVATE", auth="SECRET")),
        row("turn_context", dict(model="model", effort="high", base_instructions="PRIVATE")),
        row("response_item", dict(type="message", role="user", content=[dict(type="input_text", text="PRIVATE")])),
        row("response_item", dict(type="reasoning", summary="PRIVATE", encrypted_content="SECRET")),
        row("response_item", dict(type="custom_tool_call", call_id="call-1", name="functions.exec", input="text(1)")),
        row("token_usage_record", dict(thread_id="actor-1", response_id="response-1", usage=COUNTERS.copy(), thread_token_usage=COUNTERS.copy())),
        row("event_msg", dict(type="token_count", info=dict(total_token_usage=COUNTERS.copy()))),
        row("response_item", dict(type="message", role="assistant", phase="final_answer", content=[dict(type="output_text", text="{\"answer\": 42}")])),
        row("event_msg", dict(type="task_complete", started_at="2026-09-08T12:00:00Z", completed_at="2026-09-08T12:00:02Z", duration_ms=2000)),
    ]

class CollectorTests(unittest.TestCase):
    def collect(self, rows):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "trial.jsonl"
            path.write_text("".join(json.dumps(r) + "\n" for r in rows))
            return MODULE.collect_log(path)

    def test_complete_allowlisted_report(self):
        result = self.collect(fixture())
        self.assertEqual(result["answer"]["parsed_json"], {"answer": 42})
        self.assertEqual(result["runtime_usage"]["counters"], COUNTERS)
        self.assertEqual(result["runtime_usage"]["unique_responses"], 1)
        self.assertEqual(result["runtime"]["actor_id"], "actor-1")
        self.assertEqual(result["completion"]["duration_ms"], 2000)
        self.assertEqual(result["tool_calls"][0]["input"], "text(1)")
        serialized = json.dumps(result)
        self.assertNotIn("PRIVATE", serialized)
        self.assertNotIn("SECRET", serialized)
        self.assertNotIn("encrypted_content", serialized)

    def test_identical_response_duplicate_counted_once(self):
        rows = fixture()
        rows.insert(6, copy.deepcopy(rows[5]))
        result = self.collect(rows)
        self.assertEqual(result["runtime_usage"]["counters"], COUNTERS)
        self.assertEqual(result["runtime_usage"]["responses"],
                         [{"response_id": "response-1", "counters": COUNTERS}])

    def test_sums_responses_not_running_totals(self):
        rows = fixture()
        other = copy.deepcopy(rows[5])
        other["payload"]["response_id"] = "response-2"
        other["payload"]["thread_token_usage"] = {k: v * 2 for k, v in COUNTERS.items()}
        rows.insert(6, other)
        result = self.collect(rows)
        self.assertEqual(result["runtime_usage"]["counters"], {k: v * 2 for k, v in COUNTERS.items()})
        self.assertEqual(result["runtime_usage"]["unique_responses"], 2)
        evidence = result["runtime_usage"]["responses"]
        self.assertEqual([record["response_id"] for record in evidence], ["response-1", "response-2"])
        recomputed = {key: sum(record["counters"][key] for record in evidence) for key in COUNTERS}
        self.assertEqual(recomputed, result["runtime_usage"]["counters"])

    def test_invalid_usage_is_unknown(self):
        for mutation in ["missing_counter", "negative", "boolean", "float", "wrong_thread", "missing_response", "cumulative_mismatch"]:
            with self.subTest(mutation=mutation):
                rows = fixture()
                usage = rows[5]["payload"]
                if mutation == "missing_counter": del usage["usage"]["total_tokens"]
                elif mutation == "negative": usage["usage"]["input_tokens"] = -1
                elif mutation == "boolean": usage["usage"]["input_tokens"] = True
                elif mutation == "float": usage["usage"]["input_tokens"] = 1.5
                elif mutation == "wrong_thread": usage["thread_id"] = "other"
                elif mutation == "missing_response": del usage["response_id"]
                else: usage["thread_token_usage"]["input_tokens"] += 1
                result = self.collect(rows)
                self.assertIsNone(result["runtime_usage"]["counters"])
                self.assertTrue(result["runtime_usage"]["unknown_reasons"])
                self.assertIsNone(result["runtime_usage"]["responses"])

    def test_conflicting_duplicate_and_metadata_unknown(self):
        for variant in ["usage", "metadata", "model"]:
            rows = fixture()
            if variant == "usage":
                other = copy.deepcopy(rows[5])
                other["payload"]["usage"]["input_tokens"] += 1
                rows.insert(6, other)
            elif variant == "metadata":
                other = copy.deepcopy(rows[0])
                other["payload"]["id"] = "other"
                rows.insert(1, other)
            else:
                rows.insert(2, row("turn_context", dict(model="different", effort="high")))
            result = self.collect(rows)
            self.assertIsNone(result["runtime_usage"]["counters"])

    def test_incomplete_trial_unknown(self):
        result = self.collect(fixture()[:-1])
        self.assertFalse(result["completion"]["complete"])
        self.assertIsNone(result["runtime_usage"]["counters"])

    def test_numeric_completion_timestamps(self):
        rows = fixture()
        rows[-1]["payload"].update(started_at=1788870922, completed_at=1788870969, duration_ms=46335)
        result = self.collect(rows)
        self.assertTrue(result["completion"]["complete"])
        self.assertEqual(result["completion"]["started_at"], 1788870922)
        self.assertEqual(result["completion"]["completed_at"], 1788870969)
        self.assertEqual(result["runtime_usage"]["counters"], COUNTERS)

    def test_invalid_numeric_completion_timestamps(self):
        for start, end in [(2, 1), (-1, 2), (True, 2), (1, False)]:
            with self.subTest(start=start, end=end):
                rows = fixture()
                rows[-1]["payload"].update(started_at=start, completed_at=end)
                result = self.collect(rows)
                self.assertFalse(result["completion"]["complete"])
                self.assertIsNone(result["runtime_usage"]["counters"])

    def test_fractional_completion_timestamps(self):
        rows = fixture()
        rows[-1]["payload"].update(started_at=1788870922.1, completed_at=1788870969.9)
        result = self.collect(rows)
        self.assertTrue(result["completion"]["complete"])
        self.assertEqual(result["completion"]["started_at"], 1788870922.1)

    def test_missing_final_or_usage_unknown(self):
        for remove in [5, 7]:
            rows = fixture()
            del rows[remove]
            self.assertIsNone(self.collect(rows)["runtime_usage"]["counters"])

    def test_function_input_retained_without_execution(self):
        rows = fixture()
        rows.insert(5, row("response_item", dict(type="function_call", call_id="call-2", name="dangerous", arguments="{not executed}")))
        result = self.collect(rows)
        self.assertEqual(result["tool_calls"][1]["input"], "{not executed}")

    def test_malformed_metadata_does_not_export_nested_fields(self):
        rows = fixture()
        rows[0]["payload"]["agent_path"] = {"auth": "SECRET"}
        rows[4]["payload"]["input"] = {"auth": "SECRET"}
        result = self.collect(rows)
        self.assertIsNone(result["runtime_usage"]["counters"])
        self.assertNotIn("SECRET", json.dumps(result))

    def test_missing_metadata_and_cumulative_are_unknown(self):
        for variant in ["session", "context", "cumulative"]:
            rows = fixture()
            if variant == "session": del rows[0]
            elif variant == "context": del rows[1]
            else: del rows[5]["payload"]["thread_token_usage"]
            result = self.collect(rows)
            self.assertIsNone(result["runtime_usage"]["counters"])
            self.assertIsNone(result["runtime_usage"]["final_cumulative_counters"])

    def test_cli_output_is_exclusive(self):
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / "trial.jsonl"
            output = Path(directory) / "report.json"
            log.write_text("".join(json.dumps(r) + "\n" for r in fixture()))
            command = [sys.executable, str(Path(__file__).with_name("collect_codex.py")),
                       "--log", str(log), "--output", str(output)]
            first = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(first.returncode, 0, first.stderr)
            original = output.read_bytes()
            second = subprocess.run(command, capture_output=True, text=True)
            self.assertNotEqual(second.returncode, 0)
            self.assertEqual(output.read_bytes(), original)

    def test_invalid_json_fails_without_echoing_content(self):
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / "trial.jsonl"
            log.write_text("SECRET not JSON\n")
            with self.assertRaisesRegex(ValueError, "Invalid JSON at line 1") as raised:
                MODULE.collect_log(log)
            self.assertNotIn("SECRET", str(raised.exception))

    def test_duplicate_final_answer_keys_are_parse_errors(self):
        rows = fixture()
        rows[7]["payload"]["content"][0]["text"] = '{"answer":42,"answer":0}'
        result = self.collect(rows)
        self.assertIsNone(result["answer"]["parsed_json"])
        self.assertEqual(result["answer"]["parse_error"], "Final answer is not valid JSON")

    def test_duplicate_log_identity_or_usage_fails_closed(self):
        for raw in [
            '{"type":"session_meta","payload":{"id":"actor-1","id":"other"}}',
            '{"type":"token_usage_record","payload":{"usage":{"input_tokens":1,"input_tokens":2}}}',
        ]:
            with self.subTest(raw=raw), tempfile.TemporaryDirectory() as directory:
                log = Path(directory) / "trial.jsonl"
                log.write_text(raw + "\n")
                with self.assertRaisesRegex(ValueError, "Invalid JSON at line 1"):
                    MODULE.collect_log(log)

    def test_nonfinite_json_constants_fail_closed(self):
        for constant in ["NaN", "Infinity", "-Infinity"]:
            with self.subTest(constant=constant), tempfile.TemporaryDirectory() as directory:
                log = Path(directory) / "trial.jsonl"
                log.write_text('{"type":"token_usage_record","payload":{"usage":' + constant + '}}\n')
                with self.assertRaisesRegex(ValueError, "Invalid JSON at line 1"):
                    MODULE.collect_log(log)
                rows = fixture()
                rows[7]["payload"]["content"][0]["text"] = '{"answer":' + constant + '}'
                result = self.collect(rows)
                self.assertIsNone(result["answer"]["parsed_json"])
                self.assertIsNotNone(result["answer"]["parse_error"])

if __name__ == "__main__":
    unittest.main()
