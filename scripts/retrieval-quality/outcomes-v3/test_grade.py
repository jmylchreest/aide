import importlib.util
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("pilot_grade", Path(__file__).with_name("grade.py"))
grader = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(grader)


class GradeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.package = self.base / "package"
        task = self.package / "edit"
        (task / "template/src").mkdir(parents=True)
        (task / "grader").mkdir()
        (task / "template/src/file.ts").write_text("seed")
        (task / "template/reproduce.test.ts").write_text("visible")
        (task / "template/README.md").write_text("immutable")
        (task / "grader/hidden.test.ts").write_text("hidden")
        (task / "provenance.json").write_text('{"allowed_edit_paths":["src/file.ts"],"visible_checks":2,"hidden_checks":2}')
        self.candidate = self.base / "candidate"
        shutil.copytree(task / "template", self.candidate)

    def run_result(self, code=0, output=" 2 pass\n 0 fail\n"):
        return subprocess.CompletedProcess([], code, "bun test\n", output)

    def test_allowed_edit_and_scratch_pass_without_writing_candidate(self):
        (self.candidate / "src/file.ts").write_text("fixed")
        (self.candidate / "scratch.test.ts").write_text("independent")
        seen = []
        def execute(command, **kwargs):
            work = Path(kwargs["cwd"])
            self.assertNotEqual(work, self.candidate)
            self.assertEqual(kwargs["env"]["AIDE_TRIAL_ROOT"], str(work))
            self.assertEqual((work / "src/file.ts").read_text(), "fixed")
            (work / "generated.txt").write_text("side effect")
            seen.append(command)
            return self.run_result()
        with patch.object(grader.subprocess, "run", side_effect=execute):
            report = grader.grade("edit", self.candidate, self.package)
        self.assertTrue(report["quality"]["passed"])
        self.assertEqual(len(seen), 2)
        self.assertEqual(report["checks"]["hidden"]["pass_count"], 2)
        self.assertFalse((self.candidate / "generated.txt").exists())

    def test_changed_provided_test_fails_even_if_suites_pass(self):
        (self.candidate / "reproduce.test.ts").write_text("tampered")
        with patch.object(grader.subprocess, "run", return_value=self.run_result()) as execute:
            report = grader.grade("edit", self.candidate, self.package)
        execute.assert_not_called()
        self.assertFalse(report["quality"]["passed"])
        self.assertIn("reproduce.test.ts", report["integrity"]["modified_immutable"])

    def test_missing_source_and_new_config_fail_integrity(self):
        (self.candidate / "src/file.ts").unlink()
        (self.candidate / "bunfig.toml").write_text("preload=[]")
        with patch.object(grader.subprocess, "run", return_value=self.run_result()) as execute:
            report = grader.grade("edit", self.candidate, self.package)
        execute.assert_not_called()
        self.assertFalse(report["quality"]["passed"])
        self.assertEqual(report["integrity"]["missing"], ["src/file.ts"])
        self.assertEqual(report["integrity"]["unexpected_files"], ["bunfig.toml"])

    def test_controller_index_is_excluded_but_similarly_named_paths_are_not(self):
        (self.candidate / ".aide/code").mkdir(parents=True)
        (self.candidate / ".aide/code/index.db").write_bytes(b"controller index")
        with patch.object(grader.subprocess, "run", return_value=self.run_result()):
            report = grader.grade("edit", self.candidate, self.package)
        self.assertTrue(report["quality"]["passed"])
        self.assertEqual(report["integrity"]["excluded_controller_paths"], [".aide"])
        (self.candidate / ".aide-other").write_text("participant file")
        audit = grader.integrity(self.package / "edit/template", self.candidate, {"src/file.ts"})
        self.assertFalse(audit["passed"])
        self.assertEqual(audit["unexpected_files"], [".aide-other"])

    def test_controller_index_is_not_copied_into_grading_sandbox(self):
        (self.candidate / ".aide").mkdir()
        (self.candidate / ".aide/index.db").write_bytes(b"controller index")
        (self.candidate / ".aide/bin").mkdir()
        (self.candidate / ".aide/bin/aide").symlink_to(self.base / "external-frozen-binary")
        def execute(command, **kwargs):
            self.assertFalse((Path(kwargs["cwd"]) / ".aide").exists())
            return self.run_result()
        with patch.object(grader.subprocess, "run", side_effect=execute):
            self.assertTrue(grader.grade("edit", self.candidate, self.package)["quality"]["passed"])

    def test_suite_failure_and_unparseable_counts_not_pass(self):
        for result in (self.run_result(1, " 1 pass\n 1 fail\n"), self.run_result(0, "unknown"),
                       self.run_result(0, " 1 pass\n 0 fail\n")):
            with self.subTest(result=result), patch.object(grader.subprocess, "run", return_value=result):
                report = grader.grade("edit", self.candidate, self.package)
                self.assertFalse(report["quality"]["passed"])
                self.assertEqual(report["checks"]["visible"]["stderr"], result.stderr)

    def test_missing_runtime_and_timeout_report_unknown(self):
        for error in (FileNotFoundError("bun"), subprocess.TimeoutExpired(["bun"], 120, output=b"partial")):
            with self.subTest(error=error), patch.object(grader.subprocess, "run", side_effect=error):
                report = grader.grade("edit", self.candidate, self.package)
                self.assertFalse(report["quality"]["passed"])
                self.assertIsNone(report["checks"]["hidden"]["exit_code"])

    def test_symlink_records_failure_without_execution_and_navigation_rejected(self):
        (self.candidate / "linked.test.ts").symlink_to(self.package / "edit/grader/hidden.test.ts")
        with patch.object(grader.subprocess, "run") as execute:
            report = grader.grade("edit", self.candidate, self.package)
        execute.assert_not_called()
        self.assertFalse(report["quality"]["passed"])
        self.assertEqual(report["integrity"]["symlinks"], ["linked.test.ts"])
        self.assertEqual(report["checks"]["hidden"]["error"], "not_run_integrity_failed")
        with self.assertRaises(ValueError):
            grader.grade("navigation", self.candidate, self.package)

    def test_cli_refuses_output_inside_candidate(self):
        output = self.candidate / "grade.json"
        with patch("sys.argv", ["grade.py", "edit", str(self.candidate), "--output", str(output)]):
            with self.assertRaises(SystemExit):
                grader.main()
        self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
