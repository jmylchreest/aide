#!/usr/bin/env python3
"""Prepare and preflight a frozen four-cell guidance comparison; never launch actors."""
import argparse
import importlib.util
from pathlib import Path
from types import SimpleNamespace

BASE = Path(__file__).resolve().parent
CORPUS = BASE.parent / "outcomes-v4"
V5 = BASE.parent / "outcomes-v5"
spec = importlib.util.spec_from_file_location("v6_base", V5 / "run.py")
v5 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(v5)
v4 = v5.v4
launch = v4.inherited("outcomes-v5/launch.py")
TRIAL_ORDER = [
    {"id": "t01", "task": "implement", "treatment": "ordinary", "repetition": 1},
    {"id": "t02", "task": "implement", "treatment": "assisted", "repetition": 1},
    {"id": "t03", "task": "implement", "treatment": "assisted", "repetition": 2},
    {"id": "t04", "task": "implement", "treatment": "ordinary", "repetition": 2},
]
CONTROLLER_HELPERS = (
    "results/2026-09-09-outcomes-v6/capture_trial.py",
    "results/2026-09-09-outcomes-v6/snapshot_candidate.py",
)


def controller_hashes():
    return {name: v4.digest(BASE.parent / name) for name in CONTROLLER_HELPERS}


def verify(package=BASE):
    package = Path(package).resolve()
    inherited = v5.verify(V5)
    recorded = v4.read_json(package / "manifest.json").get("files")
    if not recorded or recorded != v4.inventory(package):
        raise ValueError("V6 package differs from frozen manifest")
    protocol = v4.read_json(package / "protocol.json")
    treatments = protocol.get("treatments")
    if (protocol.get("version") != 6
            or protocol.get("base_package", {}).get("manifest_sha256") != v4.digest(V5 / "manifest.json")
            or protocol.get("dependencies") != inherited["dependencies"]
            or protocol.get("controller_helpers") != controller_hashes()
            or protocol.get("trial_order") != TRIAL_ORDER
            or any(type(row.get("repetition")) is not int for row in protocol["trial_order"])
            or protocol.get("retrieval_bridge", {}).get("binary_sha256") != inherited["retrieval_bridge"]["binary_sha256"]
            or not isinstance(treatments, dict) or set(treatments) != {"ordinary", "assisted"}
            or any(not isinstance(text, str) or not text.strip() for text in treatments.values())
            or treatments["ordinary"] == treatments["assisted"]):
        raise ValueError("V6 source, four-cell plan, treatment or inherited dependency mismatch")
    return protocol


def prompt_for(protocol, row, binary):
    return v5.prompt_for(protocol, row, binary)


def prepare(destination, binary):
    protocol = verify()
    destination, binary = Path(destination).absolute(), Path(binary).resolve()
    if destination.exists():
        raise FileExistsError("Trial destination already exists")
    if not binary.is_file() or v4.digest(binary) != protocol["retrieval_bridge"]["binary_sha256"]:
        raise ValueError("Binary differs from frozen digest")
    # Validate all templates and resolve every prompt before creating trial state.
    rows, prompts = [], {}
    for planned in protocol["trial_order"]:
        v4.validate_template(CORPUS, planned["task"])
        target = destination / planned["id"]
        row = {**planned, "root": str(target / "root"),
               "prompt_path": str(target / "participant-prompt.md"),
               "bridge_evidence_dir": str(target / "bridge-evidence"),
               "status": "planned", "agent_id": None, "runtime_session_id": None,
               "log_path": None, "scope_verified": None, "trace_reviewed": None,
               "quality_verified": None, "quality": None, "aide_operations": None,
               "aide_uptake": None}
        prompts[row["id"]] = prompt_for(protocol, row, binary)
        rows.append(row)
    destination.mkdir(parents=True)
    for row in rows:
        v4.copy_template(CORPUS, row["task"], Path(row["root"]))
        prompt = Path(row["prompt_path"])
        prompt.write_text(prompts[row["id"]], encoding="utf-8", newline="")
        row["prompt_sha256"] = v4.digest(prompt)
    ledger = {"schema_version": 6, "protocol_sha256": v4.digest(BASE / "protocol.json"),
              "trials": rows, "retrieval_bridge": {
                  "path": str(BASE.parent / "outcomes-v3/bridge.py"),
                  "binary": str(binary), "binary_sha256": v4.digest(binary)}}
    v4.write_json(destination / "ledger.json", ledger)
    return ledger


def _launcher():
    # Rebind only the runner. Frozen launch identity, ordering, capture and bridge
    # validation stay unchanged. Explicit binding also supports importlib callers.
    launch.runner = SimpleNamespace(BASE=BASE, CORPUS=CORPUS, v4=v4,
                                    verify=verify, prompt_for=prompt_for)
    return launch


def preflight(results, tid):
    return _launcher().preflight(results, tid)


def launched(results, tid, actor):
    return _launcher().launched(results, tid, actor)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("verify")
    prep = commands.add_parser("prepare")
    prep.add_argument("destination", type=Path)
    prep.add_argument("--binary", type=Path, required=True)
    for name in ("preflight", "launched"):
        command = commands.add_parser(name)
        command.add_argument("results", type=Path)
        command.add_argument("trial")
        if name == "launched":
            command.add_argument("--actor", required=True)
    args = parser.parse_args()
    try:
        if args.command == "verify":
            verify()
            print("V6 guidance package, frozen v5/v4 corpus and helpers verified.")
        elif args.command == "prepare":
            ledger = prepare(args.destination, args.binary)
            print(f"Prepared {len(ledger['trials'])} roots and prompts; no indexing or participants launched.")
        elif args.command == "preflight":
            print(preflight(args.results, args.trial)["launcher_message"], end="")
        else:
            launched(args.results, args.trial, args.actor)
            print("Launch recorded")
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.exit(1, f"Offline v6 operation failed: {error}\n")


if __name__ == "__main__":
    main()
