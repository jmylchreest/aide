import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { TokenContributionOverview } from "../components/shared/TokenContributionOverview";
import type { TokenAccounting, TokenChange, RetrievalWindow } from "./types";

const delta = (before: number, after: number): TokenChange => ({
  before_bytes: before,
  after_bytes: after,
  delta_bytes: before - after,
  estimated_token_delta: Math.round(before / 3) - Math.round(after / 3),
  events: 1,
});
const window = (
  epoch: string,
  before: number,
  after: number,
): RetrievalWindow => ({
  host: "codex",
  session_id: "session",
  actor_id: "actor",
  epoch,
  first: "2026-09-09T12:00:00Z",
  last: "2026-09-09T12:01:00Z",
  boundary: "reset_observed",
  events: 1,
  observed: {
    bytes: after,
    estimated_tokens: Math.round(after / 3),
    events: 1,
  },
  reference: {
    bytes: before,
    estimated_tokens: Math.round(before / 3),
    events: 1,
  },
  unattributed: { bytes: 0, estimated_tokens: 0, events: 0 },
  comparison: delta(before, after),
  full_read_events: 0,
  search_events: 0,
  failed_events: 0,
  missing_payload: 0,
  clipped: false,
  issues: [],
  sources: [{ file: "file.ts", sha256: "a".repeat(64), bytes: before }],
  steps: [],
  steps_limited: false,
});
function report(): TokenAccounting {
  return {
    version: 1,
    estimator: "utf8-bytes/3-v1",
    by_stage: {
      aide_context: { bytes: 90, estimated_tokens: 30, events: 1 },
    },
    arguments: { bytes: 0, estimated_tokens: 0, events: 0 },
    legacy_events: 0,
    missing_payload: 0,
    missing_identity: 0,
    transformations: {
      by_stage: {
        adapter_change: delta(900, 300),
        rewrite_candidate: delta(900, 150),
      },
      windows: [],
      windows_limited: false,
      unwindowed_events: 0,
      invalid_events: 0,
    },
    retrievals: {
      windows: [window("first", 3000, 600), window("second", 600, 900)],
      windows_limited: true,
      unwindowed_events: 1,
    },
  };
}
const render = (accounting?: TokenAccounting) =>
  renderToStaticMarkup(
    <TokenContributionOverview
      accounting={accounting}
      onDetails={() => {}}
      onAccounting={() => {}}
    />,
  );
describe("contribution overview", () => {
  it("surfaces conditional reductions without adding overlapping stages or hiding added text", () => {
    const html = render(report());
    expect(html).toContain("~200 tokens less");
    expect(html).toContain("~700 tokens less"); // includes the second window's overhead
    expect(html).not.toContain("~900 tokens less"); // no combined savings total
    expect(html).not.toContain("~250 tokens less"); // unselected proposal not added
    expect(html).toContain("Output comparison source");
    expect(html).toContain("2/2 shown windows comparable");
    expect(html).toContain("more windows omitted");
    expect(html).toContain("unassigned observations");
    expect(html).toContain("avoided reads unproven");
    expect(html).toContain("not model or billing savings");
    expect(html).toContain("Prepared aide context: ~30 estimated tokens");
    expect(html).toContain("reference 3,600 bytes, result 1,500 bytes");
  });
  it("shows signed additions, measured zero, and unapplied proposals explicitly", () => {
    const value = report();
    value.transformations!.by_stage = { rewrite_candidate: delta(30, 90) };
    value.retrievals!.windows = [window("zero", 300, 300)];
    const html = render(value);
    expect(html).toContain("Proposed output reduction");
    expect(html).toContain("~20 tokens added");
    expect(html).toContain("60 measured bytes added");
    expect(html).toContain("host may ignore proposals");
    expect(html).toContain("~0 tokens change");
  });
  it("never turns missing or ineligible evidence into zero savings", () => {
    const value = report();
    value.transformations!.by_stage = {};
    value.retrievals!.windows[0].issues = ["context_gap"];
    value.retrievals!.windows[0].comparison = undefined;
    value.retrievals!.windows[1].clipped = true;
    value.retrievals!.windows[1].issues = ["filtered_window"];
    value.retrievals!.windows[1].comparison = undefined;
    const html = render(value);
    expect(html).toContain("Not measured");
    expect(html).toContain("Not established");
    expect(html).toContain("0/2 shown windows comparable");
    expect(html).not.toContain("~0 tokens");
    expect(html).not.toContain('role="img"');
    for (const unavailable of [undefined, { ...value, version: 2 }]) {
      expect(render(unavailable)).toContain("Comparison evidence unavailable");
      expect(render(unavailable)).toContain("Operation evidence unavailable");
    }
  });
});
