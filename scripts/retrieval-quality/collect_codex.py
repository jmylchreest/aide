#!/usr/bin/env python3
"""Collect allowlisted evidence from one explicitly selected, completed trial log.

Runtime counters are reported evidence, not billing or provider savings. This
module never discovers logs, executes tool inputs, or starts model sessions.
"""

import argparse
from datetime import datetime
import json
import math
from pathlib import Path


COUNTERS = (
    "input_tokens", "cached_input_tokens", "cache_write_input_tokens",
    "output_tokens", "reasoning_output_tokens", "total_tokens",
)
META = ("id", "session_id", "agent_path", "cli_version", "model_provider")


def _identity(value):
    return isinstance(value, str) and bool(value.strip())


def _text(value):
    return value if isinstance(value, str) else None


def _invalid_constant(value):
    raise ValueError("Non-finite JSON number")


def _unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            # Do not echo potentially sensitive keys or values in diagnostics.
            raise ValueError("Duplicate JSON object key")
        result[key] = value
    return result


def _strict_json(text):
    return json.loads(text, parse_constant=_invalid_constant, object_pairs_hook=_unique_object)


def _counters(value):
    if not isinstance(value, dict):
        return None
    if any(type(value.get(key)) is not int or value[key] < 0 for key in COUNTERS):
        return None
    return {key: value[key] for key in COUNTERS}


def _time(value):
    if type(value) in (int, float):
        if value < 0 or (type(value) is float and not math.isfinite(value)):
            return None
        return value
    if not isinstance(value, str):
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
        return parsed.timestamp() if parsed.tzinfo is not None else None
    except (ValueError, OverflowError, OSError):
        return None


def collect_log(path):
    """Return a JSON-serializable report; unknown quantities remain None.

    Bad JSON/rows fail closed because omitted rows could contain usage evidence.
    Only selected metadata, tool-call inputs, the final answer, completion times,
    and runtime usage counters are exported. Other message content is discarded.
    """
    reasons = set()
    metadata, context, final_text, completion = None, None, None, None
    tool_calls, usage_records = [], []
    with Path(path).open(encoding="utf-8") as stream:
        for line_number, line in enumerate(stream, 1):
            if not line.strip():
                continue
            try:
                row = _strict_json(line)
            except (ValueError, TypeError):
                raise ValueError(f"Invalid JSON at line {line_number}") from None
            if not isinstance(row, dict) or not isinstance(row.get("payload"), dict):
                raise ValueError(f"Invalid row at line {line_number}")
            kind, payload = row.get("type"), row["payload"]
            if kind == "session_meta":
                selected = {key: _text(payload.get(key)) for key in META}
                if any(not _identity(value) for value in selected.values()):
                    reasons.add("missing_runtime_metadata")
                if metadata is not None and selected != metadata:
                    reasons.add("conflicting_runtime_metadata")
                metadata = metadata or selected
            elif kind == "turn_context":
                selected = {key: _text(payload.get(key)) for key in ("model", "effort")}
                if any(not _identity(value) for value in selected.values()):
                    reasons.add("missing_model_metadata")
                if context is not None and selected != context:
                    reasons.add("conflicting_model_metadata")
                context = context or selected
            elif kind == "token_usage_record":
                usage_records.append({key: payload.get(key) for key in
                                      ("thread_id", "response_id", "usage", "thread_token_usage")})
            elif kind == "event_msg" and payload.get("type") == "task_complete":
                selected = {key: payload.get(key) if _time(payload.get(key)) is not None else None
                            for key in ("started_at", "completed_at")}
                selected["duration_ms"] = (payload.get("duration_ms")
                                           if type(payload.get("duration_ms")) is int else None)
                if completion is not None and completion != selected:
                    reasons.add("conflicting_completion")
                completion = completion or selected
            elif kind == "response_item":
                item_type = payload.get("type")
                if item_type in ("custom_tool_call", "function_call"):
                    tool_calls.append({
                        "call_id": _text(payload.get("call_id")), "name": _text(payload.get("name")),
                        "type": item_type,
                        "input": _text(payload.get("input" if item_type == "custom_tool_call" else "arguments")),
                    })
                elif (item_type == "message" and payload.get("role") == "assistant"
                      and payload.get("phase") == "final_answer"):
                    content = payload.get("content")
                    if not isinstance(content, list):
                        reasons.add("invalid_final_answer")
                        continue
                    pieces = [part["text"] for part in content if isinstance(part, dict)
                              and part.get("type") == "output_text" and isinstance(part.get("text"), str)]
                    selected = "".join(pieces)
                    if not pieces:
                        reasons.add("invalid_final_answer")
                    if final_text is not None and selected != final_text:
                        reasons.add("conflicting_final_answer")
                    if final_text is None:
                        final_text = selected

    if metadata is None:
        reasons.add("missing_runtime_metadata")
    if context is None:
        reasons.add("missing_model_metadata")
    if final_text is None:
        reasons.add("missing_final_answer")
    completion = completion or {}
    start, end = _time(completion.get("started_at")), _time(completion.get("completed_at"))
    duration = completion.get("duration_ms")
    complete = bool(start is not None and end is not None and end >= start
                    and type(duration) is int and duration >= 0
                    and final_text is not None and "conflicting_completion" not in reasons)
    if not complete:
        reasons.add("incomplete_trial")

    actor_id = (metadata or {}).get("id")
    responses, cumulative = {}, None
    for record in usage_records:
        response_id = record["response_id"]
        if not _identity(record["thread_id"]) or record["thread_id"] != actor_id:
            reasons.add("usage_thread_mismatch")
        if not _identity(response_id):
            reasons.add("missing_response_identity")
            continue
        usage = _counters(record["usage"])
        running = _counters(record["thread_token_usage"])
        if usage is None or running is None:
            reasons.add("invalid_or_missing_usage_counter")
            continue
        if response_id in responses and responses[response_id] != usage:
            reasons.add("conflicting_response_usage")
        responses.setdefault(response_id, usage)
        cumulative = running
    if not usage_records:
        reasons.add("missing_usage")
    summed = {key: sum(value[key] for value in responses.values()) for key in COUNTERS}
    if cumulative is not None and summed != cumulative:
        reasons.add("cumulative_usage_mismatch")

    parsed, parse_error = None, None
    if final_text is not None:
        try:
            parsed = _strict_json(final_text)
        except ValueError:
            parse_error = "Final answer is not valid JSON"
    metadata = metadata or {}
    return {
        "schema_version": 1,
        "runtime": {"actor_id": metadata.get("id"),
                    **{key: metadata.get(key) for key in META if key != "id"},
                    **(context or {"model": None, "effort": None})},
        "completion": {"complete": complete,
                       **{key: completion.get(key) for key in ("started_at", "completed_at", "duration_ms")}},
        "answer": {"raw_text": final_text, "parsed_json": parsed, "parse_error": parse_error},
        "tool_calls": tool_calls,
        "runtime_usage": {
            "basis": "runtime_reported; not billing or provider savings",
            "counters": None if reasons else summed,
            "responses": None if reasons else [
                {"response_id": response_id, "counters": counters}
                for response_id, counters in responses.items()
            ],
            "unique_responses": len(responses),
            "final_cumulative_counters": None if reasons else cumulative,
            "unknown_reasons": sorted(reasons),
        },
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--log", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    try:
        report = collect_log(args.log)
        # Exclusive creation: never overwrite source logs or prior evidence.
        with args.output.open("x", encoding="utf-8") as output:
            json.dump(report, output, indent=2, ensure_ascii=False, allow_nan=False)
            output.write("\n")
    except (OSError, ValueError) as error:
        parser.exit(1, f"Collection failed: {error}\n")


if __name__ == "__main__":
    main()
