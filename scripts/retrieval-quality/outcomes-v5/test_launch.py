import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("v5_launch_test", Path(__file__).with_name("launch.py"))
launch = importlib.util.module_from_spec(spec)
spec.loader.exec_module(launch)
runner = launch.runner


class LaunchTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="aide-v5-launch-test-")
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.results = self.base / "results"
        self.results.mkdir()
        self.protocol = runner.v4.read_json(runner.BASE / "protocol.json")
        self.rows = [{**p, "root":str(self.base / p["id"] / "root"),
                      "bridge_evidence_dir":str(self.base / p["id"] / "bridge"),
                      "prompt_path":str(self.base / p["id"] / "prompt.md"),
                      "status":"planned"} for p in self.protocol["trial_order"]]
        self.row = self.rows[0]
        self.root = Path(self.row["root"])
        runner.v4.copy_template(runner.CORPUS, "trace", self.root)
        self.evidence = Path(self.row["bridge_evidence_dir"])
        self.evidence.mkdir()
        (self.evidence / "setup.json").write_text("{}")
        self.binary = Path("/tmp/aide-retrieval-v4")
        self.message = runner.prompt_for(self.protocol, self.row, self.binary)
        Path(self.row["prompt_path"]).write_text(self.message)
        self.row["prompt_sha256"] = runner.v4.digest(self.row["prompt_path"])
        self.ledger = {"protocol_sha256":runner.v4.digest(runner.BASE / "protocol.json"),
                       "trials":self.rows,"retrieval_bridge":{"binary":str(self.binary)}}
        self.save_ledger()
        self.binding = {"binary_sha256":self.protocol["retrieval_bridge"]["binary_sha256"],
                        "seed_files":launch.bridge.source_inventory(self.root)}
        self.addCleanup(patch.stopall)
        patch.object(runner,"verify",return_value=self.protocol).start()
        patch.object(launch.bridge,"bound_configuration",
                     return_value=(self.root,self.binary,self.evidence,self.binding)).start()

    def save_ledger(self):
        (self.results / "ledger.json").write_text(json.dumps(self.ledger))

    def test_full_prompt_archived_before_launch(self):
        record = launch.preflight(self.results,"t01")
        self.assertEqual(record["launcher_message"],self.message)
        self.assertEqual((self.results / "t01-launch-message.txt").read_bytes(),self.message.encode())
        self.assertTrue(self.message.startswith("Every shell command string"))
        self.assertIn("Trace the current host model-usage path",self.message)
        launch.launched(self.results,"t01","/root/v5_t01")
        recorded = runner.v4.read_json(self.results / "t01-launch.json")
        self.assertEqual(recorded["message_sha256"],self.row["prompt_sha256"])

    def test_pointer_only_message_rejected(self):
        with self.assertRaises(ValueError):
            launch.validate_message("Read " + self.row["prompt_path"],self.message)

    def test_modified_prompt_rejected_even_with_updated_ledger_hash(self):
        Path(self.row["prompt_path"]).write_text("Read instructions elsewhere")
        self.row["prompt_sha256"] = runner.v4.digest(self.row["prompt_path"])
        self.save_ledger()
        with self.assertRaises(ValueError): launch.preflight(self.results,"t01")

    def test_missing_prompt_rejected(self):
        Path(self.row["prompt_path"]).unlink()
        with self.assertRaises(FileNotFoundError): launch.preflight(self.results,"t01")

    def test_modified_initial_source_rejected(self):
        (self.root / "README.md").write_text("changed")
        with self.assertRaises(ValueError): launch.preflight(self.results,"t01")

    def test_wrong_seed_inventory_rejected(self):
        self.binding["seed_files"] = {}
        with self.assertRaises(ValueError): launch.preflight(self.results,"t01")

    def test_wrong_binary_binding_rejected(self):
        self.binding["binary_sha256"] = "0" * 64
        with self.assertRaises(ValueError): launch.preflight(self.results,"t01")

    def test_prior_bridge_call_rejected(self):
        (self.evidence / "call-previous.json").write_text("{}")
        with self.assertRaises(ValueError): launch.preflight(self.results,"t01")

    def test_out_of_order_and_duplicate_preflight_rejected(self):
        with self.assertRaises(ValueError): launch.preflight(self.results,"t02")
        launch.preflight(self.results,"t01")
        with self.assertRaises(ValueError): launch.preflight(self.results,"t01")

    def test_completed_label_without_capture_cannot_unlock_next(self):
        self.rows[0].update(status="completed",agent_id="actor1")
        self.save_ledger()
        with self.assertRaises(FileNotFoundError): launch.preflight(self.results,"t02")

    def test_later_active_cell_rejected(self):
        self.rows[-1]["status"] = "completed"
        self.save_ledger()
        with self.assertRaises(ValueError): launch.preflight(self.results,"t01")

    def test_orphan_launch_message_rejected_before_writing_prelaunch(self):
        (self.results / "t01-launch-message.txt").write_text(self.message)
        with self.assertRaises(ValueError): launch.preflight(self.results,"t01")
        self.assertFalse((self.results / "t01-prelaunch.json").exists())

    def test_implementation_example_and_treatment_are_preserved(self):
        row = next(r for r in self.rows if r["task"]=="implement" and r["treatment"]=="assisted")
        message = runner.prompt_for(self.protocol,row,self.binary)
        self.assertIn("Run `rtk proxy bun test tests`",message)
        for tool in ("code_search","code_references","code_symbols","code_outline","code_read_symbol"):
            self.assertIn(tool,message)
        self.assertNotIn("{{",message)

    def test_fractional_bridge_metric_survives_inherited_report(self):
        inherited = {"trials":[{"bridge_duration_ms":None,"bridge_metrics":{"timings_ms":{"total":{"total":1.25}}}}],"provenance":{}}
        with patch.object(runner.v4,"verified_report",return_value=copy.deepcopy(inherited)):
            report = runner.verified_report(runner.BASE,self.ledger)
        self.assertEqual(report["trials"][0]["bridge_metrics"]["timings_ms"]["total"]["total"],1.25)
        self.assertIsNone(report["trials"][0]["bridge_duration_ms"])


if __name__ == "__main__":
    unittest.main()
