import hashlib, importlib.util, json, tempfile, unittest
from pathlib import Path
SPEC=importlib.util.spec_from_file_location("v4run",Path(__file__).with_name("run.py"))
m=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(m)

class PreparationTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
  self.base=Path(self.tmp.name);self.pkg=self.base/"package";self.pkg.mkdir()
  (self.pkg/"common/template").mkdir(parents=True);(self.pkg/"common/template/source.ts").write_text("original")
  for task in ("trace","impact","implement"):
   (self.pkg/task).mkdir();(self.pkg/task/"prompt.md").write_text("Task "+task+" under {{ROOT}}")
  self.binary=self.base/"test binary";self.binary.write_bytes(b"binary")
  self.protocol={"version":4,"shared_instructions":"Root {{ROOT}}", "treatments":{"ordinary":"Shell only", "assisted":"{{BRIDGE}} {{BINARY}} {{EVIDENCE}}"},"dependencies":m.dependency_hashes(),"retrieval_bridge":{"binary_sha256":m.digest(self.binary)},"measurement_basis":"test only","trial_order":[{"id":f"t{i+1:02}","task":task,"treatment":condition,"repetition":rep} for i,(rep,task,condition) in enumerate((r,t,c) for r in (1,2) for t in ("trace","impact","implement") for c in ("ordinary","assisted"))]}
  (self.pkg/"implement/review-rubric.json").write_text(json.dumps({"criteria":[{"id":"types"},{"id":"callers"}]}))
  self.freeze()
 def freeze(self):
  (self.pkg/"protocol.json").write_text(json.dumps(self.protocol));m.freeze_manifest(self.pkg)
 def test_isolation_and_no_overwrite(self):
  dest=self.base/"trials with spaces";ledger=m.prepare(self.pkg,dest,self.binary)
  self.assertEqual(len(ledger["trials"]),12)
  for row in ledger["trials"]:
   root=Path(row["root"]);self.assertEqual((root/"source.ts").read_text(),"original")
   prompt=Path(row["prompt_path"]).read_text();self.assertNotIn("{{",prompt)
   self.assertNotIn("rubric",[p.name for p in root.iterdir()])
  first=Path(ledger["trials"][0]["root"])/"source.ts";first.write_text("changed")
  self.assertEqual((Path(ledger["trials"][1]["root"])/"source.ts").read_text(),"original")
  with self.assertRaises(FileExistsError):m.prepare(self.pkg,dest,self.binary)
 def test_modified_and_unlisted_files_rejected(self):
  for mutation in ("modified","added"):
   with self.subTest(mutation=mutation):
    self.freeze();p=self.pkg/"common/template"/("source.ts" if mutation=="modified" else "extra.ts");p.write_text("new")
    dest=self.base/mutation
    with self.assertRaises(ValueError):m.prepare(self.pkg,dest,self.binary)
    self.assertFalse(dest.exists())
 def test_changed_binary_or_dependency_rejected(self):
  self.binary.write_bytes(b"changed")
  with self.assertRaises(ValueError):m.prepare(self.pkg,self.base/"badbin",self.binary)
  self.binary.write_bytes(b"binary");key=next(iter(self.protocol["dependencies"]));self.protocol["dependencies"][key]="0"*64;self.freeze()
  with self.assertRaises(ValueError):m.prepare(self.pkg,self.base/"baddep",self.binary)
 def test_incomplete_design_rejected(self):
  self.protocol["trial_order"].pop();self.freeze()
  with self.assertRaises(ValueError):m.prepare(self.pkg,self.base/"missing",self.binary)
 def test_symlink_rejected(self):
  (self.pkg/"common/template/link.ts").symlink_to(self.binary)
  with self.assertRaises(ValueError):m.freeze_manifest(self.pkg)
 def test_no_launch_or_index_during_prepare(self):
  ledger=m.prepare(self.pkg,self.base/"unseeded",self.binary)
  self.assertTrue(all(not (Path(row["root"])/".aide").exists() for row in ledger["trials"]))
 def test_task_overlay_cannot_replace_shared_source(self):
  extra=self.pkg/"implement/participant";extra.mkdir()
  (extra/"source.ts").write_text("replacement")
  self.freeze();dest=self.base/"collision"
  with self.assertRaises(ValueError):m.prepare(self.pkg,dest,self.binary)
  self.assertFalse(dest.exists())
 def test_unhashed_caches_never_reach_participants(self):
  (self.pkg/"common/template/unchecked.pyc").write_bytes(b"unchecked")
  cache=self.pkg/"common/template/__pycache__";cache.mkdir();(cache/"unchecked.txt").write_text("unchecked")
  ledger=m.prepare(self.pkg,self.base/"cache-test",self.binary)
  for row in ledger["trials"]:
   self.assertFalse((Path(row["root"])/"unchecked.pyc").exists())
   self.assertFalse((Path(row["root"])/"__pycache__").exists())


 def reviewed_row(self):
  functional={key:True for key in ("passed","visible_passed","hidden_passed","immutable_files_preserved")}
  return {"id":"t05","task":"implement","treatment":"ordinary","repetition":1,
   "agent_id":"participant", "functional_quality_verified":True,"functional_quality":functional,
   "quality_verified":True,"quality":{"passed":True},
   "implementation_review":{"reviewer":"independent-reviewer","checks":{"types":True,"callers":True},
    "evidence":{"types":"source.ts:1 exact exported types", "callers":"source.ts:3 all callers migrated"}}}
 def test_implementation_report_requires_independent_complete_review(self):
  row=self.reviewed_row();row.pop("implementation_review")
  ledger={"protocol_sha256":m.digest(self.pkg/"protocol.json"),"trials":[row]}
  report=m.verified_report(self.pkg,ledger)
  self.assertEqual(len(report["trials"]),12);self.assertEqual(report["comparisons"],[])
  result=next(r for r in report["trials"] if r["id"]=="t05")
  self.assertFalse(result["quality_verified"]);self.assertIsNone(result["quality"])
  self.assertTrue(result["functional_quality"]["passed"])
  self.assertTrue(ledger["trials"][0]["quality"]["passed"])
 def test_implementation_review_pass_fail_and_missing_evidence(self):
  row=self.reviewed_row()
  self.assertTrue(m.implementation_quality(self.pkg,row)["quality"]["passed"])
  row["implementation_review"]["checks"]["callers"]=False
  graded=m.implementation_quality(self.pkg,row)
  self.assertTrue(graded["quality_verified"]);self.assertFalse(graded["quality"]["passed"])
  row["implementation_review"]["evidence"].pop("callers")
  self.assertFalse(m.implementation_quality(self.pkg,row)["quality_verified"])
 def test_implementation_rejects_self_review_and_nonboolean_grade(self):
  row=self.reviewed_row();row["implementation_review"]["reviewer"]="participant"
  self.assertFalse(m.implementation_quality(self.pkg,row)["quality_verified"])
  row=self.reviewed_row();row["implementation_review"]["checks"]["types"]=1
  self.assertFalse(m.implementation_quality(self.pkg,row)["quality_verified"])
 def test_known_implementation_failure_needs_no_review(self):
  row=self.reviewed_row();row.pop("implementation_review")
  row["functional_quality"].update(passed=False,hidden_passed=False)
  result=m.implementation_quality(self.pkg,row)
  self.assertTrue(result["quality_verified"]);self.assertFalse(result["quality"]["passed"])
  row.pop("functional_quality");row["integrity"]={"passed":False}
  self.assertFalse(m.implementation_quality(self.pkg,row)["quality"]["passed"])

if __name__=="__main__":unittest.main()
