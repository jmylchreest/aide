"""Derive the v6 report from frozen protocol, captures and independent reviews."""
import copy
import importlib.util
import json
from pathlib import Path
import sys

BASE = Path(__file__).resolve().parent
SHARED = BASE.parents[1]
PACKAGE = SHARED / "outcomes-v6"
CORPUS = SHARED / "outcomes-v4"


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


runner = module("v6_report_runner", PACKAGE / "run.py")
v4 = runner.v4
bridge = module("v6_bridge_summary", BASE.parent / "2026-09-09-outcomes-v5/bridge_summary.py")


def write(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False, allow_nan=False) + "\n")


def assemble(base=BASE):
    base = Path(base).resolve()
    protocol = runner.verify(PACKAGE)
    ledger = v4.read_json(base / "ledger.json")
    if ledger.get("protocol_sha256") != v4.digest(PACKAGE / "protocol.json"):
        raise ValueError("Ledger does not match frozen v6 protocol")
    if [{k: row.get(k) for k in ("id", "task", "treatment", "repetition")}
            for row in ledger.get("trials", [])] != protocol["trial_order"]:
        raise ValueError("Ledger differs from frozen four-cell plan")
    if ledger.get("retrieval_bridge", {}).get("binary_sha256") != protocol["retrieval_bridge"]["binary_sha256"]:
        raise ValueError("Ledger binary differs from frozen v6 protocol")
    mapping = v4.read_json(base / "blinding.json")["mapping"]
    if set(mapping) != {row["id"] for row in ledger["trials"]} or set(mapping.values()) != {"L4", "B9", "S2", "G7"}:
        raise ValueError("Unexpected four-cell blinding map")
    for row in ledger["trials"]:
        tid = row["id"]
        audit_path = base / (tid + "-trace-review.json")
        if audit_path.exists():
            audit = v4.read_json(audit_path)
            for key in ("scope_verified", "trace_reviewed", "protocol_violations", "aide_operations",
                        "aide_uptake", "retrieval_output_bytes", "hook_metrics"):
                row[key] = audit.get(key)
            row["trace_evidence"] = audit
        review_path = base / ("quality-" + mapping[tid] + ".json")
        if review_path.exists():
            row["implementation_review"] = v4.read_json(review_path)
        folder = base / "bridge" / tid
        if folder.exists():
            prelaunch = base / (tid + "-prelaunch.json")
            binding = v4.read_json(prelaunch)["binding"] if prelaunch.exists() else None
            sources = {}
            for root in (CORPUS / "common/template", Path(row["root"])):
                for source in root.rglob("*"):
                    relative = source.relative_to(root)
                    if source.is_file() and ".aide" not in relative.parts:
                        sources.setdefault(relative.as_posix(), []).append(source.read_bytes())
            events_path = base / (tid + "-server-work.json")
            summary = bridge.summarize_bridge(folder, expected_binding=binding,
                expected_binary_sha256=protocol["retrieval_bridge"]["binary_sha256"],
                work_events=v4.read_json(events_path) if events_path.exists() else None,
                source_snapshots=sources)
            metrics = summary["bridge_metrics"]
            metrics["evidence_complete_for_host_attempts"] = (row.get("trace_reviewed") is True
                and row.get("aide_operations") == metrics["attempts"] and metrics["invalid_evidence"] == 0)
            row.update(summary)
            row["bridge_duration_ms"] = metrics["timings_ms"]["total"]["total"]
            row["bridge_setup"] = metrics["setup"]
    ledger["trials"] = [v4.implementation_quality(CORPUS, row) for row in ledger["trials"]]
    report = v4.inherited("outcomes-v3/run.py").report(protocol, copy.deepcopy(ledger))
    supplied = {row["id"]: row for row in ledger["trials"]}
    for row in report["trials"]:
        source = supplied[row["id"]]
        for key in ("functional_quality_verified", "functional_quality", "implementation_review", "trace_evidence"):
            row[key] = source.get(key)
        audit = source.get("trace_evidence") or {}
        for key in ("response_evidence", "overlap_review", "execution_verification"):
            row[key] = audit.get(key)
        if not row["quality_verified"]:
            row["unknown_reasons"].append("implementation_grade_or_independent_review_pending")
    report.update(schema_version=6, measurement_basis=protocol["measurement_basis"], provenance={
        "protocol_sha256": v4.digest(PACKAGE / "protocol.json"),
        "manifest_sha256": v4.digest(PACKAGE / "manifest.json"),
        "base_manifest_sha256": v4.digest(runner.V5 / "manifest.json"),
        "dependencies": protocol["dependencies"], "controller_helpers": protocol["controller_helpers"],
        "report_helpers": {name: v4.digest(BASE / name) for name in ("assemble_report.py", "render_report.py")},
        "bridge_summary_sha256": v4.digest(BASE.parent / "2026-09-09-outcomes-v5/bridge_summary.py")},
        measurement_limitations=[
            "Frozen inherited reporting accepts only integer top-level bridge_duration_ms. Fractional measured elapsed remains in bridge_metrics.timings_ms.total.total; the HTML uses that field without rounding the evidence.",
            "Four implementation trials, two repetitions per condition, current shared host guidance and uncontrolled provider cache. Historical v5 observations are not pooled or a causal baseline. No billing, general savings or cross-host performance claim.",
            "Response and source-overlap evidence describe observed work. They do not identify counterfactual avoided calls or saved tokens; source bytes are not substituted for runtime counters."])
    write(base / "reviewed-ledger.json", ledger)
    write(base / "report.json", report)
    return report


if __name__ == "__main__":
    result = assemble(Path(sys.argv[1]) if len(sys.argv) > 1 else BASE)
    print(json.dumps({"planned":result["planned_trials"],
        "completed":sum(r["status"] == "completed" for r in result["trials"]),
        "quality_passed":sum(r["quality_verified"] and r["quality"]["passed"] for r in result["trials"]),
        "protocol_valid":sum(r["protocol_valid"] for r in result["trials"]),
        "eligible_comparisons":len(result["comparisons"])}))
