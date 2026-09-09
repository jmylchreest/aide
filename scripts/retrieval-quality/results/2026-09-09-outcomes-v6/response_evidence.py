"""Associate captured responses with outgoing calls without executing their inputs."""
from pathlib import Path
import hashlib
import json
import sys

BASE = Path(__file__).resolve().parent


def derive(tid):
    capture = json.loads((BASE / f"{tid}-capture.json").read_text())
    provenance = json.loads((BASE / f"{tid}-provenance.json").read_text())
    raw = Path(provenance["raw_log"]).read_bytes()
    assert hashlib.sha256(raw).hexdigest() == provenance["raw_log_sha256"]
    pending, rows, messages = [], [], []
    for line in raw.decode().splitlines():
        event = json.loads(line)
        payload = event.get("payload", {})
        if event.get("type") == "response_item":
            if payload.get("type") in ("custom_tool_call", "function_call"):
                pending.append(payload["call_id"])
            if payload.get("type") == "message" and payload.get("role") in ("system", "developer", "user"):
                # Record only exposure indicators, not unrelated host instruction text.
                parts = payload.get("content", [])
                plaintext = "\n".join(p.get("text", "") for p in parts if isinstance(p, dict)) if isinstance(parts, list) else ""
                messages.append({"timestamp": event.get("timestamp"), "role": payload["role"],
                                 "visible_text_bytes": len(plaintext.encode()),
                                 "contains_revised_guidance": "a current symbol-body read can supply that evidence" in plaintext,
                                 "contains_old_guidance": "Choose retrieval that helps correctness and avoids redundant work" in plaintext,
                                 "limitations": "Absence from visible text does not prove absence from encrypted or implicit host context."})
        if event.get("type") == "token_usage_record":
            counters = payload["usage"]
            assert counters["input_tokens"] >= counters["cached_input_tokens"]
            rows.append({"response_id": payload["response_id"], "counters": counters,
                         "uncached_input_tokens": counters["input_tokens"] - counters["cached_input_tokens"],
                         "timestamp": event["timestamp"], "outgoing_call_ids": pending})
            pending = []
    assert not pending
    assert [{"response_id": r["response_id"], "counters": r["counters"]} for r in rows] == capture["runtime_usage"]["responses"]
    assert [c for r in rows for c in r["outgoing_call_ids"]] == [c["call_id"] for c in capture["tool_calls"]]
    return {"trial": tid, "raw_log_sha256": provenance["raw_log_sha256"],
            "basis": "Outgoing calls are associated with the next runtime usage record and checked against the frozen collector. Input includes accumulated context; these are not per-tool costs.",
            "responses": rows, "observable_instruction_exposure": messages}


if __name__ == "__main__":
    tid = sys.argv[1]
    result = derive(tid)
    (BASE / f"{tid}-response-evidence.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"trial": tid, "responses": len(result["responses"])}))
