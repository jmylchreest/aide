#!/usr/bin/env python3
"""Compare explicitly named, graded before/after pilot reports offline.

Pairs retain both report rows, including failures and unverified outcomes. Each
metric has before/after values and a signed after-minus-before delta only when
both outcomes pass and both measurements are available and eligible. No agent
runs, cost estimates, percentages, aggregate effects or causal claims are made.
"""

import argparse
import copy
from pathlib import Path

from report import _nonnegative_number, _read_json, write_report


USAGE_FIELDS = ("input_tokens", "cached_input_tokens", "cache_write_input_tokens",
                "output_tokens", "reasoning_output_tokens", "total_tokens")


def _index(report, side):
    if not isinstance(report, dict) or report.get("kind") != "retrieval_quality_pilot_report" or type(report.get("schema")) is not int or report["schema"] != 1:
        raise ValueError(f"{side}: expected a schema 1 pilot report")
    rows = report.get("trials")
    if not isinstance(rows, list) or not rows:
        raise ValueError(f"{side}: no trial rows")
    if report.get("complete") is not True or report.get("missing_ids") != [] or report.get("duplicate_ids") != []:
        raise ValueError(f"{side}: incomplete or duplicate trial set")
    for key in ("expected_trials", "recorded_trials"):
        if type(report.get(key)) is not int or report[key] != len(rows):
            raise ValueError(f"{side}: {key} does not match trial rows")
    indexed, identities = {}, set()
    for row in rows:
        if not isinstance(row, dict) or any(not isinstance(row.get(key), str) or not row[key] for key in ("id", "task", "treatment")):
            raise ValueError(f"{side}: invalid trial identity")
        original = row.get("original")
        if not isinstance(original, dict) or any(original.get(key) != row[key] for key in ("id", "task", "treatment")):
            raise ValueError(f"{side}: original trial identity mismatch")
        key = (row["task"], row["treatment"])
        if key in indexed or row["id"] in identities:
            raise ValueError(f"{side}: duplicate trial identity or task/treatment pair")
        indexed[key] = row
        identities.add(row["id"])
    return indexed


def _measurement(before, after, eligible, integer=True):
    before = before if _nonnegative_number(before, integer) else None
    after = after if _nonnegative_number(after, integer) else None
    return {"before": before, "after": after,
            "delta": after - before if eligible and before is not None and after is not None else None}


def _comparison_conditions(left, right):
    runtime = {}
    reasons = []
    for field in ("model", "effort", "model_provider", "cli_version"):
        values = []
        for side, row in (("before", left), ("after", right)):
            metadata = row["original"].get("runtime")
            value = metadata.get(field) if isinstance(metadata, dict) else None
            values.append(value)
            if not isinstance(value, str) or not value.strip():
                reasons.append(f"{side} runtime.{field} is missing or not a nonempty string")
        known = all(isinstance(value, str) and value.strip() for value in values)
        matches = bool(known and values[0] == values[1])
        if known and not matches:
            reasons.append(f"runtime.{field} differs between before and after")
        runtime[field] = {"before": values[0], "after": values[1], "matches": matches}
    return {"runtime": runtime, "runtime_metadata_matches": not reasons, "reasons": reasons}


def build_comparison(before, after):
    """Pair report rows without mutating them or treating unknown as zero.

    Verification flags remain grader attestations, as in report.py. Validity of
    source-call counts rests on the passing trace review; model-response counts
    additionally require the original runtime-usage attestation.
    """
    old, new = _index(before, "before"), _index(after, "after")
    if old.keys() != new.keys():
        raise ValueError("missing peer: before/after task and treatment sets differ")
    pairs = []
    for key, left in old.items():
        right = new[key]
        if left["id"] != right["id"]:
            raise ValueError(f"trial identity mismatch for {key}")
        passing = all(row.get("outcome") == "pass" and row.get("task_pass") is True
                      and not row.get("issues") and row.get("failure") is None for row in (left, right))
        text_trusted = all(row["original"].get("text_measurement_verified") is True for row in (left, right))
        usage_trusted = all(row["original"].get("provider_usage_verified") is True for row in (left, right))
        conditions = _comparison_conditions(left, right)
        runtime_eligible = passing and conditions["runtime_metadata_matches"]
        metrics = {
            "observed_text_bytes": _measurement(left.get("observed_text_bytes"), right.get("observed_text_bytes"), passing and text_trusted),
            "wall_time_seconds": _measurement(left.get("wall_time_seconds"), right.get("wall_time_seconds"), runtime_eligible, integer=False),
            "source_calls": _measurement(left["original"].get("source_calls"), right["original"].get("source_calls"), passing),
            "model_responses": _measurement(left["original"].get("model_responses"), right["original"].get("model_responses"), runtime_eligible and usage_trusted),
        }
        left_usage = left.get("provider_usage") if isinstance(left.get("provider_usage"), dict) else {}
        right_usage = right.get("provider_usage") if isinstance(right.get("provider_usage"), dict) else {}
        fields = sorted(set(USAGE_FIELDS) | left_usage.keys() | right_usage.keys())
        metrics["provider_usage"] = {
            field: _measurement(left_usage.get(field), right_usage.get(field), runtime_eligible and usage_trusted)
            for field in fields
        }
        pairs.append({"id": left["id"], "task": key[0], "treatment": key[1],
                      "both_pass": passing, "comparison_conditions": conditions, "before": copy.deepcopy(left),
                      "after": copy.deepcopy(right), "metrics": metrics})
    return {
        "schema": 1, "kind": "retrieval_quality_pilot_comparison",
        "delta_definition": "after minus before; null unless both trials pass and the metric is eligible on both sides",
        "limitations": [
            "Measurement verification flags are grader attestations, not independently validated here.",
            "Provider usage fields are runtime-reported counters, not billing verification; cached input is not added to input tokens.",
            "Source bytes, source calls, model responses and runtime input measure different boundaries.",
            "Wall time and call counts retain the report's measurement boundary and validity review.",
            "Runtime usage, model-response and wall-time deltas require matching nonempty model, effort, provider and CLI metadata.",
            "Matching runtime metadata does not verify cache equivalence; source and host conditions require separate review.",
            "Source-byte and source-call deltas are arithmetic observations, not causal effects.",
            "Paired observations do not establish causality, savings, costs or statistical confidence.",
        ],
        "pairs": pairs,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--before", type=Path, required=True)
    parser.add_argument("--after", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        result = build_comparison(_read_json(args.before), _read_json(args.after))
        write_report(args.output, result)
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.exit(2, f"error: {error}\n")


if __name__ == "__main__":
    main()
