#!/usr/bin/env python3
"""Frozen full-prompt rerun; reuse the unchanged v4 corpus and quality gates."""
import argparse
import copy
import importlib.util
import json
from pathlib import Path
import re
import shlex

BASE = Path(__file__).resolve().parent
CORPUS = BASE.parent / "outcomes-v4"
spec = importlib.util.spec_from_file_location("v5_base", CORPUS / "run.py")
v4 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(v4)


def verify(package=BASE):
    package = Path(package).resolve()
    base = v4.verify(CORPUS)
    if v4.read_json(package / "manifest.json").get("files") != v4.inventory(package):
        raise ValueError("V5 package differs from frozen manifest")
    protocol = v4.read_json(package / "protocol.json")
    if (protocol.get("version") != 5
            or protocol.get("base_package", {}).get("manifest_sha256") != v4.digest(CORPUS / "manifest.json")
            or protocol.get("dependencies") != base["dependencies"]
            or protocol.get("trial_order") != base["trial_order"]
            or protocol.get("treatments") != base["treatments"]
            or protocol.get("retrieval_bridge") != base["retrieval_bridge"]):
        raise ValueError("V5 source, trial plan, treatment or inherited dependency mismatch")
    return protocol


def prompt_for(protocol, row, binary):
    values = {"ROOT": row["root"], "BRIDGE": BASE.parent / "outcomes-v3/bridge.py",
              "BINARY": binary, "EVIDENCE": row["bridge_evidence_dir"]}
    task = (CORPUS / row["task"] / "prompt.md").read_text()
    # Only the executable spelling changes; the same visible suite is requested.
    if row["task"] == "implement":
        task = task.replace("Run `bun test tests`", "Run `rtk proxy bun test tests`")
    text = "\n\n".join((protocol["shared_instructions"], protocol["treatments"][row["treatment"]], task))
    def replace(match):
        if match[1] not in values:
            raise ValueError("Unknown prompt placeholder")
        return shlex.quote(str(values[match[1]]))
    return re.sub(r"\{\{([A-Z_]+)\}\}", replace, text).rstrip() + "\n"


def prepare(destination, binary):
    protocol = verify()
    ledger = v4.prepare(CORPUS, destination, binary)
    ledger.update(schema_version=5, protocol_sha256=v4.digest(BASE / "protocol.json"))
    for row in ledger["trials"]:
        prompt = Path(row["prompt_path"])
        prompt.write_text(prompt_for(protocol, row, ledger["retrieval_bridge"]["binary"]), encoding="utf-8")
        row["prompt_sha256"] = v4.digest(prompt)
    (Path(destination) / "ledger.json").write_text(json.dumps(ledger, indent=2) + "\n")
    return ledger


def verified_report(package, ledger):
    protocol = verify(package)
    if ledger.get("protocol_sha256") != v4.digest(Path(package) / "protocol.json"):
        raise ValueError("Ledger does not match v5 protocol")
    adapted = copy.deepcopy(ledger)
    adapted["protocol_sha256"] = v4.digest(CORPUS / "protocol.json")
    report = v4.verified_report(CORPUS, adapted)
    report.update(schema_version=5, measurement_basis=protocol["measurement_basis"])
    report["provenance"]["base_manifest_sha256"] = v4.digest(CORPUS / "manifest.json")
    report["provenance"].update(protocol_sha256=v4.digest(Path(package) / "protocol.json"),
                                manifest_sha256=v4.digest(Path(package) / "manifest.json"))
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("verify")
    prep = sub.add_parser("prepare")
    prep.add_argument("destination", type=Path)
    prep.add_argument("--binary", type=Path, required=True)
    args = parser.parse_args()
    if args.command == "verify":
        verify()
        print("V5 package and unchanged v4 corpus/helpers verified.")
    else:
        value = prepare(args.destination, args.binary)
        print(f"Prepared {len(value['trials'])} roots and full launch prompts; no participants launched.")
