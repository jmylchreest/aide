#!/usr/bin/env python3
"""Derive implementation accounting from frozen v5 evidence; launch no trials."""
import argparse
import hashlib
import json
from pathlib import Path

BASE = Path(__file__).resolve().parent
RESULTS = BASE.parent.parent / "results/2026-09-09-outcomes-v5"
# Manual trace annotations: 1-based response containing the first source edit.
# Inspect capture.tool_calls and trace-review.input_review to audit these boundaries.
FIRST_EDIT = {"t05": 3, "t06": 5, "t07": 4, "t08": 3}


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def derive(verify_raw=False):
    manifest = json.loads((RESULTS / "evidence-manifest.json").read_text())
    for entry in manifest["files"]:
        path = RESULTS / entry["path"]
        assert path.stat().st_size == entry["bytes"], path
        assert digest(path) == entry["sha256"], path
    hashes = {"evidence-manifest.json": digest(RESULTS / "evidence-manifest.json")}
    trials = {}
    for tid, first_edit in FIRST_EDIT.items():
        docs = {}
        for kind in ("capture", "trace-review", "provenance"):
            name = f"{tid}-{kind}.json"
            docs[kind] = json.loads((RESULTS / name).read_text())
            hashes[name] = digest(RESULTS / name)
        capture, review = docs["capture"], docs["trace-review"]
        usage = capture["runtime_usage"]
        responses = usage["responses"]
        calls = [c["call_id"] for c in capture["tool_calls"]]
        assert calls == [r["call_id"] for r in review["input_review"]]
        assert len(calls) == len(responses) - 1
        # The frozen traces have one outgoing outer call per nonfinal response.
        # Optional local raw verification checks this association, not just counts.
        if verify_raw:
            provenance = docs["provenance"]
            raw = Path(provenance["raw_log"])
            assert digest(raw) == provenance["raw_log_sha256"], raw
            pending, observed = [], []
            for line in raw.read_text().splitlines():
                event = json.loads(line)
                payload = event.get("payload", {})
                if event.get("type") == "response_item" and payload.get("type") in ("custom_tool_call", "function_call"):
                    pending.append(payload["call_id"])
                if event.get("type") == "token_usage_record":
                    observed.append((payload["response_id"], payload["usage"], pending))
                    pending = []
            expected = [(r["response_id"], r["counters"], [calls[i]] if i < len(calls) else [])
                        for i, r in enumerate(responses)]
            assert observed == expected and not pending, tid
        totals = usage["counters"]
        assert all(sum(r["counters"][k] for r in responses) == v for k, v in totals.items())
        timeline, phases = [], {}
        for index, response in enumerate(responses, 1):
            phase = ("final_response" if index == len(responses) else
                     "before_first_edit" if index < first_edit else "editing_and_verification")
            counters = dict(response["counters"])
            counters["uncached_input_tokens"] = counters["input_tokens"] - counters["cached_input_tokens"]
            assert counters["uncached_input_tokens"] >= 0
            timeline.append({"response": index, "response_id": response["response_id"],
                             "outgoing_call_id": calls[index - 1] if index <= len(calls) else None,
                             "phase": phase, "counters": counters})
            subtotal = phases.setdefault(phase, {k: 0 for k in counters})
            for key, value in counters.items():
                subtotal[key] += value
        trials[tid] = {"condition": "assisted" if tid in ("t06", "t07") else "ordinary",
                       "counters": dict(totals, uncached_input_tokens=totals["input_tokens"] - totals["cached_input_tokens"]),
                       "elapsed_ms": capture["completion"]["duration_ms"],
                       "source_output_bytes": review["retrieval_output_bytes"],
                       "response_count": len(responses), "first_edit_response": first_edit,
                       "phases": phases, "timeline": timeline}
    pairs = []
    for ordinary, assisted in (("t05", "t06"), ("t08", "t07")):
        a, b = trials[ordinary], trials[assisted]
        delta = {k: b["counters"][k] - v for k, v in a["counters"].items()}
        pairs.append({"ordinary": ordinary, "assisted": assisted, "counter_delta": delta,
                      "input_change_percent": 100 * delta["input_tokens"] / a["counters"]["input_tokens"],
                      "cached_share_of_input_increase_percent": 100 * delta["cached_input_tokens"] / delta["input_tokens"],
                      "source_output_bytes_delta": b["source_output_bytes"] - a["source_output_bytes"],
                      "elapsed_ms_delta": b["elapsed_ms"] - a["elapsed_ms"],
                      "phase_input_deltas": {k: b["phases"][k]["input_tokens"] - v["input_tokens"]
                                             for k, v in a["phases"].items()}})
    return {"basis": "Runtime counters, not billing or causal savings. Cached input is a subset of input; reasoning is a subset of output. Phases label response activity, not tool-attributable token costs.",
            "source_hashes": hashes, "trials": trials, "pairs": pairs}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify-raw", action="store_true", help="Also verify locally retained provider logs and response/call mapping")
    parser.add_argument("--check", action="store_true", help="Compare with committed accounting.json instead of writing it")
    args = parser.parse_args()
    output = json.dumps(derive(args.verify_raw), indent=2) + "\n"
    target = BASE / "accounting.json"
    if args.check:
        assert target.read_text() == output, "Derived accounting differs"
    else:
        target.write_text(output)
    print("Frozen evidence and derived accounting verified" + (" including raw logs" if args.verify_raw else ""))
