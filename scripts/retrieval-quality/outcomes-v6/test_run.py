import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("v6_runner_test", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


def protocol_fixture():
    value = copy.deepcopy(runner.v5.verify(runner.V5))
    value.update(version=6, trial_order=copy.deepcopy(runner.TRIAL_ORDER),
                 base_package={"manifest_sha256": runner.v4.digest(runner.V5 / "manifest.json")},
                 controller_helpers=runner.controller_hashes())
    return value


class RunnerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="aide-v6-runner-test-")
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.package = self.base / "package"
        self.package.mkdir()
        self.protocol = protocol_fixture()
        self.write_package()

    def write_package(self):
        (self.package / "protocol.json").write_text(json.dumps(self.protocol))
        runner.v4.freeze_manifest(self.package)

    def test_exact_four_cell_plan_verified(self):
        self.assertEqual(runner.verify(self.package)["trial_order"], [
            {"id":"t01", "task":"implement", "treatment":"ordinary", "repetition":1},
            {"id":"t02", "task":"implement", "treatment":"assisted", "repetition":1},
            {"id":"t03", "task":"implement", "treatment":"assisted", "repetition":2},
            {"id":"t04", "task":"implement", "treatment":"ordinary", "repetition":2},
        ])

    def test_refrozen_invalid_plan_and_dependencies_rejected(self):
        original = copy.deepcopy(self.protocol)
        for mutate in (
            lambda p: p["trial_order"].reverse(),
            lambda p: p["trial_order"].pop(),
            lambda p: p["trial_order"][0].update(repetition=True),
            lambda p: p["trial_order"][0].update(task="trace"),
            lambda p: p["dependencies"].update({"collect_codex.py":"0" * 64}),
            lambda p: p.pop("controller_helpers"),
            lambda p: p["controller_helpers"].update({runner.CONTROLLER_HELPERS[0]:"0" * 64}),
            lambda p: p["base_package"].update(manifest_sha256="0" * 64),
            lambda p: p["retrieval_bridge"].update(binary_sha256="0" * 64),
            lambda p: p["treatments"].update(assisted=p["treatments"]["ordinary"]),
        ):
            with self.subTest(mutate=mutate):
                self.protocol = copy.deepcopy(original)
                mutate(self.protocol)
                self.write_package()
                with self.assertRaises(ValueError): runner.verify(self.package)

    def test_unfrozen_file_change_rejected(self):
        (self.package / "extra.txt").write_text("changed")
        with self.assertRaises(ValueError): runner.verify(self.package)

    def test_prompt_diff_is_only_treatment_for_same_paths(self):
        row = {**runner.TRIAL_ORDER[0], "root":"/tmp/root with spaces",
               "bridge_evidence_dir":"/tmp/evidence"}
        plain = runner.prompt_for(self.protocol, row, "/tmp/binary")
        assisted_row = {**row, "treatment":"assisted"}
        assisted = runner.prompt_for(self.protocol, assisted_row, "/tmp/binary")
        neutral = copy.deepcopy(self.protocol)
        neutral["treatments"] = dict.fromkeys(("ordinary", "assisted"), "TREATMENT")
        self.assertEqual(runner.prompt_for(neutral, row, "/tmp/binary"),
                         runner.prompt_for(neutral, assisted_row, "/tmp/binary"))
        self.assertNotEqual(plain, assisted)
        self.assertIn("Run `rtk proxy bun test tests`", plain)
        self.assertIn("code_read_symbol", assisted)
        self.assertNotIn("{{", assisted)
        self.assertEqual(assisted, runner.v5.prompt_for(self.protocol, assisted_row, "/tmp/binary"))

    def test_prepare_only_four_identical_unseeded_roots(self):
        binary = self.base / "binary"
        binary.write_bytes(b"test fixture executable; never executed")
        self.protocol["retrieval_bridge"]["binary_sha256"] = runner.v4.digest(binary)
        destination = self.base / "trials"
        with patch.object(runner, "verify", return_value=self.protocol), patch.object(runner, "BASE", self.package):
            ledger = runner.prepare(destination, binary)
            with self.assertRaises(FileExistsError): runner.prepare(destination, binary)
        self.assertEqual(ledger["schema_version"], 6)
        self.assertEqual({p.name for p in destination.iterdir()}, {"ledger.json", "t01", "t02", "t03", "t04"})
        inventories = []
        for row in ledger["trials"]:
            self.assertEqual(row["status"], "planned")
            self.assertIsNone(row["agent_id"])
            self.assertFalse(Path(row["bridge_evidence_dir"]).exists())
            self.assertFalse((Path(row["root"]) / ".aide").exists())
            self.assertEqual(row["prompt_sha256"], runner.v4.digest(row["prompt_path"]))
            self.assertEqual(Path(row["prompt_path"]).read_text(), runner.prompt_for(self.protocol, row, binary))
            inventories.append(runner.launch.bridge.source_inventory(Path(row["root"])))
        self.assertTrue(all(i == inventories[0] for i in inventories))
        self.assertEqual(runner.v4.read_json(destination / "ledger.json"), ledger)

    def test_bad_binary_and_placeholder_leave_no_trial_state(self):
        binary = self.base / "binary"
        binary.write_bytes(b"fixture")
        destination = self.base / "trials"
        with patch.object(runner, "verify", return_value=self.protocol):
            with self.assertRaises(ValueError): runner.prepare(destination, binary)
            self.assertFalse(destination.exists())
            self.protocol["retrieval_bridge"]["binary_sha256"] = runner.v4.digest(binary)
            self.protocol["treatments"]["assisted"] += " {{UNKNOWN}}"
            with self.assertRaises(ValueError): runner.prepare(destination, binary)
            self.assertFalse(destination.exists())


class LaunchTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="aide-v6-launch-test-")
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.results = self.base / "results"
        self.results.mkdir()
        self.protocol = protocol_fixture()
        self.rows = [{**p, "root":str(self.base / p["id"] / "root"),
                      "bridge_evidence_dir":str(self.base / p["id"] / "bridge"),
                      "prompt_path":str(self.base / p["id"] / "prompt.md"),
                      "status":"planned"} for p in self.protocol["trial_order"]]
        self.row = self.rows[0]
        self.root = Path(self.row["root"])
        runner.v4.copy_template(runner.CORPUS, "implement", self.root)
        self.evidence = Path(self.row["bridge_evidence_dir"])
        self.evidence.mkdir()
        (self.evidence / "setup.json").write_text("{}")
        self.binary = self.base / "binary"
        self.message = runner.prompt_for(self.protocol, self.row, self.binary)
        Path(self.row["prompt_path"]).write_text(self.message)
        self.row["prompt_sha256"] = runner.v4.digest(self.row["prompt_path"])
        (self.base / "protocol.json").write_text(json.dumps(self.protocol))
        self.ledger = {"protocol_sha256":runner.v4.digest(self.base / "protocol.json"),
                       "trials":self.rows,"retrieval_bridge":{"binary":str(self.binary)}}
        self.save_ledger()
        self.binding = {"binary_sha256":self.protocol["retrieval_bridge"]["binary_sha256"],
                        "seed_files":runner.launch.bridge.source_inventory(self.root)}
        self.addCleanup(patch.stopall)
        patch.object(runner,"BASE",self.base).start()
        patch.object(runner,"verify",return_value=self.protocol).start()
        patch.object(runner.launch.bridge,"bound_configuration",
                     return_value=(self.root,self.binary,self.evidence,self.binding)).start()

    def save_ledger(self):
        (self.results / "ledger.json").write_text(json.dumps(self.ledger))

    def test_full_prompt_archived_then_launch_recorded_once(self):
        record = runner.preflight(self.results,"t01")
        self.assertEqual(record["launcher_message"], self.message)
        self.assertEqual((self.results / "t01-launch-message.txt").read_bytes(), self.message.encode())
        runner.launched(self.results,"t01","/root/v6_t01")
        with self.assertRaises(ValueError): runner.launched(self.results,"t01","/root/v6_t01")
        with self.assertRaises(FileExistsError): runner.launched(self.results,"t01","/root/other")

    def test_out_of_order_and_duplicate_preflight_rejected(self):
        with self.assertRaises(ValueError): runner.preflight(self.results,"t02")
        runner.preflight(self.results,"t01")
        with self.assertRaises(ValueError): runner.preflight(self.results,"t01")

    def test_completed_label_without_capture_cannot_unlock_next(self):
        self.rows[0].update(status="completed",agent_id="actor1")
        self.save_ledger()
        with self.assertRaises(FileNotFoundError): runner.preflight(self.results,"t02")

    def test_later_started_cell_rejected(self):
        (self.results / "t04-launch-message.txt").write_text("orphan")
        with self.assertRaises(ValueError): runner.preflight(self.results,"t01")

    def test_source_or_prompt_mutation_rejected(self):
        Path(self.row["prompt_path"]).write_text("Read another instruction file")
        self.row["prompt_sha256"] = runner.v4.digest(self.row["prompt_path"])
        self.save_ledger()
        with self.assertRaises(ValueError): runner.preflight(self.results,"t01")
        Path(self.row["prompt_path"]).write_text(self.message)
        self.row["prompt_sha256"] = runner.v4.digest(self.row["prompt_path"])
        self.save_ledger()
        (self.root / "README.md").write_text("changed")
        with self.assertRaises(ValueError): runner.preflight(self.results,"t01")

    def test_modified_plan_rejected_before_prelaunch(self):
        self.rows[-1]["treatment"] = "assisted"
        self.save_ledger()
        with self.assertRaises(ValueError): runner.preflight(self.results,"t01")
        self.assertFalse((self.results / "t01-prelaunch.json").exists())


if __name__ == "__main__":
    unittest.main()
