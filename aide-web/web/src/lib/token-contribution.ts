import type { TokenChange, TokenQuantity } from "./types";

const object = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === "object" && !Array.isArray(value);
const integer = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value);
const count = (value: unknown): value is number => integer(value) && value >= 0;
const identity = (value: unknown): value is string =>
  typeof value === "string" && value.trim().length > 0;

function change(value: unknown): value is TokenChange {
  return (
    object(value) &&
    count(value.before_bytes) &&
    count(value.after_bytes) &&
    count(value.events) &&
    integer(value.delta_bytes) &&
    integer(value.estimated_token_delta) &&
    value.delta_bytes === value.before_bytes - value.after_bytes &&
    (value.events > 0 ||
      (value.before_bytes === 0 &&
        value.after_bytes === 0 &&
        value.estimated_token_delta === 0))
  );
}

function quantity(value: unknown): value is TokenQuantity {
  return (
    object(value) &&
    count(value.bytes) &&
    count(value.estimated_tokens) &&
    count(value.events) &&
    (value.events > 0 || (value.bytes === 0 && value.estimated_tokens === 0))
  );
}

/** Stage totals include all recorded pairs, including unwindowed observations. */
export function validatedTransformationChange(
  report: unknown,
  stage: string,
): TokenChange | undefined {
  if (
    !["adapter_change", "rewrite_candidate"].includes(stage) ||
    !object(report) ||
    !object(report.by_stage) ||
    !Array.isArray(report.windows) ||
    typeof report.windows_limited !== "boolean" ||
    !count(report.unwindowed_events) ||
    !count(report.invalid_events)
  )
    return;
  const value = report.by_stage[stage];
  if (!change(value) || value.events === 0) return;
  return value;
}

export interface RetrievalContribution {
  change?: TokenChange;
  eligible_windows: number;
  shown_windows: number;
  windows_limited: boolean;
  unwindowed_events: number;
}

/** Conditional full-file comparisons are never an estimate of provider savings. */
export function summarizeRetrievalContribution(
  report: unknown,
): RetrievalContribution | undefined {
  if (
    !object(report) ||
    !Array.isArray(report.windows) ||
    report.windows.length > 64 ||
    typeof report.windows_limited !== "boolean" ||
    !count(report.unwindowed_events)
  )
    return;
  const summary: RetrievalContribution = {
    eligible_windows: 0,
    shown_windows: report.windows.length,
    windows_limited: report.windows_limited,
    unwindowed_events: report.unwindowed_events,
  };
  const identities = new Set<string>();
  for (const window of report.windows) {
    if (
      !object(window) ||
      ![window.host, window.session_id, window.actor_id, window.epoch].every(
        identity,
      ) ||
      window.session_id === "unknown" ||
      !count(window.events) ||
      window.events === 0 ||
      !quantity(window.reference) ||
      !quantity(window.observed) ||
      !quantity(window.unattributed) ||
      !count(window.missing_payload) ||
      typeof window.clipped !== "boolean" ||
      !Array.isArray(window.issues) ||
      !window.issues.every(identity)
    )
      return;
    const key = JSON.stringify([
      window.host,
      window.session_id,
      window.actor_id,
      window.epoch,
    ]);
    if (identities.has(key)) return;
    identities.add(key);
    const measured = window.observed.events + window.unattributed.events;
    const observations = measured + window.missing_payload;
    if (
      !count(measured) ||
      !count(observations) ||
      observations !== window.events
    )
      return;

    const comparison = window.comparison;
    if (comparison === undefined || comparison === null) continue;
    if (
      !change(comparison) ||
      comparison.events !== window.events ||
      comparison.before_bytes !== window.reference.bytes ||
      comparison.after_bytes !== window.observed.bytes ||
      comparison.estimated_token_delta !==
        window.reference.estimated_tokens - window.observed.estimated_tokens
    )
      return;
    if (
      window.clipped ||
      window.issues.length > 0 ||
      window.missing_payload > 0 ||
      window.unattributed.events > 0 ||
      window.reference.events === 0
    )
      continue;

    const total = summary.change ?? {
      before_bytes: 0,
      after_bytes: 0,
      delta_bytes: 0,
      estimated_token_delta: 0,
      events: 0,
    };
    const next: TokenChange = {
      before_bytes: total.before_bytes + comparison.before_bytes,
      after_bytes: total.after_bytes + comparison.after_bytes,
      delta_bytes: total.delta_bytes + comparison.delta_bytes,
      estimated_token_delta:
        total.estimated_token_delta + comparison.estimated_token_delta,
      events: total.events + comparison.events,
    };
    if (!change(next)) return;
    summary.change = next;
    summary.eligible_windows++;
  }
  return summary;
}
