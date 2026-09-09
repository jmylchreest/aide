#!/usr/bin/env python3
"""Prepare the frozen cross-file pilot offline; never launch agents or index files."""
import argparse
import copy
import hashlib
import importlib.util
import itertools
import json
from pathlib import Path
import re
import shlex
import shutil

BASE = Path(__file__).resolve().parent
SHARED = BASE.parent
DEPENDENCIES = ("collect_codex.py", "outcomes-v3/run.py", "outcomes-v3/grade.py", "outcomes-v3/bridge.py")
TASKS = ("trace", "impact", "implement")
CONDITIONS = ("ordinary", "assisted")


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def dependency_hashes():
    return {name: digest(SHARED / name) for name in DEPENDENCIES}


def read_json(path):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError(f"Duplicate JSON key: {key}")
            result[key] = value
        return result
    return json.loads(Path(path).read_text(), object_pairs_hook=pairs,
                      parse_constant=lambda value: (_ for _ in ()).throw(ValueError(value)))


def write_json(path, value):
    with Path(path).open("x", encoding="utf-8") as stream:
        json.dump(value, stream, indent=2, ensure_ascii=False, allow_nan=False)
        stream.write("\n")


def inventory(package):
    package = Path(package).resolve()
    files = {}
    for path in sorted(package.rglob("*")):
        if path.is_symlink():
            raise ValueError(f"Package symlinks forbidden: {path}")
        relative = path.relative_to(package)
        if relative == Path("manifest.json") or "__pycache__" in relative.parts or path.suffix == ".pyc":
            continue
        if path.is_file():
            files[relative.as_posix()] = digest(path)
    return files


def freeze_manifest(package):
    package = Path(package).resolve()
    value = {"files": inventory(package)}
    (package / "manifest.json").write_text(json.dumps(value, indent=2) + "\n")
    return value


def verify(package):
    package = Path(package).resolve()
    recorded = read_json(package / "manifest.json").get("files")
    if not recorded or recorded != inventory(package):
        raise ValueError("Package inventory differs from frozen manifest")
    protocol = read_json(package / "protocol.json")
    if protocol.get("version") != 4 or protocol.get("dependencies") != dependency_hashes():
        raise ValueError("Protocol version or pinned inherited helpers differ")
    plan = protocol.get("trial_order", [])
    cells = {(row.get("task"), row.get("treatment"), row.get("repetition")) for row in plan}
    expected = set(itertools.product(TASKS, CONDITIONS, (1, 2)))
    ids = [row.get("id") for row in plan]
    if (len(plan) != 12 or cells != expected or len(set(ids)) != 12
            or any(not isinstance(i, str) or not re.fullmatch(r"t[0-9]{2}", i) for i in ids)
            or any(type(row.get("repetition")) is not int for row in plan)):
        raise ValueError("Protocol must contain all twelve unique task/condition/repetition cells")
    return protocol


def inherited(name):
    path = SHARED / name
    spec = importlib.util.spec_from_file_location("v4_" + path.stem, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def copy_template(package, task, destination):
    ignore = shutil.ignore_patterns("__pycache__", "*.pyc")
    shutil.copytree(package / "common/template", destination, ignore=ignore)
    extra = package / task / "participant"
    if extra.exists():
        for path in extra.rglob("*"):
            if path.is_file() and (destination / path.relative_to(extra)).exists():
                raise ValueError("Task additions must not overwrite shared source")
        shutil.copytree(extra, destination, dirs_exist_ok=True, ignore=ignore)


def validate_template(package, task):
    common = package / "common/template"
    if not common.is_dir():
        raise ValueError("Missing common template")
    extra = package / task / "participant"
    if extra.exists():
        for path in extra.rglob("*"):
            relative = path.relative_to(extra)
            target = common / relative
            if target.exists() and (path.is_file() or target.is_file()):
                raise ValueError("Task additions must not overwrite shared source")


def prepare(package, destination, binary):
    package, destination, binary = Path(package).resolve(), Path(destination).absolute(), Path(binary).resolve()
    if destination.exists():
        raise FileExistsError("Trial destination already exists")
    protocol = verify(package)
    if not binary.is_file() or digest(binary) != protocol["retrieval_bridge"]["binary_sha256"]:
        raise ValueError("Binary differs from frozen digest")
    # Resolve and validate every prompt before creating any trial state.
    prompts = {}
    for row in protocol["trial_order"]:
        validate_template(package, row["task"])
        root = destination / row["id"] / "root"
        values = {"ROOT": root, "BRIDGE": SHARED / "outcomes-v3/bridge.py", "BINARY": binary,
                  "EVIDENCE": destination / row["id"] / "bridge-evidence"}
        text = "\n\n".join((protocol["shared_instructions"], protocol["treatments"][row["treatment"]],
                            (package / row["task"] / "prompt.md").read_text()))
        def replace(match):
            if match[1] not in values:
                raise ValueError("Unknown prompt placeholder")
            return shlex.quote(str(values[match[1]]))
        prompts[row["id"]] = re.sub(r"\{\{([A-Z_]+)\}\}", replace, text).rstrip() + "\n"
    destination.mkdir(parents=True)
    rows = []
    for row in protocol["trial_order"]:
        target = destination / row["id"]
        target.mkdir()
        copy_template(package, row["task"], target / "root")
        prompt = target / "participant-prompt.md"
        prompt.write_text(prompts[row["id"]])
        rows.append({**row, "root": str(target / "root"), "prompt_path": str(prompt),
                     "prompt_sha256": digest(prompt), "status": "planned", "agent_id": None,
                     "runtime_session_id": None, "log_path": None, "scope_verified": None,
                     "trace_reviewed": None, "quality_verified": None, "quality": None,
                     "aide_operations": None, "aide_uptake": None,
                     "bridge_evidence_dir": str(target / "bridge-evidence")})
    result = {"schema_version": 4, "protocol_sha256": digest(package / "protocol.json"),
              "trials": rows, "retrieval_bridge": {"path": str(SHARED / "outcomes-v3/bridge.py"),
              "binary": str(binary), "binary_sha256": digest(binary)}}
    write_json(destination / "ledger.json", result)
    return result


def implementation_quality(package, row):
    """Derive full correctness; never accept a bare externally supplied pass."""
    row = copy.deepcopy(row)
    functional = row.get("functional_quality")
    verified = (row.get("functional_quality_verified") is True
                and isinstance(functional, dict)
                and all(type(functional.get(key)) is bool for key in
                        ("passed", "visible_passed", "hidden_passed", "immutable_files_preserved"))
                and functional["passed"] == all(functional[key] for key in
                        ("visible_passed", "hidden_passed", "immutable_files_preserved")))
    # The integrity grader can establish failure without executing candidate code.
    integrity_failed = (isinstance(row.get("integrity"), dict)
                        and row["integrity"].get("passed") is False)
    row.update(quality_verified=False, quality=None)
    if integrity_failed or (verified and functional["passed"] is False):
        row.update(quality_verified=True, quality={"passed": False,
                   "functional": functional, "integrity_failed": integrity_failed})
        return row
    review = row.get("implementation_review")
    criteria = {item["id"] for item in read_json(package / "implement/review-rubric.json")["criteria"]}
    checks = review.get("checks") if isinstance(review, dict) else None
    evidence = review.get("evidence") if isinstance(review, dict) else None
    complete = (isinstance(review, dict) and isinstance(review.get("reviewer"), str)
                and bool(review["reviewer"].strip()) and review["reviewer"] != row.get("agent_id")
                and review["reviewer"] != row.get("runtime_session_id")
                and isinstance(checks, dict) and set(checks) == criteria
                and all(type(value) is bool for value in checks.values())
                and isinstance(evidence, dict) and set(evidence) == criteria
                and all(isinstance(value, str) and value.strip() for value in evidence.values()))
    if complete and (verified or not all(checks.values())):
        row.update(quality_verified=True, quality={"passed": verified and functional["passed"]
                   and all(checks.values()), "functional": functional, "review_checks": checks})
    return row


def verified_report(package, ledger):
    package = Path(package).resolve()
    protocol = verify(package)
    if ledger.get("protocol_sha256") != digest(package / "protocol.json"):
        raise ValueError("Ledger does not match frozen protocol")
    ledger = copy.deepcopy(ledger)
    implementation_ids = {row["id"] for row in protocol["trial_order"] if row["task"] == "implement"}
    ledger["trials"] = [implementation_quality(package, row) if row.get("id") in implementation_ids
                        else row for row in ledger["trials"]]
    result = inherited("outcomes-v3/run.py").report(protocol, ledger)
    supplied = {row["id"]: row for row in ledger["trials"]}
    for row in result["trials"]:
        if row["task"] == "implement":
            review = supplied.get(row["id"], {})
            for key in ("functional_quality_verified", "functional_quality", "implementation_review"):
                row[key] = review.get(key)
            if not row["quality_verified"]:
                row["unknown_reasons"].append("implementation_grade_or_independent_review_pending")
    result["schema_version"] = 4
    result["measurement_basis"] = protocol["measurement_basis"]
    result["provenance"] = {"protocol_sha256": digest(package / "protocol.json"),
                            "manifest_sha256": digest(package / "manifest.json"),
                            "dependencies": protocol["dependencies"]}
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--package", type=Path, default=BASE)
    commands = parser.add_subparsers(dest="command", required=True)
    prep = commands.add_parser("prepare")
    prep.add_argument("destination", type=Path)
    prep.add_argument("--binary", type=Path, required=True)
    commands.add_parser("verify")
    report = commands.add_parser("report")
    report.add_argument("--ledger", type=Path, required=True)
    report.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        if args.command == "verify":
            verify(args.package)
            print("Frozen package and helper dependencies verified.")
        elif args.command == "prepare":
            value = prepare(args.package, args.destination, args.binary)
            print(f"Prepared {len(value['trials'])} roots; no indexing or participants launched.")
        else:
            write_json(args.output, verified_report(args.package, read_json(args.ledger)))
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.exit(1, f"Offline v4 operation failed: {error}\n")

if __name__ == "__main__":
    main()
