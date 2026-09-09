"""Snapshot runtime-reported evaluation overhead in explicit timestamp windows."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import re

BASE = Path(__file__).resolve().parent
ROOT_SESSION = "01a07b15-668c-7453-b59c-9be3d32b2005"
START = "2026-09-09T18:14:29.148Z"  # Current root user request; ordinal 14886.
START_EVIDENCE = {"root_log": "/home/johnm/.codex/sessions/2026/09/07/rollout-2026-09-07T09-56-35-01a07b15-668c-7453-b59c-9be3d32b2005.jsonl",
                  "user_ordinal": 14886, "user_text": "rebuilt/restsrted. please continue if there is something next?",
                  "first_commentary_ordinal": 14889, "first_commentary_at": "2026-09-09T18:14:31.747Z"}
REUSED_EVALUATORS = {"/root/implementation_value_review", "/root/guidance_runner"}
END = datetime.now(timezone.utc).isoformat()
EXECUTION = (json.loads((BASE / "execution-start.json").read_text())["started_at"]
             if (BASE / "execution-start.json").exists() else None)
spec = importlib.util.spec_from_file_location("collector", BASE.parents[1] / "collect_codex.py")
c = importlib.util.module_from_spec(spec)
spec.loader.exec_module(c)


def total(values):
    values = list(values)
    return {key: sum(value[key] for value in values) for key in c.COUNTERS}


actors = []
for path in Path("/home/johnm/.codex/sessions").glob("2026/09/*/*.jsonl"):
    with path.open() as stream:
        try:
            meta = json.loads(stream.readline()).get("payload", {})
        except (ValueError, TypeError):
            continue
    actor = meta.get("agent_path") or "/root"
    is_root = meta.get("id") == ROOT_SESSION
    if not is_root and (meta.get("session_id") != ROOT_SESSION or not (actor.startswith("/root/v6_") or actor in REUSED_EVALUATORS)):
        continue
    if re.search(r"/v6_t\d{2}$", actor):
        continue  # Task participant usage is reported separately.
    data = path.read_bytes()
    all_seen, selected, errors, cumulative = {}, {}, [], None
    for line in data.decode().splitlines():
        row = c._strict_json(line)
        if row.get("type") != "token_usage_record":
            continue
        payload = row["payload"]
        rid = payload.get("response_id")
        usage = c._counters(payload.get("usage"))
        running = c._counters(payload.get("thread_token_usage"))
        if not rid or usage is None or running is None or payload.get("thread_id") != meta.get("id"):
            errors.append("invalid_usage_record")
            continue
        if rid in all_seen and all_seen[rid] != usage:
            errors.append("conflicting_response")
        all_seen[rid] = usage
        cumulative = running
        timestamp = c._time(row.get("timestamp"))
        if timestamp is None:
            errors.append("invalid_response_timestamp")
        elif c._time(START) <= timestamp <= c._time(END):
            selected[rid] = {"response_id": rid, "timestamp": row["timestamp"], "counters": usage,
                            "phase": "preparation" if EXECUTION is None or timestamp < c._time(EXECUTION) else "execution_review_reporting"}
    if total(all_seen.values()) != cumulative:
        errors.append("cumulative_mismatch")
    counts = None if errors else total(row["counters"] for row in selected.values())
    phases = None if errors else {phase: total(row["counters"] for row in selected.values() if row["phase"] == phase)
                                  for phase in ("preparation", "execution_review_reporting")}
    actors.append({"agent_path": actor, "actor_id": meta["id"], "raw_log": str(path),
                   "snapshot_sha256": hashlib.sha256(data).hexdigest(), "runtime_usage": counts,
                   "phase_usage": phases, "unknown_reasons": sorted(set(errors)), "responses": list(selected.values())})
result = {"basis": "Runtime-reported controller/preparation/review overhead; not participant usage, normal aide overhead, billing or CPU.",
          "window_start": START, "window_start_evidence": START_EVIDENCE,
          "actor_selection": {"root_session": ROOT_SESSION, "evaluator_prefix": "/root/v6_",
                              "explicit_reused_evaluators": sorted(REUSED_EVALUATORS),
                              "excluded_participant_pattern": r"/v6_t\d{2}$"},
          "execution_start": EXECUTION, "window_end": END, "actors": actors,
          "known_actor_sum": total(row["runtime_usage"] for row in actors if row["runtime_usage"] is not None),
          "unknown_actors": sum(row["runtime_usage"] is None for row in actors),
          "limitations": ["Response-timestamp snapshot excludes responses finishing after cutoff, including this capture and final delivery.",
                          "Root reuses prior context; input includes replay of that context. Cached input remains a subset.",
                          "Phase is assigned at response completion; a response can include work started before a boundary.",
                          "Current-window work of explicitly reused evaluator actors is included; earlier v4 responses are outside the window.",
                          "Before execution-start.json exists, captured work is classified as preparation and execution_start remains null."]}
(BASE / "overhead.json").write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps({"actors": len(actors), "unknown": result["unknown_actors"], "known_sum": result["known_actor_sum"]}))
