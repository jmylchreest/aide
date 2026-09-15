#!/usr/bin/env python3
"""Grade the v4 fixture in a disposable copy; human rubric grades stay separate."""
import argparse
import importlib.util
from pathlib import Path
import shutil
import tempfile

SPEC = importlib.util.spec_from_file_location("v4_runner", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner)


def grade(task, candidate, package=Path(__file__).parent):
    candidate, package = Path(candidate).resolve(), Path(package).resolve()
    runner.verify(package)
    if task not in runner.TASKS or not candidate.is_dir():
        raise ValueError("Unknown task or missing candidate root")
    legacy = runner.inherited("outcomes-v3/grade.py")
    spec = runner.read_json(package / "implement/grader/manifest.json") if task == "implement" else {}
    allowed = spec.get("allowed_edit_paths", [])
    if not isinstance(allowed, list) or any(not isinstance(path, str) for path in allowed):
        raise ValueError("Invalid allowed edit paths")
    with tempfile.TemporaryDirectory(prefix="aide-v4-grade-") as temp:
        baseline = Path(temp) / "baseline"
        runner.copy_template(package, task, baseline)
        if not set(allowed) <= legacy.inventory(baseline).keys():
            raise ValueError("Allowed edit paths must belong to the frozen baseline")
        audit = legacy.integrity(baseline, candidate, set(allowed))
        if task != "implement" and audit["scratch_tests"]:
            audit["unexpected_files"] = sorted(audit["unexpected_files"] + audit["scratch_tests"])
            audit["passed"] = False
        result = {"schema_version": 4, "task": task, "candidate_root": str(candidate),
                  "integrity": audit, "candidate_unchanged": True,
                  "quality_verified": False, "quality": None}
        if not audit["passed"]:
            result.update(quality_verified=True, quality={"passed": False, "immutable_files_preserved": False},
                          grading_mode="not_run_integrity_failed")
            return result
        if task != "implement":
            result["grading_mode"] = "independent_blind_rubric_grade_required"
            return result
        for key in ("visible_checks", "hidden_checks"):
            if type(spec.get(key)) is not int or spec[key] <= 0:
                raise ValueError("Frozen check counts must be positive integers")
        before = legacy.inventory(candidate, exclude_controller=True)
        work = Path(temp) / "candidate"
        shutil.copytree(candidate, work, ignore=lambda directory, names:
                        [".aide"] if Path(directory) == candidate and ".aide" in names else [])
        hidden = work / ".grader"
        hidden.mkdir()
        shutil.copyfile(package / "implement/grader/hidden.test.ts", hidden / "hidden.test.ts")
        visible_files = sorted("./" + p.relative_to(baseline).as_posix()
                               for p in (baseline / "tests").rglob("*.test.ts"))
        if not visible_files:
            raise ValueError("Missing frozen visible tests")
        visible = legacy.run_suite(["rtk", "proxy", "bun", "test", *visible_files], work, spec["visible_checks"])
        checked = legacy.run_suite(["rtk", "proxy", "bun", "test", "./.grader/hidden.test.ts"], work, spec["hidden_checks"])
        unchanged = before == legacy.inventory(candidate, exclude_controller=True)
        functional = {"passed": unchanged and visible["passed"] and checked["passed"],
                      "visible_passed": visible["passed"], "hidden_passed": checked["passed"],
                      "immutable_files_preserved": audit["passed"] and unchanged}
        result.update(grading_mode="disposable_copy", candidate_unchanged=unchanged,
                      checks={"visible": visible, "hidden": checked},
                      functional_quality_verified=True, functional_quality=functional)
        if not functional["passed"]:
            result.update(quality_verified=True, quality=functional)
        else:
            result["review_required"] = "Independent implementation/review-rubric.json assessment"
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("task", choices=runner.TASKS)
    parser.add_argument("root", type=Path)
    parser.add_argument("--package", type=Path, default=Path(__file__).parent)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        root, output = args.root.resolve(), args.output.resolve()
        if output == root or root in output.parents:
            raise ValueError("Grade output must be outside the candidate root")
        with output.open("x", encoding="utf-8") as stream:
            import json
            json.dump(grade(args.task, root, args.package), stream, indent=2, allow_nan=False)
            stream.write("\n")
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.exit(1, f"Offline grading failed: {error}\n")

if __name__ == "__main__":
    main()
