#!/usr/bin/env python3
"""Offline preparation and evidence reporting; never starts participants or providers."""

import argparse
import hashlib
import importlib.util
import itertools
import json
from pathlib import Path
import re
import shutil


_SPEC = importlib.util.spec_from_file_location("outcome_capture", Path(__file__).parent.parent / "collect_codex.py")
collector = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(collector)
IDENTIFIERS = ("id", "task", "treatment", "repetition")
RUNTIME_SIGNATURE = ("model", "model_provider", "effort", "cli_version")


def read_json(path):
    return collector._strict_json(Path(path).read_text(encoding="utf-8"))


def write_json(path, value):
    with Path(path).open("x", encoding="utf-8") as stream:
        json.dump(value, stream, indent=2, ensure_ascii=False, allow_nan=False)
        stream.write("\n")


def planned_trials(protocol):
    rows = protocol.get("trial_order") if isinstance(protocol, dict) else None
    if not isinstance(rows, list) or not rows:
        raise ValueError("Protocol needs a nonempty trial_order")
    ids = set()
    cells = set()
    for row in rows:
        if not isinstance(row, dict):
            raise ValueError("Invalid planned trial")
        if any(not isinstance(row.get(k), str) or not re.fullmatch(r"[A-Za-z0-9_-]+", row[k])
               for k in IDENTIFIERS[:3]):
            raise ValueError("Invalid planned trial identifier")
        if type(row.get("repetition")) is not int or row["repetition"] < 1:
            raise ValueError("Invalid repetition")
        cell = (row["task"], row["treatment"], row["repetition"])
        if row["id"] in ids or cell in cells:
            raise ValueError("Duplicate planned trial")
        ids.add(row["id"])
        cells.add(cell)
    return rows


def package_path(package, relative):
    if not isinstance(relative, str) or not relative or Path(relative).is_absolute():
        raise ValueError("Manifest paths must be relative")
    path = package / relative
    if ".." in Path(relative).parts or path.is_symlink() or package not in path.resolve().parents:
        raise ValueError("Unsafe package path")
    return path


def verify_manifest(package):
    manifest = read_json(package / "manifest.json")
    files = manifest.get("files") if isinstance(manifest, dict) else None
    if not isinstance(files, dict) or not files:
        raise ValueError("Manifest needs a files mapping")
    for relative, expected in files.items():
        path = package_path(package, relative)
        if not isinstance(expected, str) or not re.fullmatch(r"[0-9a-f]{64}", expected):
            raise ValueError("Invalid manifest digest")
        if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            raise ValueError(f"Manifest verification failed: {relative}")
    return files


def prepare(package, destination):
    package, destination = Path(package).resolve(), Path(destination).absolute()
    if destination.exists():
        raise FileExistsError("Trial destination already exists")
    files = verify_manifest(package)
    protocol = read_json(package / "protocol.json")
    plan = planned_trials(protocol)
    required = {"protocol.json"}
    prompts = {}
    for trial in plan:
        task = trial["task"]
        template = package / task / "template"
        if not template.is_dir() or template.is_symlink():
            raise ValueError(f"Missing or unsafe template: {task}")
        required.add(f"{task}/prompt.md")
        for path in template.rglob("*"):
            if path.is_symlink():
                raise ValueError("Template symlinks are not allowed")
            if path.is_file():
                required.add(path.relative_to(package).as_posix())
        shared = protocol.get("shared_instructions")
        treatment = protocol.get("treatments", {}).get(trial["treatment"])
        if not isinstance(shared, str) or not isinstance(treatment, str):
            raise ValueError("Missing participant instructions")
        root = destination / trial["id"] / "root"
        prompts[trial["id"]] = "\n\n".join((shared.replace("ROOT", str(root)), treatment,
                                              (package / task / "prompt.md").read_text())).rstrip() + "\n"
    if not required <= files.keys():
        raise ValueError("Manifest does not cover all participant inputs")
    destination.mkdir(parents=True, exist_ok=False)
    trials = []
    for trial in plan:
        target = destination / trial["id"]
        target.mkdir()
        shutil.copytree(package / trial["task"] / "template", target / "root")
        (target / "participant-prompt.md").write_text(prompts[trial["id"]], encoding="utf-8")
        trials.append({**trial, "root": str(target / "root"),
                       "prompt_path": str(target / "participant-prompt.md"),
                       "status": "planned", "agent_id": None, "runtime_session_id": None,
                       "log_path": None, "scope_verified": None, "trace_reviewed": None,
                       "quality_verified": None, "quality": None, "aide_operations": None,
                       "aide_uptake": None})
    result = {"schema_version": 2, "protocol_sha256": files["protocol.json"], "trials": trials}
    write_json(destination / "ledger.json", result)
    return result


def nonnegative(value):
    return value if type(value) is int and value >= 0 else None


def report(protocol, ledger):
    plan = planned_trials(protocol)
    if not isinstance(ledger, dict) or not isinstance(ledger.get("trials"), list):
        raise ValueError("Ledger needs a trials array")
    supplied = {}
    ids = {row["id"] for row in plan}
    for row in ledger["trials"]:
        if (not isinstance(row, dict) or not isinstance(row.get("id"), str)
                or row["id"] not in ids or row["id"] in supplied):
            raise ValueError("Unknown, malformed, or duplicate ledger trial")
        supplied[row["id"]] = row
    result = []
    for planned in plan:
        reviewed = supplied.get(planned["id"], {})
        reasons = []
        capture = None
        if not reviewed:
            reasons.append("missing_trial")
        elif any(reviewed.get(key) != planned[key] for key in IDENTIFIERS):
            reasons.append("planned_identity_mismatch")
        log = reviewed.get("log_path")
        if isinstance(log, str) and log:
            try:
                capture = collector.collect_log(log)
            except (OSError, ValueError, UnicodeError):
                reasons.append("capture_unreadable_or_malformed")
        else:
            reasons.append("missing_capture")
        runtime = capture["runtime"] if capture else None
        identity = reviewed.get("runtime_session_id") or reviewed.get("agent_id")
        identity_matches = bool(capture and isinstance(identity, str) and identity
                                and runtime.get("actor_id") == identity)
        if capture and not identity_matches:
            reasons.append("agent_identity_mismatch")
        if capture:
            reasons.extend(capture["runtime_usage"]["unknown_reasons"])
        for flag in ("scope_verified", "trace_reviewed"):
            if reviewed.get(flag) is not True:
                reasons.append(flag + "_not_confirmed")
        violations = reviewed.get("protocol_violations", [])
        if not isinstance(violations, list) or any(not isinstance(item, str) for item in violations):
            reasons.append("invalid_protocol_review")
            violations = None
        elif violations or reviewed.get("protocol_valid") is False:
            reasons.append("reviewed_protocol_deviation")
        status = reviewed.get("status", "missing")
        if status not in ("planned", "completed", "failed", "cancelled", "missing"):
            status = "unknown"
        complete = bool(capture and capture["completion"]["complete"] and identity_matches)
        protocol_valid = bool(complete and reviewed.get("scope_verified") is True
                              and reviewed.get("trace_reviewed") is True
                              and violations == [] and reviewed.get("protocol_valid") is not False
                              and "planned_identity_mismatch" not in reasons
                              and status in ("completed", "failed"))
        quality = reviewed.get("quality")
        quality = quality if isinstance(quality, dict) else None
        verified = reviewed.get("quality_verified") is True and quality is not None
        # Protocol validity and correctness are separate; failed grades retain all evidence.
        usage = capture["runtime_usage"]["counters"] if identity_matches else None
        row = {**planned, "status": status, "agent_id": reviewed.get("agent_id"),
               "runtime_session_id": reviewed.get("runtime_session_id"), "raw_capture": log,
               "runtime": runtime, "completion": capture["completion"] if identity_matches else None,
               "runtime_usage": usage, "elapsed_ms": capture["completion"]["duration_ms"]
               if complete else None, "tool_calls": len(capture["tool_calls"]) if identity_matches else None,
               "response_count": capture["runtime_usage"]["unique_responses"] if usage is not None else None,
               "aide_operations": nonnegative(reviewed.get("aide_operations")),
               "aide_uptake": reviewed.get("aide_uptake") if type(reviewed.get("aide_uptake")) is bool else None,
               "aide_duration_ms": nonnegative(reviewed.get("aide_duration_ms")),
               "retrieval_output_bytes": nonnegative(reviewed.get("retrieval_output_bytes")),
               "hook_metrics": reviewed.get("hook_metrics") if isinstance(reviewed.get("hook_metrics"), dict) else None,
               "scope_verified": reviewed.get("scope_verified") is True,
               "trace_reviewed": reviewed.get("trace_reviewed") is True,
               "protocol_violations": violations,
               "protocol_valid": protocol_valid, "quality_verified": verified, "quality": quality,
               "unknown_reasons": sorted(set(reasons))}
        result.append(row)
    # A repeated runtime actor cannot establish two fresh-context participants.
    actors = [row["runtime"].get("actor_id") for row in result if row["runtime"]]
    for row in result:
        actor = row["runtime"].get("actor_id") if row["runtime"] else None
        if actor and actors.count(actor) > 1:
            row["protocol_valid"] = False
            row["unknown_reasons"].append("reused_runtime_identity")
    comparisons = []
    for left, right in itertools.combinations(result, 2):
        if left["task"] != right["task"] or left["repetition"] != right["repetition"]:
            continue
        if not all(row["protocol_valid"] and row["quality_verified"] and row["quality"].get("passed") is True
                   and row["status"] == "completed" and row["runtime_usage"] is not None
                   for row in (left, right)):
            continue
        if not all(left["runtime"].get(key) and left["runtime"][key] == right["runtime"].get(key)
                   for key in RUNTIME_SIGNATURE):
            continue
        comparisons.append({"task": left["task"], "repetition": left["repetition"],
                            "left": left["id"], "right": right["id"],
                            "runtime_comparable": True,
                            "runtime_usage_delta_right_minus_left": {
                                key: right["runtime_usage"][key] - left["runtime_usage"][key]
                                for key in collector.COUNTERS},
                            "elapsed_ms_delta_right_minus_left": right["elapsed_ms"] - left["elapsed_ms"]})
    return {"schema_version": 2,
            "measurement_basis": "Runtime-reported counters, not billing or provider savings. Descriptive pilot; no causal savings or aggregate efficiency score.",
            "planned_trials": len(plan), "trials": result, "comparisons": comparisons}


def verified_report(protocol_path, ledger):
    protocol_path = Path(protocol_path).resolve()
    files = verify_manifest(protocol_path.parent)
    digest = hashlib.sha256(protocol_path.read_bytes()).hexdigest()
    if files.get(protocol_path.name) != digest or ledger.get("protocol_sha256") != digest:
        raise ValueError("Reviewed ledger does not match frozen protocol")
    protocol = read_json(protocol_path)
    collector_digest = hashlib.sha256(Path(collector.__file__).read_bytes()).hexdigest()
    if protocol.get("collector_sha256") != collector_digest:
        raise ValueError("Usage collector differs from frozen protocol")
    result = report(protocol, ledger)
    result["provenance"] = {"protocol_sha256": digest, "collector_sha256": collector_digest,
                            "manifest_sha256": hashlib.sha256((protocol_path.parent / "manifest.json").read_bytes()).hexdigest()}
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    prep = commands.add_parser("prepare")
    prep.add_argument("destination", type=Path)
    prep.add_argument("--package", type=Path, default=Path(__file__).parent)
    rep = commands.add_parser("report")
    rep.add_argument("--protocol", type=Path, default=Path(__file__).with_name("protocol.json"))
    rep.add_argument("--ledger", type=Path, required=True)
    rep.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        if args.command == "prepare":
            result = prepare(args.package, args.destination)
            print(f"Prepared {len(result['trials'])} trial roots; no participants launched.")
        else:
            write_json(args.output, verified_report(args.protocol, read_json(args.ledger)))
    except (OSError, ValueError, TypeError) as error:
        parser.exit(1, f"Offline pilot operation failed: {error}\n")


if __name__ == "__main__":
    main()
