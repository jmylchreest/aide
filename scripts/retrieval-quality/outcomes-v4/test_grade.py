import importlib.util, json, subprocess, unittest
from pathlib import Path
from unittest.mock import patch
import test_run as fixture
SPEC=importlib.util.spec_from_file_location("v4grade",Path(__file__).with_name("grade.py"))
g=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(g)

class GradingTests(unittest.TestCase):
 freeze=fixture.PreparationTests.freeze
 def setUp(self):
  fixture.PreparationTests.setUp(self)
  grader=self.pkg/"implement/grader";grader.mkdir()
  (grader/"hidden.test.ts").write_text("hidden assertion")
  (grader/"manifest.json").write_text(json.dumps({"allowed_edit_paths":["source.ts"],"visible_checks":1,"hidden_checks":1}))
  extra=self.pkg/"implement/participant";extra.mkdir();(extra/"visible.txt").write_text("immutable")
  (extra/"tests").mkdir();(extra/"tests/visible.test.ts").write_text("provided")
  self.freeze();self.candidate=self.base/"candidate"
  g.runner.copy_template(self.pkg,"implement",self.candidate)
 def test_grades_in_copy_and_preserves_candidate(self):
  (self.candidate/"source.ts").write_text("change")
  controller=self.candidate/".aide";controller.mkdir();(controller/"state").write_text("private")
  def execute(command,**kwargs):
   work=Path(kwargs["cwd"])
   self.assertNotEqual(work,self.candidate)
   self.assertFalse((work/".aide").exists())
   if '.grader' in command[-1]:self.assertEqual((work/".grader/hidden.test.ts").read_text(),"hidden assertion")
   return subprocess.CompletedProcess(command,0,"1 pass\n0 fail\n","")
  with patch("subprocess.run",side_effect=execute):result=g.grade("implement",self.candidate,self.pkg)
  self.assertTrue(result["functional_quality"]["passed"])
  self.assertFalse(result["quality_verified"])
  self.assertIsNone(result["quality"])
  self.assertEqual((self.candidate/"source.ts").read_text(),"change")
  self.assertFalse((self.candidate/".grader").exists())
  self.assertEqual((controller/"state").read_text(),"private")
 def test_immutable_change_fails_without_running_checks(self):
  (self.candidate/"visible.txt").write_text("changed")
  with patch("subprocess.run",side_effect=AssertionError("must not run")):result=g.grade("implement",self.candidate,self.pkg)
  self.assertFalse(result["quality"]["passed"])
  self.assertIn("visible.txt",result["integrity"]["modified_immutable"])
 def test_read_only_quality_stays_pending(self):
  self.candidate=self.base/"trace-root";g.runner.copy_template(self.pkg,"trace",self.candidate)
  result=g.grade("trace",self.candidate,self.pkg)
  self.assertFalse(result["quality_verified"])
  self.assertIsNone(result["quality"])
 def test_read_only_scratch_test_is_a_violation(self):
  root=self.base/"read-only";g.runner.copy_template(self.pkg,"impact",root)
  (root/"new.test.ts").write_text("unexpected")
  result=g.grade("impact",root,self.pkg)
  self.assertFalse(result["integrity"]["passed"])
 def test_added_scratch_tests_do_not_change_frozen_visible_count(self):
  (self.candidate/"tests/extra.test.ts").write_text("extra passing test")
  calls=[]
  def execute(command,**kwargs):
   calls.append(command)
   return subprocess.CompletedProcess(command,0,"1 pass\n0 fail\n","")
  with patch("subprocess.run",side_effect=execute):result=g.grade("implement",self.candidate,self.pkg)
  self.assertTrue(result["functional_quality"]["passed"])
  self.assertIn("./tests/visible.test.ts",calls[0])
  self.assertNotIn("./tests/extra.test.ts",calls[0])

if __name__=="__main__":unittest.main()
