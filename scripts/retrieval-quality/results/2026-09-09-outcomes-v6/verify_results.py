"""Verify retained result evidence; optional raw-log checks need the original machine."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path

BASE = Path(__file__).resolve().parent


def read(path):
    return json.loads(path.read_text())


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify(raw=False):
    for manifest, root in (("evidence-manifest.json", BASE), ("blind-inputs-manifest.json", BASE / "blind-inputs")):
        for entry in read(BASE / manifest)["files"]:
            path = BASE / entry["retained_source"] if "retained_source" in entry else root / entry["path"]
            assert not path.is_symlink() and path.stat().st_size == entry["bytes"], path
            assert digest(path) == entry["sha256"], path
    package = BASE.parents[1] / "outcomes-v6"
    spec = importlib.util.spec_from_file_location("v6_verify", package / "run.py")
    runner = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(runner)
    protocol = runner.verify()
    ledger = read(BASE / "ledger.json")
    assert ledger["protocol_sha256"] == digest(package / "protocol.json")
    assert [{k: row[k] for k in ("id", "task", "treatment", "repetition")} for row in ledger["trials"]] == protocol["trial_order"]
    actors = []
    for row in ledger["trials"]:
        tid = row["id"]
        capture = read(BASE / f"{tid}-capture.json")
        provenance = read(BASE / f"{tid}-provenance.json")
        prompt = (BASE / f"{tid}-launch-message.txt").read_text()
        assert prompt == runner.prompt_for(protocol, row, ledger["retrieval_bridge"]["binary"])
        assert digest(BASE / f"{tid}-launch-message.txt") == row["prompt_sha256"]
        assert capture["completion"]["complete"] and row["status"] == "completed"
        assert capture["runtime"]["actor_id"] == row["agent_id"] == provenance["runtime_actor_id"]
        actors.append(row["agent_id"])
        for key, total in capture["runtime_usage"]["counters"].items():
            assert sum(r["counters"][key] for r in capture["runtime_usage"]["responses"]) == total
        if raw:
            path = Path(provenance["raw_log"])
            assert digest(path) == provenance["raw_log_sha256"]
            assert runner.v4.inherited("collect_codex.py").collect_log(path) == capture
    assert len(set(actors)) == 4
    print("V6 result hashes, frozen inputs, launch archives and counters verified" + ("; raw logs match" if raw else ""))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify-raw", action="store_true")
    verify(parser.parse_args().verify_raw)
