#!/usr/bin/env python3
"""Preflight and archive full launch messages; never launch participants itself."""
import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile

spec = importlib.util.spec_from_file_location("v5_runner", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)
v4 = runner.v4
bridge = v4.inherited("outcomes-v3/bridge.py")
collector = v4.inherited("collect_codex.py")


def validate_message(message, expected):
    if not isinstance(message, str) or message != expected:
        raise ValueError("Launch message must equal the complete prepared prompt")
    return hashlib.sha256(message.encode("utf-8")).hexdigest()


def next_row(ledger, tid, results):
    rows = ledger["trials"]
    matches = [i for i, row in enumerate(rows) if row["id"] == tid]
    if len(matches) != 1:
        raise ValueError("Unknown or duplicate trial")
    selected = matches[0]
    suffixes = ("-prelaunch.json", "-launch.json", "-capture.json", "-launch-message.txt")
    for later in rows[selected + 1:]:
        if later["status"] != "planned" or any((results / (later["id"] + s)).exists() for s in suffixes):
            raise ValueError("Later cell already started; execution must stay sequential")
    seen = set()
    for row in rows:
        if row["id"] == tid:
            if row["status"] != "planned" or any((results / (tid + suffix)).exists()
                    for suffix in suffixes):
                raise ValueError("Trial was already prepared, launched or captured; no replacements")
            return row
        if row["status"] != "completed" or not row.get("agent_id") or row["agent_id"] in seen:
            raise ValueError("Previous planned cells must have distinct completed actors")
        seen.add(row["agent_id"])
        capture = v4.read_json(results / (row["id"] + "-capture.json"))
        provenance = v4.read_json(results / (row["id"] + "-provenance.json"))
        if (capture["completion"]["complete"] is not True
                or capture["runtime"]["actor_id"] != row["agent_id"]
                or row.get("runtime_session_id") != row["agent_id"]
                or provenance.get("runtime_actor_id") != row["agent_id"]
                or provenance.get("raw_log") != row["log_path"]
                or v4.digest(row["log_path"]) != provenance["raw_log_sha256"]):
            raise ValueError("Previous completed capture identity or raw evidence mismatch")
        if collector.collect_log(row["log_path"]) != capture:
            raise ValueError("Saved previous capture differs from recollected runtime evidence")
    raise ValueError("Unknown trial")


def preflight(results, tid):
    results = Path(results).resolve()
    protocol = runner.verify()
    ledger = v4.read_json(results / "ledger.json")
    if (ledger["protocol_sha256"] != v4.digest(runner.BASE / "protocol.json")
            or [{k:r[k] for k in ("id", "task", "treatment", "repetition")} for r in ledger["trials"]]
            != protocol["trial_order"]):
        raise ValueError("Ledger differs from frozen plan")
    row = next_row(ledger, tid, results)
    binary = ledger["retrieval_bridge"]["binary"]
    expected = runner.prompt_for(protocol, row, binary)
    raw = Path(row["prompt_path"]).read_bytes()
    digest = validate_message(raw.decode("utf-8"), expected)
    if digest != row["prompt_sha256"]:
        raise ValueError("Prepared prompt digest mismatch")
    root, binary, evidence, binding = bridge.bound_configuration(
        Path(row["root"]), Path(binary), Path(row["bridge_evidence_dir"]))
    if binding["binary_sha256"] != protocol["retrieval_bridge"]["binary_sha256"]:
        raise ValueError("Binary differs from frozen protocol")
    if {p.name for p in evidence.iterdir()} != {"setup.json"}:
        raise ValueError("Bridge evidence contains prior calls or unexpected files")
    actual = bridge.source_inventory(root)
    with tempfile.TemporaryDirectory(prefix="aide-v5-prelaunch-") as temp:
        baseline = Path(temp) / "root"
        v4.copy_template(runner.CORPUS, row["task"], baseline)
        if actual != bridge.source_inventory(baseline) or actual != binding["seed_files"]:
            raise ValueError("Initial source differs from frozen fixture or seed")
    record = {"id":tid, "verified_at":datetime.now(timezone.utc).isoformat(),
              "prompt_sha256":digest, "launcher_message":expected,
              "binding":binding, "initial_source_inventory":actual,
              "prior_cells_completed":True, "no_previous_bridge_calls":True,
              "message_equality_basis":"Controller checks exact UTF-8 prompt bytes; encrypted host messages are not independently decrypted."}
    v4.write_json(results / (tid + "-prelaunch.json"), record)
    with (results / (tid + "-launch-message.txt")).open("x", encoding="utf-8", newline="") as stream:
        stream.write(expected)
    return record


def launched(results, tid, actor):
    results = Path(results).resolve()
    pre = v4.read_json(results / (tid + "-prelaunch.json"))
    archived = (results / (tid + "-launch-message.txt")).read_text()
    if validate_message(archived, pre["launcher_message"]) != pre["prompt_sha256"]:
        raise ValueError("Archived launch changed")
    for path in results.glob("t??-launch.json"):
        if v4.read_json(path)["agent_path"] == actor:
            raise ValueError("Actor already assigned")
    v4.write_json(results / (tid + "-launch.json"), {
        "id":tid, "launch_recorded_at":datetime.now(timezone.utc).isoformat(),
        "agent_path":actor, "message":archived, "message_sha256":pre["prompt_sha256"],
        "fork_turns":"none", "model_override":None, "effort_override":None})


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("preflight", "launched"))
    parser.add_argument("results", type=Path)
    parser.add_argument("trial")
    parser.add_argument("--actor")
    args = parser.parse_args()
    if args.mode == "preflight":
        record = preflight(args.results, args.trial)
        print(record["launcher_message"], end="")
    else:
        if not args.actor:
            parser.error("--actor is required for launched")
        launched(args.results, args.trial, args.actor)
        print("Launch recorded")
