import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


SPEC = importlib.util.spec_from_file_location("bridge_summary", Path(__file__).with_name("bridge_summary.py"))
summary = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(summary)


class BridgeSummaryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="bridge-summary-test-")
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.source_hash = hashlib.sha256(b"original fixture source").hexdigest()
        self.binding = {"schema_version":1, "id":"binding-1", "root":"/tmp/fixture",
                        "binary":"/tmp/binary", "binary_sha256":"f" * 64,
                        "evidence_dir":"/tmp/original-evidence", "seed_files":{"source.ts":self.source_hash},
                        "index_policy":"seeded; no automatic refresh after participant edits"}
        self.setup = {"operation":"prepare", "binding":self.binding, "error":None, "returncode":0,
                      "index_elapsed_ms":7.25, "total_elapsed_ms":8.5}
        self.write("setup.json", self.setup)

    def write(self, name, value):
        (self.directory / name).write_text(json.dumps(value))

    def record(self, tool="code_outline", text="é\n", work_id="work-1", elapsed="0"):
        digest = hashlib.sha256(text.encode()).hexdigest()
        refs = [{"file":"source.ts", "sha256":self.source_hash, "bytes":23}]
        result = {"content":[{"type":"text","text":text}], "_meta":{
            "aide/work":{"version":1,"id":work_id,"tool":tool,"text_sha256":digest},
            "aide/retrieval":{"version":1,"id":"retrieval-" + work_id,"tool":tool,"text_sha256":digest,"references":refs}}}
        identity = {"project_root":"/tmp/fixture","real_project_root":"/tmp/fixture","cwd":"/tmp/fixture",
                    "db_path":"/tmp/fixture/.aide/memory/memory.db","socket_path":"/tmp/fixture/.aide/aide.sock"}
        identity_result = {"content":[{"type":"text","text":json.dumps(identity)}]}
        request = {"name":tool,"arguments":{"file":"source.ts"}}
        transcript = [
            {"direction":"request","message":{"id":1,"method":"initialize","params":{}}},
            {"direction":"response","message":{"id":1,"result":{}}},
            {"direction":"request","message":{"id":2,"method":"tools/call","params":{"name":"instance_info","arguments":{}}}},
            {"direction":"response","message":{"id":2,"result":identity_result}},
            {"direction":"request","message":{"id":3,"method":"tools/call","params":request}},
            {"direction":"response","message":{"id":3,"result":result}},
        ]
        record = {"operation":"call","id":"call-" + work_id,"binding_id":"binding-1","request":request,
                  "forwarded_request":request,"identity":identity,"index_policy":self.binding["index_policy"],
                  "transcript":transcript,"result":result,"error":None,"returncode":0,
                  "source_files":{"source.ts":self.source_hash},"source_changed_since_seed":False,
                  "timings_ms":{"guard":1,"startup_initialize":2.1,"identity":3,"request":4,"exit":5,"total":15.1}}
        event = {"id":"event-" + work_id,"kind":"tool_call","name":tool,"attrs":{
            "accounting_version":"1","observation_stage":"server_result","work_version":"1",
            "work_id":work_id,"work_text_sha256":digest,"work_elapsed_ms":elapsed,
            "work_outcome":"returned","payload_bytes":str(len(text.encode())),
            "retrieval_id":"retrieval-" + work_id,"source_references":json.dumps(refs)}}
        return record, event

    def summarize(self, events=None, **kwargs):
        return summary.summarize_bridge(self.directory, expected_binding=self.binding,
                                        expected_binary_sha256="f" * 64, work_events=events, **kwargs)

    def test_zero_handler_duration_preserved_separate_from_bridge_and_setup(self):
        record, event = self.record()
        self.write("call.json", record)
        result = self.summarize([event])
        metrics = result["bridge_metrics"]
        self.assertEqual(result["aide_duration_ms"], 0)
        self.assertEqual(metrics["setup"]["index_elapsed_ms"], 7.25)
        self.assertEqual(metrics["setup"]["total_elapsed_ms"], 8.5)
        self.assertEqual(metrics["timings_ms"]["request"]["total"], 4)
        self.assertEqual(metrics["timings_ms"]["total"]["total"], 15.1)
        self.assertEqual(metrics["emitted_text_utf8_bytes"]["total"], 3)
        self.assertTrue(metrics["calls"][0]["receipts"]["retrieval"]["valid"])
        self.assertIsNone(metrics["calls"][0]["receipts"]["retrieval"]["source_references"][0]["byte_count_independently_verified"])

    def test_missing_export_never_infers_duration_from_request_or_receipt_id(self):
        record, _ = self.record()
        self.write("call.json", record)
        result = self.summarize()
        self.assertIsNone(result["aide_duration_ms"])
        self.assertEqual(result["bridge_metrics"]["handler_duration_ms"]["unknown_count"], 1)

    def test_known_source_snapshot_verifies_bytes_only_for_matching_version(self):
        record, event = self.record()
        self.write("call.json", record)
        for content, expected in [(b"original fixture source", True), (b"another source version", None)]:
            with self.subTest(content=content):
                result = self.summarize([event], source_snapshots={"source.ts": [content]})
                reference = result["bridge_metrics"]["calls"][0]["receipts"]["retrieval"]["source_references"][0]
                self.assertIs(reference["byte_count_independently_verified"], expected)
                self.assertEqual(result["aide_duration_ms"], 0)

    def test_matching_source_snapshot_rejects_incorrect_receipt_byte_count(self):
        record, event = self.record()
        refs = record["result"]["_meta"]["aide/retrieval"]["references"]
        refs[0]["bytes"] += 1
        event["attrs"]["source_references"] = json.dumps(refs)
        self.write("call.json", record)
        result = self.summarize([event], source_snapshots={"source.ts": [b"original fixture source"]})
        receipt = result["bridge_metrics"]["calls"][0]["receipts"]["retrieval"]
        self.assertFalse(receipt["valid"])
        self.assertFalse(receipt["source_references"][0]["byte_count_independently_verified"])
        self.assertIsNone(result["aide_duration_ms"])

    def test_rejection_and_shutdown_failure_keep_attempts_and_zero_emitted_text(self):
        record, event = self.record()
        record.update(error="shutdown failed", returncode=-15, forced_shutdown=True)
        self.write("call.json", record)
        self.write("rejected.json", {"operation":"call","id":"rejected","binding_id":"binding-1",
            "index_policy":self.binding["index_policy"],"request":"{malformed","transcript":[],
            "result":None,"error":"bad JSON","timings_ms":{"total":0.5}})
        result = self.summarize([event])
        metrics = result["bridge_metrics"]
        self.assertEqual(metrics["attempts"], 2)
        self.assertEqual(metrics["bridge_errors"], 1)
        self.assertEqual(metrics["rejected"], 1)
        self.assertEqual(metrics["emitted_text_utf8_bytes"]["total"], 0)
        self.assertIsNone(metrics["timings_ms"]["request"]["total"])
        self.assertEqual(metrics["timings_ms"]["request"]["known_sum"], 4)
        self.assertEqual(result["aide_duration_ms"], 0)

    def test_tool_error_is_emitted_and_not_counted_as_success(self):
        record, event = self.record()
        record["result"]["isError"] = True
        event["attrs"]["work_outcome"] = "reported_error"
        self.write("call.json", record)
        result = self.summarize([event])
        self.assertEqual(result["bridge_metrics"]["tool_errors"], 1)
        self.assertEqual(result["bridge_metrics"]["successful"], 0)
        self.assertEqual(result["bridge_metrics"]["emitted_text_utf8_bytes"]["total"], 3)

    def test_binding_binary_identity_and_transcript_tamper_fail_closed(self):
        for mutation in ("binding", "binary", "identity", "transcript"):
            with self.subTest(mutation=mutation):
                record, event = self.record()
                setup = copy.deepcopy(self.setup)
                if mutation == "binding":
                    record["binding_id"] = "other"
                elif mutation == "binary":
                    setup["binding"]["binary_sha256"] = "a" * 64
                elif mutation == "identity":
                    record["identity"]["project_root"] = "/outside"
                else:
                    record["transcript"][-1]["message"]["result"] = {"content":[{"type":"text","text":"other"}]}
                self.write("setup.json", setup)
                self.write("call.json", record)
                result = self.summarize([event])
                self.assertEqual(result["bridge_metrics"]["invalid_evidence"], 1)
                self.assertIsNone(result["aide_duration_ms"])
                self.assertIsNone(result["bridge_metrics"]["emitted_text_utf8_bytes"]["total"])

    def test_duplicate_events_or_work_ids_do_not_double_count_handler(self):
        record, event = self.record()
        self.write("call.json", record)
        self.assertIsNone(self.summarize([event, event])["aide_duration_ms"])
        self.write("copy.json", record)
        result = self.summarize([event])
        self.assertEqual(result["bridge_metrics"]["invalid_evidence"], 2)
        self.assertIsNone(result["aide_duration_ms"])

    def test_missing_or_wrong_event_fields_never_fall_back_to_dur_ms(self):
        record, original = self.record()
        self.write("call.json", record)
        for key, value in [("work_elapsed_ms",None), ("payload_bytes","99"),
                           ("work_text_sha256","0" * 64), ("observation_stage","host_result"),
                           ("retrieval_id","other"), ("source_references","[]")]:
            with self.subTest(key=key):
                event = copy.deepcopy(original)
                event["dur_ms"] = 99
                event["attrs"][key] = value
                self.assertIsNone(self.summarize([event])["aide_duration_ms"])

    def test_source_hash_and_outside_reference_fail_receipt_verification(self):
        for mutation in ("hash", "path"):
            with self.subTest(mutation=mutation):
                record, event = self.record()
                refs = record["result"]["_meta"]["aide/retrieval"]["references"]
                if mutation == "hash":
                    refs[0]["sha256"] = "0" * 64
                else:
                    refs[0]["file"] = "../outside.ts"
                event["attrs"]["source_references"] = json.dumps(refs)
                self.write("call.json", record)
                result = self.summarize([event])
                self.assertFalse(result["bridge_metrics"]["calls"][0]["receipts"]["retrieval"]["valid"])
                self.assertIsNone(result["aide_duration_ms"])

    def test_malformed_json_and_malformed_nested_receipt_remain_rows(self):
        (self.directory / "bad.json").write_text('{"id":1,"id":2}')
        record, event = self.record()
        record["result"]["_meta"]["aide/retrieval"] = "wrong type"
        self.write("call.json", record)
        result = self.summarize([event])
        self.assertEqual(result["bridge_metrics"]["attempts"], 2)
        self.assertEqual(result["bridge_metrics"]["invalid_evidence"], 2)
        self.assertIsNone(result["aide_duration_ms"])

    def test_nonfinite_timing_unknown_and_no_calls_not_invented_receipt_zero(self):
        result = self.summarize()
        self.assertEqual(result["bridge_metrics"]["attempts"], 0)
        self.assertIsNone(result["aide_duration_ms"])
        record, event = self.record()
        record["timings_ms"]["guard"] = True
        self.write("call.json", record)
        result = self.summarize([event])
        self.assertIsNone(result["bridge_metrics"]["timings_ms"]["guard"]["total"])


if __name__ == "__main__":
    unittest.main()
