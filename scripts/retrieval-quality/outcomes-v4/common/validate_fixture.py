#!/usr/bin/env python3
"""Offline fixture-only integrity and reference preflight; never runs participants."""
from pathlib import Path
import hashlib
import json
import re
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
TEST_TIMEOUT_SECONDS = 30


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def summary_counts(output):
    counts = {}
    for category in ("pass", "fail", "skip", "todo"):
        matches = re.findall(rf"(?m)^[ \t]*(\d+) {category}[ \t]*$", output)
        valid_summary = len(matches) == 1 if category in ("pass", "fail") else len(matches) <= 1
        require(valid_summary,
                f"Missing or ambiguous {category} summary:\n{output}")
        counts[category] = int(matches[0]) if matches else 0
    require(not re.search(r"(?m)^\((?:skip|todo)\)", output), f"Skipped test detected:\n{output}")
    return counts


def validate():
    provenance = json.loads((ROOT / "common/provenance.json").read_text())
    template = ROOT / "common/template"
    for item in provenance["files"]:
        content = (template / item["path"]).read_bytes()
        require(hashlib.sha256(content).hexdigest() == item["sha256"],
                f"Source hash mismatch: {item['path']}")
    known = {item["path"] for item in provenance["files"]} | {"README.md"}
    actual = {str(p.relative_to(template)) for p in template.rglob("*") if p.is_file()}
    require(known == actual, json.dumps({"missing": sorted(known - actual), "extra": sorted(actual - known)}))
    results = {"source_files_verified": len(provenance["files"]), "participant_model_runs": 0}
    for variant in ("seed", "reference"):
        with tempfile.TemporaryDirectory(prefix="aide-v4-fixture-") as temporary:
            trial = Path(temporary)
            shutil.copytree(template, trial, dirs_exist_ok=True)
            shutil.copytree(ROOT / "implement/participant", trial, dirs_exist_ok=True)
            if variant == "reference":
                shutil.copytree(ROOT / "implement/reference/files", trial, dirs_exist_ok=True)
            (trial / ".grader").mkdir()
            shutil.copy2(ROOT / "implement/grader/hidden.test.ts", trial / ".grader/hidden.test.ts")
            results[variant] = {}
            for suite, target in (("visible", "./tests"), ("hidden", "./.grader/hidden.test.ts")):
                run = subprocess.run(["rtk", "proxy", "bun", "test", target], cwd=trial,
                                     text=True, capture_output=True, timeout=TEST_TIMEOUT_SECONDS)
                output = run.stdout + run.stderr
                expected = 1 if variant == "seed" and suite == "hidden" else 0
                require(run.returncode == expected, f"Unexpected {variant}/{suite} exit {run.returncode}:\n{output}")
                passed, failed = (7, 13) if expected else ((4, 0) if suite == "visible" else (20, 0))
                expected_counts = {"pass": passed, "fail": failed, "skip": 0, "todo": 0}
                counts = summary_counts(output)
                require(counts == expected_counts,
                        f"Unexpected {variant}/{suite} counts {counts}, wanted {expected_counts}:\n{output}")
                results[variant][suite] = {"exit_code": run.returncode, "counts": counts}
    print(json.dumps(results, indent=2))


if __name__ == "__main__":
    validate()
