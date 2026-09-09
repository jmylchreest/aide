#!/usr/bin/env python3
"""Read copied bridge evidence; never run aide, candidates, or their code.

Use summarize_bridge('bridge/t01', expected_binding=..., work_events=[...]).
Observe exports are controller work and must be captured/timed separately.
"""

import argparse
from collections import Counter
import hashlib
import json
import math
from pathlib import Path, PurePosixPath
import re


TOOLS = {"code_search", "code_references", "code_symbols", "code_outline", "code_read_symbol"}
PHASES = ("guard", "startup_initialize", "identity", "request", "exit", "total")
HASH = re.compile(r"[0-9a-f]{64}\Z")


def strict_json(text):
    def pairs(items):
        value = {}
        for key, item in items:
            if key in value:
                raise ValueError("duplicate JSON key")
            value[key] = item
        return value
    def invalid(value):
        raise ValueError(f"invalid JSON constant {value}")
    return json.loads(text, object_pairs_hook=pairs, parse_constant=invalid)


def read_json(path):
    path = Path(path)
    if any(p.is_symlink() for p in (path, *path.parents)):
        raise ValueError("symlink evidence is not allowed")
    return strict_json(path.read_text(encoding="utf-8"))


def nonnegative(value):
    return value if type(value) in (int, float) and math.isfinite(value) and value >= 0 else None


def uint(value):
    return int(value) if isinstance(value, str) and re.fullmatch(r"0|[1-9][0-9]*", value) else None


def digest(value):
    return isinstance(value, str) and HASH.fullmatch(value) is not None


def relative_source(value):
    if not isinstance(value, str) or not value:
        return False
    path = PurePosixPath(value)
    return not path.is_absolute() and not {"..", ".aide", ".git", ".hg", ".svn", ".bzr", ".fossil"}.intersection(path.parts)


def inventory(value):
    return isinstance(value, dict) and all(relative_source(k) and digest(v) for k, v in value.items())


def text_contents(result):
    if not isinstance(result, dict) or not isinstance(result.get("content"), list):
        raise ValueError("missing result content")
    texts = []
    for part in result["content"]:
        if not isinstance(part, dict) or part.get("type") != "text" or not isinstance(part.get("text"), str):
            raise ValueError("non-text or malformed result")
        texts.append(part["text"])
    return "".join(texts)


def totals(values):
    known = [v for v in values if v is not None]
    return {"known_count": len(known), "unknown_count": len(values) - len(known),
            "known_sum": sum(known), "total": sum(known) if len(known) == len(values) else None}


def validate_setup(setup, expected_binding, expected_binary_sha256):
    errors = []
    if not isinstance(setup, dict):
        return {}, ["malformed_setup"]
    binding = setup.get("binding")
    if not isinstance(binding, dict):
        return {}, ["missing_setup_binding"]
    if (setup.get("operation") != "prepare" or "error" not in setup or setup["error"] is not None
            or type(setup.get("returncode")) is not int or setup["returncode"] != 0):
        errors.append("setup_not_successful")
    if (type(binding.get("schema_version")) is not int or binding.get("schema_version") != 1 or not isinstance(binding.get("id"), str)
            or not binding["id"] or not digest(binding.get("binary_sha256"))
            or not inventory(binding.get("seed_files")) or not binding.get("seed_files")):
        errors.append("malformed_setup_binding")
    for key in ("root", "binary", "evidence_dir"):
        path = binding.get(key)
        if not isinstance(path, str) or not PurePosixPath(path).is_absolute() or ".." in PurePosixPath(path).parts:
            errors.append(f"invalid_setup_{key}")
    if expected_binding is not None and binding != expected_binding:
        errors.append("setup_binding_does_not_match_expected")
    if expected_binary_sha256 is not None and (not digest(expected_binary_sha256) or binding.get("binary_sha256") != expected_binary_sha256):
        errors.append("setup_binary_digest_does_not_match_expected")
    return binding, errors


def check_identity(identity, binding):
    if not isinstance(identity, dict) or not isinstance(binding.get("root"), str):
        return False
    root = binding["root"]
    socket = root + "/.aide/aide.sock"
    if len(socket.encode()) > 100:
        socket = "/tmp/aide/" + hashlib.sha256(root.encode()).hexdigest()[:16] + ".sock"
    return (all(identity.get(k) == root for k in ("project_root", "real_project_root", "cwd"))
            and identity.get("db_path") == root + "/.aide/memory/memory.db"
            and identity.get("socket_path") == socket)


def transcript_requests(record):
    transcript = record.get("transcript")
    if not isinstance(transcript, list):
        raise ValueError("missing transcript")
    requests, responses = {}, {}
    for entry in transcript:
        if not isinstance(entry, dict) or entry.get("direction") not in ("request", "response"):
            raise ValueError("malformed transcript entry")
        message = entry.get("message")
        if not isinstance(message, dict):
            raise ValueError("malformed transcript message")
        if "id" not in message:
            continue
        identifier = message["id"]
        if type(identifier) is not int or identifier <= 0:
            raise ValueError("malformed transcript ID")
        target = requests if entry["direction"] == "request" else responses
        if identifier in target:
            raise ValueError("duplicate transcript ID")
        target[identifier] = message
    if any(identifier not in requests for identifier in responses):
        raise ValueError("response without request")
    calls = [(identifier, message.get("params")) for identifier, message in requests.items()
             if message.get("method") == "tools/call"]
    return calls, responses


def verify_receipts(record, tool, payload, work_events, source_snapshots):
    result = {"work": None, "retrieval": None, "handler_elapsed_ms": None,
              "handler_duration_reason": "no_valid_work_receipt"}
    metadata = record["result"].get("_meta", {})
    if not isinstance(metadata, dict):
        return result
    text_hash = hashlib.sha256(payload).hexdigest()
    work = metadata.get("aide/work")
    retrieval = metadata.get("aide/retrieval")
    if retrieval is not None:
        refs = retrieval.get("references") if isinstance(retrieval, dict) else None
        valid = (isinstance(retrieval, dict) and retrieval.get("version") == 1
                 and isinstance(retrieval.get("id"), str) and bool(retrieval.get("id"))
                 and retrieval.get("tool") == tool and retrieval.get("text_sha256") == text_hash
                 and isinstance(refs, list) and bool(refs))
        rows, seen = [], set()
        for ref in refs if isinstance(refs, list) else []:
            ref = ref if isinstance(ref, dict) else {}
            file = ref.get("file")
            path_valid = relative_source(file)
            duplicate = isinstance(file, str) and file in seen
            if isinstance(file, str):
                seen.add(file)
            files = record.get("source_files")
            hash_matches = (path_valid and digest(ref.get("sha256")) and isinstance(files, dict)
                            and files.get(file) == ref.get("sha256"))
            matching_snapshots = [content for content in source_snapshots.get(file, [])
                                  if hashlib.sha256(content).hexdigest() == ref.get("sha256")] if path_valid else []
            bytes_verified = (all(len(content) == ref.get("bytes") for content in matching_snapshots)
                              if matching_snapshots else None)
            valid = bool(valid and not duplicate and hash_matches
                         and type(ref.get("bytes")) is int and ref["bytes"] >= 0 and bytes_verified is not False)
            rows.append({"file": file, "inside_root": path_valid, "sha256_matches_call_inventory": hash_matches,
                         "reference_bytes": ref.get("bytes"), "byte_count_independently_verified": bytes_verified})
        result["retrieval"] = {"valid": bool(valid), "id": retrieval.get("id") if isinstance(retrieval, dict) else None,
                               "source_references": rows,
                               "verification_boundary": "Source SHA256 checked against pre-call inventory; byte counts independently verified only when a controller-supplied full snapshot has the same SHA256."}
    if work is None:
        return result
    valid = (isinstance(work, dict) and work.get("version") == 1 and isinstance(work.get("id"), str)
             and bool(work.get("id")) and work.get("tool") == tool and work.get("text_sha256") == text_hash)
    result["work"] = {"valid": bool(valid), "id": work.get("id") if isinstance(work, dict) else None,
                      "text_sha256_matches": isinstance(work, dict) and work.get("text_sha256") == text_hash}
    if not valid:
        return result
    if work_events is None:
        result["handler_duration_reason"] = "no_controller_observe_export; protocol_receipt_has_no_duration"
        return result
    matches = [event for event in work_events if isinstance(event, dict) and isinstance(event.get("attrs"), dict)
               and event["attrs"].get("work_id") == work["id"]]
    result["work"]["server_event_match_count"] = len(matches)
    if len(matches) != 1:
        result["handler_duration_reason"] = "server_event_match_not_unique"
        return result
    event = matches[0]
    attrs = event["attrs"]
    elapsed = uint(attrs.get("work_elapsed_ms"))
    event_valid = (event.get("kind") == "tool_call" and event.get("name") == tool
                   and attrs.get("accounting_version") == "1" and attrs.get("observation_stage") == "server_result"
                   and attrs.get("work_version") == "1" and attrs.get("work_text_sha256") == text_hash
                   and uint(attrs.get("payload_bytes")) == len(payload) and elapsed is not None
                   and attrs.get("work_outcome") == ("reported_error" if record["result"].get("isError") else "returned"))
    if retrieval is not None:
        try:
            references_match = (isinstance(retrieval, dict)
                                and strict_json(attrs.get("source_references", "null")) == retrieval.get("references"))
        except (ValueError, TypeError):
            references_match = False
        event_valid = bool(event_valid and isinstance(retrieval, dict) and result["retrieval"]["valid"] and references_match
                           and attrs.get("retrieval_id") == retrieval.get("id"))
    result["work"]["server_event_valid"] = event_valid
    result["work"]["server_event_id"] = event.get("id")
    result["handler_duration_reason"] = "verified_server_work_elapsed_ms" if event_valid else "server_event_receipt_mismatch"
    result["handler_elapsed_ms"] = elapsed if event_valid else None
    return result


def summarize_call(path, binding, setup_errors, work_events, source_snapshots):
    row = {"evidence_file": path.name, "valid": False, "errors": [], "outcome": "invalid_evidence",
           "tool": None, "timings_ms": dict.fromkeys(PHASES), "result_text_utf8_bytes": None,
           "emitted_text_utf8_bytes": None, "handler_elapsed_ms": None, "receipts": None}
    try:
        record = read_json(path)
        if not isinstance(record, dict) or record.get("operation") != "call":
            raise ValueError("not a call evidence record")
        row["id"] = record.get("id")
        if not isinstance(row["id"], str) or not row["id"]:
            row["id"] = None
            row["errors"].append("malformed_call_id")
        row["errors"].extend(setup_errors)
        if record.get("binding_id") != binding.get("id"):
            row["errors"].append("call_binding_mismatch")
        if record.get("index_policy") != binding.get("index_policy"):
            row["errors"].append("call_index_policy_mismatch")
        if "error" not in record or record["error"] is not None and not isinstance(record["error"], str):
            row["errors"].append("malformed_call_error")
        timing = record.get("timings_ms", {})
        if not isinstance(timing, dict):
            timing = {}
        row["timings_ms"] = {phase: nonnegative(timing.get(phase)) for phase in PHASES}
        calls, responses = transcript_requests(record)
        forwarded = record.get("forwarded_request")
        tool_calls = [(identifier, params) for identifier, params in calls if isinstance(params, dict) and params.get("name") != "instance_info"]
        identity_calls = [(identifier, params) for identifier, params in calls if isinstance(params, dict) and params.get("name") == "instance_info"]
        if len(calls) != len(tool_calls) + len(identity_calls) or len(tool_calls) > 1 or len(identity_calls) > 1:
            row["errors"].append("unexpected_transcript_calls")
        requested = bool(tool_calls)
        row["handler_requested"] = requested
        identity_verified = False
        if "identity" in record:
            identity_verified = check_identity(record["identity"], binding)
            if len(identity_calls) == 1:
                response = responses.get(identity_calls[0][0], {})
                identity_verified = identity_verified and strict_json(text_contents(response.get("result"))) == record["identity"]
            else:
                identity_verified = False
            if not identity_verified:
                row["errors"].append("identity_or_transcript_mismatch")
        row["identity_verified"] = identity_verified
        if requested:
            identifier, params = tool_calls[0]
            row["tool"] = params.get("name")
            if row["tool"] not in TOOLS or params != forwarded or not identity_verified:
                row["errors"].append("tool_request_or_identity_mismatch")
        else:
            raw = record.get("request")
            try:
                raw = strict_json(raw) if isinstance(raw, str) else raw
                row["tool"] = raw.get("name") if isinstance(raw, dict) else None
            except ValueError:
                pass
        if record.get("source_files") is not None:
            if not inventory(record["source_files"]):
                row["errors"].append("malformed_call_source_inventory")
            elif record.get("source_changed_since_seed") is not (record["source_files"] != binding.get("seed_files")):
                row["errors"].append("source_change_flag_mismatch")
        row["source_changed_since_seed"] = record.get("source_changed_since_seed")
        failed = record.get("error") is not None
        row["outcome"] = ("rejected" if failed and not calls else "bridge_error") if failed else "success"
        if not failed and (not requested or record.get("returncode") != 0 or record.get("forced_shutdown")):
            row["errors"].append("inconsistent_success_state")
        if record.get("result") is not None:
            payload = text_contents(record["result"]).encode("utf-8")
            row["result_text_utf8_bytes"] = len(payload)
            if not requested or responses.get(tool_calls[0][0], {}).get("result") != record["result"]:
                row["errors"].append("result_transcript_mismatch")
            if record["result"].get("isError") and not failed:
                row["outcome"] = "tool_error"
            row["receipts"] = verify_receipts(record, row["tool"], payload, work_events, source_snapshots)
            row["handler_elapsed_ms"] = row["receipts"]["handler_elapsed_ms"]
            for receipt_kind in ("work", "retrieval"):
                receipt = row["receipts"][receipt_kind]
                if receipt is not None and not receipt["valid"]:
                    row["errors"].append(f"{receipt_kind}_receipt_mismatch")
        elif not failed:
            row["errors"].append("successful_call_without_result")
        row["valid"] = not row["errors"]
        if row["valid"]:
            row["emitted_text_utf8_bytes"] = 0 if failed else row["result_text_utf8_bytes"]
        else:
            row["handler_elapsed_ms"] = None
    except (ValueError, TypeError, KeyError, OSError) as error:
        row["errors"].append(str(error))
    return row


def summarize_bridge(evidence_dir, *, expected_binding=None, expected_binary_sha256=None, work_events=None, source_snapshots=None):
    """Summarize one copied bridge/tID folder; values absent from evidence stay null."""
    directory = Path(evidence_dir)
    setup, binding, setup_errors = None, {}, []
    try:
        setup = read_json(directory / "setup.json")
        binding, setup_errors = validate_setup(setup, expected_binding, expected_binary_sha256)
    except (ValueError, OSError) as error:
        setup_errors = [str(error)]
    if work_events is not None and not isinstance(work_events, list):
        raise ValueError("work_events must be the controller observe export array")
    source_snapshots = {} if source_snapshots is None else source_snapshots
    if (not isinstance(source_snapshots, dict) or any(
            not relative_source(path) or not isinstance(versions, list) or
            any(not isinstance(content, bytes) for content in versions)
            for path, versions in source_snapshots.items())):
        raise ValueError("source_snapshots must map relative files to lists of full source bytes")
    paths = sorted(p for p in directory.glob("*.json") if p.name != "setup.json")
    rows = [summarize_call(path, binding, setup_errors, work_events, source_snapshots) for path in paths]
    ids, receipt_ids = Counter(row.get("id") for row in rows if row.get("id")), Counter()
    for row in rows:
        receipt = (row.get("receipts") or {}).get("work") or {}
        if isinstance(receipt.get("id"), str) and receipt["id"]:
            receipt_ids[receipt["id"]] += 1
    for row in rows:
        receipt = (row.get("receipts") or {}).get("work") or {}
        work_id = receipt.get("id") if isinstance(receipt.get("id"), str) else None
        if ids.get(row.get("id"), 0) > 1 or receipt_ids.get(work_id, 0) > 1:
            row["valid"] = False
            row["errors"].append("duplicate_call_or_work_receipt_id")
            row["handler_elapsed_ms"] = row["emitted_text_utf8_bytes"] = None
    timings = {phase: totals([row["timings_ms"][phase] for row in rows]) for phase in PHASES}
    handler_rows = [row for row in rows if row.get("handler_requested")]
    handler = totals([row["handler_elapsed_ms"] for row in handler_rows])
    complete_handler_duration = bool(handler_rows) and all(row["valid"] for row in rows) and handler["unknown_count"] == 0
    outcomes = Counter(row["outcome"] for row in rows)
    metrics = {
        "schema_version": 1, "evidence_dir": str(directory), "evidence_complete_for_host_attempts": None,
        "setup": {"valid": not setup_errors, "errors": setup_errors, "binding": binding,
                  "index_elapsed_ms": nonnegative(setup.get("index_elapsed_ms")) if isinstance(setup, dict) else None,
                  "total_elapsed_ms": nonnegative(setup.get("total_elapsed_ms")) if isinstance(setup, dict) else None,
                  "expected_binding_verified": expected_binding is not None and not setup_errors,
                  "expected_binary_digest_verified": expected_binary_sha256 is not None and not setup_errors},
        "attempts": len(rows), "successful": outcomes["success"], "tool_errors": outcomes["tool_error"],
        "bridge_errors": outcomes["bridge_error"], "rejected": outcomes["rejected"],
        "invalid_evidence": sum(not row["valid"] for row in rows), "timings_ms": timings,
        "result_text_utf8_bytes": totals([row["result_text_utf8_bytes"] for row in rows]),
        "emitted_text_utf8_bytes": totals([row["emitted_text_utf8_bytes"] for row in rows]),
        "handler_duration_ms": handler, "calls": rows,
        "limitations": [
            "Bridge process timings include orchestration; request roundtrip is not handler compute or CPU time.",
            "Setup indexing and controller observe export costs are separate from participant bridge calls.",
            "Emitted bytes follow the bridge's successful return path; host delivery/truncation and omitted attempts require trace review.",
            "Setup stores a binary digest; call records bind to setup but do not contain an independently repeated binary digest.",
            "Source receipt SHA256 is checked against pre-call inventory. Full-file bytes are independently verified only for supplied snapshots matching that exact hash; other versions remain unknown.",
            "Static seed search/references can be stale after edits; source_changed_since_seed is recorded without assuming result freshness.",
        ],
    }
    return {"bridge_metrics": metrics,
            "aide_duration_ms": handler["total"] if complete_handler_duration else None}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("evidence_dir")
    parser.add_argument("--expected-binding", help="JSON file holding the controller binding")
    parser.add_argument("--expected-binary-sha256")
    parser.add_argument("--work-events", help="Controller-captured observe JSON array")
    args = parser.parse_args()
    print(json.dumps(summarize_bridge(args.evidence_dir,
        expected_binding=read_json(args.expected_binding) if args.expected_binding else None,
        expected_binary_sha256=args.expected_binary_sha256,
        work_events=read_json(args.work_events) if args.work_events else None), indent=2, ensure_ascii=False, allow_nan=False))


if __name__ == "__main__":
    main()
