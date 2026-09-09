"""Combine manual command classifications with decoded captured output evidence."""
from collections import Counter
import importlib.util
import json
from pathlib import Path
import re
import sys

BASE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("v5_bodies", BASE.with_name("2026-09-09-outcomes-v5") / "trace_bodies.py")
bodies = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bodies)
bodies.BASE = BASE


def audit(tid):
    read = lambda name: json.loads((BASE / name).read_text())
    annotations = read(f"{tid}-trace-annotations.json")
    capture = read(f"{tid}-capture.json")
    decoded = bodies.bodies(tid)
    inputs, boundaries, tests, syntax = [], [], [], []
    counts = Counter()
    assert len(annotations["categories"]) == len(capture["tool_calls"])
    for index, (call, categories) in enumerate(zip(capture["tool_calls"], annotations["categories"]), 1):
        # All submitted commands in these reviewed trials use JSON string literals.
        # Fail rather than evaluate JS or guess when another form appears.
        commands = [json.loads(s) for s in re.findall(r'\bcmd\s*:\s*("(?:\\.|[^"\\])*")', call["input"])]
        assert len(commands) == len(categories), (tid, index, "command classification mismatch")
        outputs = [b for b in decoded if b["call_id"] == call["call_id"]]
        assert len(outputs) == len(commands), (tid, index, "output correspondence unknown")
        inputs.append({"call_id": call["call_id"], "nested_calls": len(commands),
                       "commands_reviewed": commands, "categories": categories,
                       "rtk_prefix_compliant": all(c.startswith("rtk ") for c in commands),
                       "scope_compliant": annotations["scope_verified"]})
        for command, category, output in zip(commands, categories, outputs):
            assert category in ("bootstrap", "discovery", "source", "edit", "test", "transpile")
            text = output["output"]
            complete = not output.get("unattributable") and not re.search(r"Warning: truncated output|tokens truncated|Output truncated", text, re.I)
            counts[category] += 1
            boundaries.append({"call_id": call["call_id"], "content_index": output["content_index"],
                               "part_index": output["part_index"], "category": category,
                               "utf8_bytes": len(text.encode()), "complete": complete,
                               "exit_code": output.get("exit_code")})
            if category in ("test", "transpile"):
                (tests if category == "test" else syntax).append({"call_id": call["call_id"], "command": command,
                              "exit_code": output.get("exit_code"), "output": text,
                              "complete": complete})
    sources = [b for b in boundaries if b["category"] == "source"]
    total = sum(b["utf8_bytes"] for b in sources)
    complete = all(b["complete"] for b in sources)
    response = read(f"{tid}-response-evidence.json")
    assert all(r["rtk_prefix_compliant"] for r in inputs)
    result = {"trial": tid, "experiment_version": 6, "reviewer": "root",
              "review_type": "manual_protocol_scope_and_overlap_with_derived_output_bytes",
              "trace_reviewed": True, "scope_verified": annotations["scope_verified"],
              "protocol_violations": annotations["protocol_violations"],
              "aide_operations": annotations["aide_operations"],
              "aide_uptake": annotations["aide_operations"] > 0,
              "counts": dict(counts, outer_tool_calls=len(inputs), nested_tool_calls=sum(counts.values())),
              "input_review": inputs, "retrieval_output_bytes": total if complete else None,
              "byte_evidence": {"basis": "Sum decoded stdout/stderr output UTF-8 bytes for manually classified source commands, excluding wrappers. No byte-to-token conversion.",
                                "source_output_complete": complete, "source_lower_bound_bytes": total,
                                "included_boundaries": boundaries},
              "response_evidence": response,
              "overlap_review": annotations["overlap_review"],
              "execution_verification": {"test_executions": tests, **({"syntax_executions": syntax} if syntax else {}), **annotations["execution_verification"]},
              "scope_evidence": annotations["scope_evidence"]}
    (BASE / f"{tid}-trace-review.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"trial": tid, "source_bytes": result["retrieval_output_bytes"], "counts": result["counts"]}))


if __name__ == "__main__":
    audit(sys.argv[1])
