import { describe, expect, it } from "vitest";
import {
  summarizeRetrievalContribution,
  validatedTransformationChange,
} from "./token-contribution";
import type { RetrievalWindow, TokenChange, TokenRetrievals } from "./types";

const paired = (
  before: number,
  after: number,
  tokens: number,
): TokenChange => ({
  before_bytes: before,
  after_bytes: after,
  delta_bytes: before - after,
  estimated_token_delta: tokens,
  events: 1,
});
const transformations = () => ({
  by_stage: {
    adapter_change: paired(30, 60, -10),
    rewrite_candidate: paired(300, 30, 90),
  },
  windows: [],
  windows_limited: false,
  unwindowed_events: 2,
  invalid_events: 1,
});
const window = (epoch = "a"): RetrievalWindow => ({
  host: "codex",
  session_id: "session",
  actor_id: "actor",
  epoch,
  first: "2026-09-10T12:00:00Z",
  last: "2026-09-10T12:01:00Z",
  boundary: "open_or_unknown",
  events: 1,
  reference: { bytes: 30, estimated_tokens: 10, events: 1 },
  observed: { bytes: 6, estimated_tokens: 2, events: 1 },
  unattributed: { bytes: 0, estimated_tokens: 0, events: 0 },
  comparison: paired(30, 6, 8),
  full_read_events: 0,
  search_events: 0,
  failed_events: 0,
  missing_payload: 0,
  clipped: false,
  issues: [],
  sources: [],
  steps: [],
  steps_limited: false,
});
const retrievals = (...windows: RetrievalWindow[]): TokenRetrievals => ({
  windows,
  windows_limited: false,
  unwindowed_events: 0,
});

describe("transformation contribution evidence", () => {
  it("preserves independent stage reductions and added text", () => {
    const report = transformations();
    expect(validatedTransformationChange(report, "adapter_change")).toEqual(
      paired(30, 60, -10),
    );
    expect(validatedTransformationChange(report, "rewrite_candidate")).toEqual(
      paired(300, 30, 90),
    );
    report.windows_limited = true;
    expect(
      validatedTransformationChange(report, "adapter_change")?.events,
    ).toBe(1);
  });

  it("preserves per-pair rounding and measured zero, without making absence zero", () => {
    const report = transformations();
    // Two 1-byte results each round to zero tokens; aggregate bytes round to one.
    report.by_stage.adapter_change = { ...paired(2, 0, 0), events: 2 };
    expect(
      validatedTransformationChange(report, "adapter_change")
        ?.estimated_token_delta,
    ).toBe(0);
    report.by_stage.adapter_change = paired(0, 0, 0);
    expect(
      validatedTransformationChange(report, "adapter_change")?.delta_bytes,
    ).toBe(0);
    report.by_stage.adapter_change.events = 0;
    expect(
      validatedTransformationChange(report, "adapter_change"),
    ).toBeUndefined();
    expect(
      validatedTransformationChange(undefined, "adapter_change"),
    ).toBeUndefined();
    expect(
      validatedTransformationChange(
        { ...report, by_stage: {} },
        "adapter_change",
      ),
    ).toBeUndefined();
    expect(
      validatedTransformationChange(report, "host_result"),
    ).toBeUndefined();
  });

  it("rejects malformed quantities, arithmetic and coverage metadata", () => {
    for (const patch of [
      { before_bytes: -1 },
      { after_bytes: 0.5 },
      { delta_bytes: 1 },
      { estimated_token_delta: Infinity },
      { before_bytes: Number.MAX_SAFE_INTEGER + 1 },
      { events: 0 },
      { events: "1" },
    ]) {
      const report = transformations();
      Object.assign(report.by_stage.adapter_change, patch);
      expect(
        validatedTransformationChange(report, "adapter_change"),
      ).toBeUndefined();
    }
    expect(
      validatedTransformationChange(
        { ...transformations(), unwindowed_events: -1 },
        "adapter_change",
      ),
    ).toBeUndefined();
  });
});

describe("conditional retrieval contribution evidence", () => {
  it("aggregates comparable windows including negative changes and explicit limits", () => {
    const overhead = window("b");
    overhead.observed = { bytes: 60, estimated_tokens: 20, events: 1 };
    overhead.comparison = paired(30, 60, -10);
    const report = {
      ...retrievals(window(), overhead),
      windows_limited: true,
      unwindowed_events: 3,
    };
    expect(summarizeRetrievalContribution(report)).toEqual({
      change: { ...paired(60, 66, -2), events: 2 },
      eligible_windows: 2,
      shown_windows: 2,
      windows_limited: true,
      unwindowed_events: 3,
    });
  });

  it("does not recompute rounded estimates from total bytes", () => {
    const first = window();
    first.reference = { bytes: 1, estimated_tokens: 0, events: 1 };
    first.observed = { bytes: 0, estimated_tokens: 0, events: 1 };
    first.comparison = paired(1, 0, 0);
    const second = { ...first, epoch: "b" };
    expect(
      summarizeRetrievalContribution(retrievals(first, second))?.change,
    ).toEqual({ ...paired(2, 0, 0), events: 2 });
  });

  it("counts gaps and noncomparable windows without manufacturing savings", () => {
    const clipped = window("clipped");
    clipped.clipped = true;
    clipped.issues = ["filtered_window"];
    delete clipped.comparison;
    const incomplete = window("missing");
    incomplete.missing_payload = 1;
    incomplete.observed = { bytes: 0, estimated_tokens: 0, events: 0 };
    incomplete.issues = ["missing_payload"];
    delete incomplete.comparison;
    const report = summarizeRetrievalContribution(
      retrievals(clipped, incomplete),
    );
    expect(report).toMatchObject({ eligible_windows: 0, shown_windows: 2 });
    expect(report?.change).toBeUndefined();
    expect(
      summarizeRetrievalContribution(retrievals())?.change,
    ).toBeUndefined();
  });

  it("requires clear evidence even when a comparison is present", () => {
    for (const mutation of [
      (w: RetrievalWindow) => {
        w.clipped = true;
      },
      (w: RetrievalWindow) => {
        w.issues = ["unverified_source_version"];
      },
      (w: RetrievalWindow) => {
        w.events = 2;
        w.missing_payload = 1;
        w.comparison!.events = 2;
      },
      (w: RetrievalWindow) => {
        w.events = 2;
        w.unattributed.events = 1;
        w.comparison!.events = 2;
      },
    ]) {
      const candidate = window();
      mutation(candidate);
      expect(
        summarizeRetrievalContribution(retrievals(candidate))?.eligible_windows,
      ).toBe(0);
    }
  });

  it("rejects duplicate identities but permits the same epoch in another actor", () => {
    expect(
      summarizeRetrievalContribution(retrievals(window(), window())),
    ).toBeUndefined();
    expect(
      summarizeRetrievalContribution(
        retrievals(window(), { ...window(), actor_id: "other" }),
      )?.eligible_windows,
    ).toBe(2);
  });

  it("rejects mismatched counters, unsafe quantities and malformed reports", () => {
    for (const mutation of [
      (w: RetrievalWindow) => {
        w.reference.bytes++;
      },
      (w: RetrievalWindow) => {
        w.observed.estimated_tokens++;
      },
      (w: RetrievalWindow) => {
        w.comparison!.events++;
      },
      (w: RetrievalWindow) => {
        w.events++;
      },
      (w: RetrievalWindow) => {
        w.actor_id = "";
      },
      (w: RetrievalWindow) => {
        w.reference.bytes = Number.MAX_SAFE_INTEGER + 1;
      },
      (w: RetrievalWindow) => {
        w.observed.bytes = -1;
      },
    ]) {
      const candidate = window();
      mutation(candidate);
      expect(
        summarizeRetrievalContribution(retrievals(candidate)),
      ).toBeUndefined();
    }
    for (const value of [
      null,
      {},
      { ...retrievals(), unwindowed_events: -1 },
      { ...retrievals(), windows_limited: "yes" },
    ]) {
      expect(summarizeRetrievalContribution(value)).toBeUndefined();
    }
  });

  it("accepts several source references in one retrieval call", () => {
    const candidate = window();
    candidate.reference.events = 2;
    const result = summarizeRetrievalContribution(retrievals(candidate));
    expect(result?.eligible_windows).toBe(1);
    expect(result?.change?.events).toBe(1);
    expect(result?.change?.delta_bytes).toBe(candidate.comparison!.delta_bytes);
  });

  it("retains the 64-window cap and rejects unsafe aggregate arithmetic", () => {
    const windows = Array.from({ length: 64 }, (_, i) => window(String(i)));
    expect(
      summarizeRetrievalContribution(retrievals(...windows))?.shown_windows,
    ).toBe(64);
    expect(
      summarizeRetrievalContribution(retrievals(...windows, window("65"))),
    ).toBeUndefined();
    const huge = window("huge");
    huge.reference.bytes = Number.MAX_SAFE_INTEGER;
    huge.reference.estimated_tokens = Number.MAX_SAFE_INTEGER;
    huge.observed = { bytes: 0, estimated_tokens: 0, events: 1 };
    huge.comparison = paired(
      Number.MAX_SAFE_INTEGER,
      0,
      Number.MAX_SAFE_INTEGER,
    );
    expect(summarizeRetrievalContribution(retrievals(huge))).toBeDefined();
    expect(
      summarizeRetrievalContribution(retrievals(huge, window())),
    ).toBeUndefined();
    // Even cancelling deltas cannot make unsafe before/after totals acceptable.
    const inverse = window("inverse");
    inverse.reference = { bytes: 0, estimated_tokens: 0, events: 1 };
    inverse.observed = {
      bytes: Number.MAX_SAFE_INTEGER,
      estimated_tokens: Number.MAX_SAFE_INTEGER,
      events: 1,
    };
    inverse.comparison = paired(
      0,
      Number.MAX_SAFE_INTEGER,
      -Number.MAX_SAFE_INTEGER,
    );
    expect(
      summarizeRetrievalContribution(retrievals(huge, inverse, window())),
    ).toBeUndefined();
  });
});
