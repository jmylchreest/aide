#!/usr/bin/env python3
"""Grade frozen functional suites in a disposable copy, preserving trial roots."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

EXPECTED_CHECKS = {"debug": (1, 11), "edit": (1, 8)}


def inventory(root, skip_symlinks=False):
    files = {}
    for path in root.rglob("*"):
        if path.is_symlink():
            if skip_symlinks:
                continue
            raise ValueError("Grading inputs must not contain symlinks")
        if path.is_file():
            files[path.relative_to(root).as_posix()] = hashlib.sha256(path.read_bytes()).hexdigest()
    return files


def integrity(template, candidate, allowed):
    original, actual = inventory(template), inventory(candidate, skip_symlinks=True)
    symlinks = sorted(path.relative_to(candidate).as_posix() for path in candidate.rglob("*")
                      if path.is_symlink())
    missing = sorted(original.keys() - actual.keys())
    modified = sorted(path for path in original.keys() & actual.keys()
                      if original[path] != actual[path] and path not in allowed)
    added = sorted(actual.keys() - original.keys())
    scratch = [path for path in added if not path.startswith("src/")
               and path.endswith((".test.ts", ".spec.ts"))]
    unexpected = sorted(set(added) - set(scratch))
    return {"passed": not (missing or modified or unexpected or symlinks),
            "missing": missing, "modified_immutable": modified,
            "symlinks": symlinks,
            "unexpected_files": unexpected, "scratch_tests": scratch,
            "changed_allowed_sources": sorted(path for path in allowed
                if path in original and path in actual and original[path] != actual[path])}


def as_text(value):
    return value.decode("utf-8", errors="replace") if isinstance(value, bytes) else value or ""


def run_suite(command, root, expected_count=None):
    env = os.environ.copy()
    env.update(AIDE_TRIAL_ROOT=str(root), NO_COLOR="1")
    error = None
    try:
        completed = subprocess.run(command, cwd=root, env=env, capture_output=True,
                                   text=True, timeout=120, check=False)
        code, stdout, stderr = completed.returncode, completed.stdout, completed.stderr
    except subprocess.TimeoutExpired as failure:
        code, stdout, stderr, error = None, as_text(failure.stdout), as_text(failure.stderr), "timeout"
    except OSError:
        code, stdout, stderr, error = None, "", "", "runtime_unavailable"
    combined = re.sub(r"\x1b\[[0-9;]*m", "", stdout + "\n" + stderr)
    counts = {}
    for label in ("pass", "fail", "skip", "todo"):
        matches = re.findall(r"^\s*(\d+) " + label + r"\s*$", combined, flags=re.MULTILINE)
        counts[label] = int(matches[0]) if len(matches) == 1 else None
    passed = bool(code == 0 and counts["pass"] is not None and counts["pass"] > 0
                  and counts["fail"] == 0 and counts["skip"] in (None, 0)
                  and counts["todo"] in (None, 0)
                  and (expected_count is None or counts["pass"] == expected_count))
    return {"command": command, "exit_code": code, "stdout": stdout, "stderr": stderr,
            "error": error, "pass_count": counts["pass"], "fail_count": counts["fail"],
            "skip_count": counts["skip"], "todo_count": counts["todo"],
            "expected_count": expected_count, "passed": passed}


def grade(task, candidate, package=Path(__file__).parent):
    if task not in ("debug", "edit"):
        raise ValueError("Navigation requires independent blind rubric grading")
    candidate, package = Path(candidate).resolve(), Path(package).resolve()
    if not candidate.is_dir():
        raise ValueError("Candidate root must be a directory")
    task_root = package / task
    spec_path = task_root / ("grader/manifest.json" if task == "debug" else "provenance.json")
    spec = json.loads(spec_path.read_text())
    allowed = spec.get("allowed_edit_paths")
    if not isinstance(allowed, list) or not allowed or any(not isinstance(path, str) for path in allowed):
        raise ValueError("Frozen task needs explicit allowed_edit_paths")
    template = task_root / "template"
    if not template.is_dir() or not (task_root / "grader/hidden.test.ts").is_file():
        raise ValueError("Missing frozen grading inputs")
    if not set(allowed) <= inventory(template).keys():
        raise ValueError("Allowed edit paths must be frozen template files")
    audit = integrity(template, candidate, set(allowed))
    if not audit["passed"]:
        skipped = {"command": None, "exit_code": None, "stdout": "", "stderr": "",
                   "error": "not_run_integrity_failed", "pass_count": None, "fail_count": None,
                   "skip_count": None, "todo_count": None, "expected_count": None, "passed": False}
        return {"schema_version": 2, "task": task, "candidate_root": str(candidate),
                "grading_mode": "not_run_integrity_failed", "candidate_unchanged": True,
                "integrity": audit, "checks": {"visible": dict(skipped), "hidden": dict(skipped)},
                "quality_verified": True, "quality": {"passed": False, "visible_passed": None,
                    "hidden_passed": None, "immutable_files_preserved": False}}
    before = inventory(candidate)
    with tempfile.TemporaryDirectory(prefix="aide-outcomes-v2-grade-") as temporary:
        work = Path(temporary) / "candidate"
        shutil.copytree(candidate, work)
        # Always execute the provided frozen reproducer; a modified candidate copy
        # still fails the independent integrity check.
        shutil.copyfile(template / "reproduce.test.ts", work / "reproduce.test.ts")
        visible = run_suite(["rtk", "proxy", "bun", "test", "./reproduce.test.ts"], work,
                            spec.get("visible_checks", EXPECTED_CHECKS[task][0]))
        hidden = run_suite(["rtk", "proxy", "bun", "test", str(task_root / "grader/hidden.test.ts")], work,
                           spec.get("hidden_checks", EXPECTED_CHECKS[task][1]))
    unchanged = before == inventory(candidate)
    passed = audit["passed"] and unchanged and visible["passed"] and hidden["passed"]
    return {"schema_version": 2, "task": task, "candidate_root": str(candidate),
            "grading_mode": "disposable_copy", "candidate_unchanged": unchanged,
            "integrity": audit, "checks": {"visible": visible, "hidden": hidden},
            "quality_verified": True, "quality": {"passed": passed,
                "visible_passed": visible["passed"], "hidden_passed": hidden["passed"],
                "immutable_files_preserved": audit["passed"] and unchanged}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("task", choices=("debug", "edit"))
    parser.add_argument("root", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        if args.root.resolve() in args.output.resolve().parents or args.root.resolve() == args.output.resolve():
            raise ValueError("Grading output must be outside the participant root")
        # Exclusive creation rejects previous evidence before running any tests.
        with args.output.open("x", encoding="utf-8") as output:
            json.dump(grade(args.task, args.root), output, indent=2, ensure_ascii=False, allow_nan=False)
            output.write("\n")
    except (OSError, ValueError) as error:
        parser.exit(1, f"Grading failed: {error}\n")


if __name__ == "__main__":
    main()
