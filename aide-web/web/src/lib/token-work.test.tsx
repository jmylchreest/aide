import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
  TokenWorkDetails,
  TokenWorkAccounting,
  supportedTokenWork,
} from "../components/shared/TokenWork";
import { TokenOverview } from "../components/shared/TokenOverview";
import type { TokenWork, TokenWorkQuantity, TokenStats } from "./types";

const measured: TokenWorkQuantity = {
  calls: 1,
  returned: 1,
  reported_errors: 0,
  unknown_outcomes: 0,
  elapsed_ms: 0,
  measured_durations: 1,
  missing_durations: 0,
  unassigned_sessions: 0,
  returned_text: { bytes: 0, estimated_tokens: 0, events: 1 },
  missing_payload: 0,
};
const report = (q: TokenWorkQuantity = measured): TokenWork => ({
  ...q,
  version: 1,
  by_tool: { code_outline: q },
});
const empty: TokenWork = {
  calls: 0,
  returned: 0,
  reported_errors: 0,
  unknown_outcomes: 0,
  elapsed_ms: 0,
  measured_durations: 0,
  missing_durations: 0,
  unassigned_sessions: 0,
  returned_text: { bytes: 0, estimated_tokens: 0, events: 0 },
  missing_payload: 0,
  version: 1,
  by_tool: {},
};
const stats = (
  work?: TokenWork,
): Pick<TokenStats, "sessions" | "event_count" | "accounting"> => ({
  sessions: 1,
  event_count: 1,
  accounting: {
    version: 1,
    estimator: "utf8-bytes/3-v1",
    by_stage: {},
    arguments: { bytes: 0, estimated_tokens: 0, events: 0 },
    legacy_events: 0,
    missing_payload: 0,
    missing_identity: 0,
    work,
  },
});

describe("Aide work evidence", () => {
  it("distinguishes measured zero from unknown time and text", () => {
    const known = renderToStaticMarkup(<TokenWorkDetails report={report()} />);
    expect(known).toContain("0 ms");
    expect(known).toContain("0 bytes");
    const unknown = renderToStaticMarkup(
      <TokenWorkDetails
        report={report({
          ...measured,
          measured_durations: 0,
          missing_durations: 1,
          returned_text: { bytes: 0, estimated_tokens: 0, events: 0 },
          missing_payload: 1,
        })}
      />,
    );
    expect(unknown.match(/Unknown/g)).toHaveLength(2);
    expect(unknown).not.toContain("0 ms");
    expect(unknown).not.toContain("0 bytes");
  });
  it("keeps absent older-server data distinct from an empty selection", () => {
    expect(renderToStaticMarkup(<TokenWorkDetails />)).toContain("unavailable");
    expect(renderToStaticMarkup(<TokenWorkDetails report={null} />)).toContain(
      "unavailable",
    );
    expect(renderToStaticMarkup(<TokenWorkDetails report={empty} />)).toContain(
      "No recorded Aide operations in this selection",
    );
  });
  it("rejects unsupported and malformed evidence without displaying misleading zeros", () => {
    for (const bad of [
      null,
      {},
      { ...report(), version: 2 },
      { ...report(), calls: -1 },
      { ...report(), elapsed_ms: NaN },
      { ...report(), missing_payload: "0" },
      { ...report(), returned_text: null },
      { ...report(), by_tool: { code_outline: {} } },
      { ...report(), returned: 3 },
      { ...report(), by_tool: {} },
    ]) {
      expect(supportedTokenWork(bad)).toBeUndefined();
      expect(
        renderToStaticMarkup(<TokenWorkDetails report={bad as TokenWork} />),
      ).toContain("unavailable");
    }
  });
  it("shows failures and incomplete measurements without implying task success", () => {
    const mixed = report({
      ...measured,
      calls: 3,
      returned: 1,
      reported_errors: 1,
      unknown_outcomes: 1,
      missing_durations: 2,
      missing_payload: 2,
      unassigned_sessions: 1,
    });
    const html = renderToStaticMarkup(
      <>
        <TokenWorkDetails report={mixed} />
        <TokenWorkAccounting report={mixed} />
      </>,
    );
    expect(html).toContain("Reported errors: 1");
    expect(html).toContain("Outcomes unknown: 1");
    expect(html).toContain("0 ms (partial)");
    expect(html).toContain("0 bytes (partial)");
    expect(html).toContain("Missing durations: 2");
    expect(html).toContain("Operations without session attribution: 1");
    expect(html).toContain("does not establish task quality");
    expect(html).toContain("not CPU time");
    expect(html).toContain("not additional usage");
    expect(html).toContain("do not establish avoided model work");
    expect(html).not.toContain("Success rate");
  });
  it("adds only a compact overview link for supported data without changing the activity chart", () => {
    const render = (work?: TokenWork) =>
      renderToStaticMarkup(
        <TokenOverview
          stats={stats(work)}
          onDetails={() => {}}
          onAccounting={() => {}}
        />,
      );
    expect(render()).not.toContain("Aide work");
    const html = render(report());
    expect(html).toContain("Aide work · 1 recorded operation");
    expect(html).toContain("Recorded token activity");
    expect(html).toContain("Estimated result tokens");
    expect(html).toContain("Recorded events");
    expect(html).toContain("Sessions");
    expect(html).not.toContain("code_outline");
    expect(html).not.toContain("Elapsed tool time");
  });
});
