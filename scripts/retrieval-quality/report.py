#!/usr/bin/env python3
"""Offline pilot grading; never runs agents or infers token savings.

grade reads a JSON array of trial records. Each record has id/task/treatment,
answers/evidence objects, source_validity/trace_validity (valid, invalid or
unverified), and optional evidence_review mapping answer keys to supported,
unsupported or unverified. Review is a grader's separate source citation check;
this script cannot establish that a citation supports an answer.

Optional measurements: observed_text_bytes needs text_measurement_verified=true;
provider_usage needs provider_usage_verified=true. Otherwise they remain unknown.
wall_time_seconds is optional. failure retains an infrastructure failure reason.
Extra metadata is preserved in original, including runtime identity and trace.
"""

import argparse
from collections import Counter
import copy
import hashlib
import json
import math
from pathlib import Path


def exact_json(actual, expected):
    """Match JSON values recursively without Python's bool/int equivalence."""
    if actual is None or type(actual) is not type(expected):
        return False
    if isinstance(expected, dict):
        return actual.keys() == expected.keys() and all(exact_json(actual[k], v) for k, v in expected.items())
    if isinstance(expected, list):
        return len(actual) == len(expected) and all(exact_json(a, b) for a, b in zip(actual, expected))
    return actual == expected


def _nonnegative_number(value, integer=False):
    if type(value) is int:
        return value >= 0
    return not integer and type(value) is float and math.isfinite(value) and value >= 0


def build_report(records, tasks, grading):
    """Return a report without modifying inputs. Missing evidence is unverified."""
    if not isinstance(records, list):
        raise ValueError("trial input must be a JSON array")
    expected = {r["id"]: (r["task"], r["treatment"]) for r in tasks["run_order"]}
    counts = Counter(r.get("id") for r in records if isinstance(r, dict) and isinstance(r.get("id"), str))
    duplicates = sorted(key for key, count in counts.items() if count > 1)
    known_tasks = {task["id"] for task in tasks["tasks"]}
    present = set()
    rows = []
    for original in records:
        trial = original if isinstance(original, dict) else {}
        issues = [] if isinstance(original, dict) else ["record must be an object"]
        identity = trial.get("id")
        task = trial.get("task")
        treatment = trial.get("treatment")
        known_task = isinstance(task, str) and task in known_tasks
        known_treatment = isinstance(treatment, str) and treatment in tasks["treatments"]
        if not known_task:
            issues.append("unknown task")
        if not known_treatment:
            issues.append("unknown treatment")
        if not isinstance(identity, str) or identity not in expected:
            issues.append("unknown trial id")
        elif expected[identity] != (task, treatment):
            issues.append("trial identity does not match frozen run order")
        else:
            present.add(identity)
        if isinstance(identity, str) and identity in duplicates:
            issues.append("duplicate trial id")
        validity = {}
        for key in ("source_validity", "trace_validity"):
            value = trial.get(key, "unverified")
            validity[key] = value
            if value not in ("valid", "invalid", "unverified"):
                issues.append(f"invalid {key}")
            elif value == "invalid":
                issues.append(f"{key} is invalid")
        objects = {}
        for key in ("answers", "evidence", "evidence_review"):
            value = trial.get(key, {})
            if not isinstance(value, dict):
                issues.append(f"{key} must be an object")
                value = {}
            objects[key] = value
        answers, evidence, reviews = (objects[k] for k in ("answers", "evidence", "evidence_review"))
        oracle = grading["answers"].get(task, {}) if known_task else {}
        if set(answers) - set(oracle):
            issues.append("unexpected answer keys")
        if set(reviews) - set(oracle):
            issues.append("unexpected evidence review keys")
        fields = {}
        for key, expected_value in oracle.items():
            review = reviews.get(key, "unverified")
            if review not in ("supported", "unsupported", "unverified"):
                issues.append(f"invalid evidence review: {key}")
                review = "unverified"
            # A review with no citation does not establish evidence validity.
            if not evidence.get(key):
                review = "unverified"
            fields[key] = {"correct": key in answers and exact_json(answers[key], expected_value),
                           "evidence_review": review}
        correct = bool(fields) and all(field["correct"] for field in fields.values())
        evidence_verified = bool(fields) and all(field["evidence_review"] == "supported" for field in fields.values())
        unsupported = any(field["evidence_review"] == "unsupported" for field in fields.values())
        measurement = trial.get("observed_text_bytes")
        usage = trial.get("provider_usage")
        elapsed = trial.get("wall_time_seconds")
        if measurement is not None and not _nonnegative_number(measurement, integer=True):
            issues.append("observed_text_bytes must be a nonnegative integer or null")
            measurement = None
        if usage is not None and (not isinstance(usage, dict) or any(
                value is not None and not _nonnegative_number(value, integer=True) for value in usage.values())):
            issues.append("provider_usage must map usage names to nonnegative integers or null")
            usage = None
        if elapsed is not None and not _nonnegative_number(elapsed):
            issues.append("wall_time_seconds must be a finite nonnegative number or null")
            elapsed = None
        for key in ("text_measurement_verified", "provider_usage_verified"):
            if key in trial and type(trial[key]) is not bool:
                issues.append(f"{key} must be boolean")
        if trial.get("text_measurement_verified") is not True:
            measurement = None
        if trial.get("provider_usage_verified") is not True:
            usage = None
        failure = trial.get("failure")
        if failure is not None and (not isinstance(failure, str) or not failure.strip()):
            issues.append("failure must be a nonempty reason or null")
        if issues:
            outcome = "invalid"
        elif failure:
            outcome = "infrastructure_failure"
        elif any(value != "valid" for value in validity.values()):
            outcome = "unverified"
        elif not correct or unsupported:
            outcome = "incorrect"
        elif not evidence_verified:
            outcome = "unverified"
        else:
            outcome = "pass"
        rows.append({"id": identity, "task": task, "treatment": treatment,
                     "outcome": outcome, "issues": issues, "failure": failure,
                     "automated_correct": correct, "correct_fields": sum(f["correct"] for f in fields.values()),
                     "required_fields": len(fields), "fields": fields,
                     "task_pass": True if outcome == "pass" else False if outcome == "incorrect" else None,
                     "observed_text_bytes": measurement, "provider_usage": copy.deepcopy(usage),
                     "wall_time_seconds": elapsed, "original": copy.deepcopy(original)})
    missing = [identity for identity in expected if identity not in present]
    return {"schema": 1, "kind": "retrieval_quality_pilot_report",
            "limitations": ["Exact-value grading is separate from citation review.",
                            "Measurement verification flags are grader attestations, not independently validated here.",
                            "No inferred avoided calls, provider savings, aggregate speedup or statistical confidence."],
            "expected_trials": len(expected), "recorded_trials": len(rows),
            "missing_ids": missing, "duplicate_ids": duplicates,
            "complete": not missing and not duplicates and len(rows) == len(expected),
            "outcomes": dict(Counter(row["outcome"] for row in rows)), "trials": rows}


def verify_sources(root, manifest):
    """Check pinned bytes and SHA256 independently of trial attestations."""
    root = Path(root).resolve()
    rows = []
    for entry in manifest["files"]:
        row = {"file": entry["file"], "valid": False}
        try:
            path = (root / entry["file"]).resolve()
            path.relative_to(root)
            data = path.read_bytes()
            row.update(bytes=len(data), sha256=hashlib.sha256(data).hexdigest())
            row["valid"] = row["bytes"] == entry["bytes"] and row["sha256"] == entry["sha256"]
        except (OSError, ValueError) as error:
            row["error"] = str(error)
        rows.append(row)
    return {"schema": 1, "kind": "pinned_source_verification",
            "source_revision": manifest.get("source_revision"),
            "valid": bool(rows) and all(row["valid"] for row in rows), "files": rows}


def write_report(path, result):
    """Create explicitly named output exclusively; never overwrite input/output."""
    text = json.dumps(result, indent=2, ensure_ascii=False, allow_nan=False) + "\n"
    with Path(path).open("x", encoding="utf-8") as stream:
        stream.write(text)


def _read_json(path):
    def reject_constant(value):
        raise ValueError(f"non-JSON numeric constant: {value}")
    def reject_duplicate_keys(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON object key: {key}")
            result[key] = value
        return result
    with Path(path).open(encoding="utf-8") as stream:
        return json.load(stream, parse_constant=reject_constant, object_pairs_hook=reject_duplicate_keys)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    subcommands = parser.add_subparsers(dest="command", required=True)
    grade = subcommands.add_parser("grade")
    grade.add_argument("--input", type=Path, required=True)
    grade.add_argument("--output", type=Path, required=True)
    verify = subcommands.add_parser("verify-sources")
    verify.add_argument("--root", type=Path, required=True)
    verify.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    directory = Path(__file__).resolve().parent
    try:
        if args.command == "grade":
            result = build_report(_read_json(args.input), _read_json(directory / "tasks.json"),
                                  _read_json(directory / "grading.json"))
        else:
            result = verify_sources(args.root, _read_json(directory / "sources.json"))
        write_report(args.output, result)
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.exit(2, f"error: {error}\n")
    if args.command == "verify-sources" and not result["valid"]:
        parser.exit(1, "Pinned source verification failed; inspect the output report.\n")


if __name__ == "__main__":
    main()
