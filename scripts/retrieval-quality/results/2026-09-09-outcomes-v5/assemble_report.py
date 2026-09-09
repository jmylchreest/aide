"""Assemble reviewed evidence without changing the frozen experiment package."""
from pathlib import Path
import importlib.util
import json

BASE = Path(__file__).resolve().parent
PACKAGE = BASE.parents[1] / "outcomes-v5"
CORPUS = BASE.parents[1] / "outcomes-v4"


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def read(path):
    return json.loads(path.read_text())


def write(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False, allow_nan=False) + "\n")


runner = module("v5_runner", PACKAGE / "run.py")
bridge = module("v5_bridge_summary", BASE / "bridge_summary.py")
runner.verify(PACKAGE)
ledger = read(BASE / "ledger.json")
mapping = read(BASE / "blinding.json")["mapping"]
for row in ledger["trials"]:
    tid = row["id"]
    audit_path = BASE / (tid + "-trace-review.json")
    if audit_path.exists():
        audit = read(audit_path)
        for key in ("scope_verified", "trace_reviewed", "protocol_violations", "aide_operations",
                    "aide_uptake", "retrieval_output_bytes", "hook_metrics"):
            row[key] = audit.get(key)
    review_path = BASE / ("quality-" + mapping[tid] + ".json")
    if review_path.exists():
        review = read(review_path)
        if row["task"] == "implement":
            row["implementation_review"] = review
        else:
            required = {check["id"] for check in read(CORPUS / row["task"] / "rubric.json")["checks"]}
            checks = review["checks"]
            if (not isinstance(checks, list) or len(checks) != len(required)
                    or {check["id"] for check in checks} != required
                    or any(type(check.get("passed")) is not bool or not check.get("evidence") for check in checks)
                    or not review.get("reviewer") or review["reviewer"] == row.get("agent_id")):
                raise ValueError("Incomplete independent rubric evidence: " + tid)
            row.update(quality_verified=True, quality={"passed": all(c["passed"] for c in checks),
                       "passed_checks": sum(c["passed"] for c in checks), "total_checks": len(checks),
                       "reviewer": review["reviewer"], "checks": checks})
    folder = BASE / "bridge" / tid
    if not folder.exists():
        continue
    prelaunch = BASE / (tid + "-prelaunch.json")
    binding = read(prelaunch)["binding"] if prelaunch.exists() else None
    sources = {}
    for root in (CORPUS / "common/template", Path(row["root"])):
        for source in root.rglob("*"):
            relative = source.relative_to(root)
            if source.is_file() and ".aide" not in relative.parts:
                sources.setdefault(relative.as_posix(), []).append(source.read_bytes())
    events_path = BASE / (tid + "-server-work.json")
    summary = bridge.summarize_bridge(folder, expected_binding=binding,
        expected_binary_sha256=ledger["retrieval_bridge"]["binary_sha256"],
        work_events=read(events_path) if events_path.exists() else None, source_snapshots=sources)
    metrics = summary["bridge_metrics"]
    # Recorded attempts and visible host calls must be reconciled by the trace reviewer.
    metrics["evidence_complete_for_host_attempts"] = (row.get("trace_reviewed") is True
        and row.get("aide_operations") == metrics["attempts"] and metrics["invalid_evidence"] == 0)
    row.update(summary)
    row["bridge_duration_ms"] = metrics["timings_ms"]["total"]["total"]
    row["bridge_setup"] = metrics["setup"]
write(BASE / "reviewed-ledger.json", ledger)
report = runner.verified_report(PACKAGE, ledger)
report["measurement_limitations"] = [
    "The frozen reporter accepts only integer bridge_duration_ms values, so its top-level field is null for measured fractional durations. The preserved bridge_metrics.timings_ms.total.total is the measured bridge elapsed time in milliseconds; the HTML uses that nested field. No rounding or frozen-runner change was applied."
]
write(BASE / "report.json", report)
print(json.dumps({"planned": report["planned_trials"], "completed": sum(r["status"] == "completed" for r in report["trials"]),
                  "quality_passed": sum(r["quality_verified"] and r["quality"]["passed"] for r in report["trials"]),
                  "protocol_valid": sum(r["protocol_valid"] for r in report["trials"]),
                  "eligible_comparisons": len(report["comparisons"])}))
