import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { TokenOverview } from "../components/shared/TokenOverview";
import type { TokenStats } from "./types";

const stats: Pick<TokenStats, "sessions" | "event_count" | "accounting"> = {
  sessions: 2,
  event_count: 5,
  accounting: {
    version: 1,
    estimator: "utf8-bytes/3-v1",
    by_stage: {
      host_result: { bytes: 6, estimated_tokens: 2, events: 1 },
      server_result: { bytes: 300, estimated_tokens: 100, events: 1 },
    },
    arguments: { bytes: 0, estimated_tokens: 0, events: 0 },
    legacy_events: 1,
    missing_payload: 1,
    missing_identity: 2,
    activity: {
      interval_seconds: 60,
      buckets: [
        {
          start: "2026-09-07T10:00:00Z",
          by_stage: {
            host_result: { bytes: 6, estimated_tokens: 2, events: 1 },
          },
          unmeasured: 1,
        },
      ],
    },
  },
};

describe("token overview", () => {
  it("uses a single labelled observation source and hides diagnostic prose", () => {
    const html = renderToStaticMarkup(
      <TokenOverview
        stats={stats}
        onDetails={() => {}}
        onAccounting={() => {}}
      />,
    );
    expect(html).toContain("~2");
    expect(html).not.toContain("~102");
    expect(html).toContain("Recorded token activity");
    expect(html).toContain("Partial data");
    expect(html).not.toContain("utf8-bytes/3-v1");
    expect(html).not.toContain("Historical and compatibility estimates");
    expect(html).not.toContain("burndown");
  });
  it("keeps unknown and known empty quantities distinct", () => {
    const empty = structuredClone(stats);
    empty.accounting!.by_stage.host_result = {
      bytes: 0,
      estimated_tokens: 0,
      events: 1,
    };
    expect(
      renderToStaticMarkup(
        <TokenOverview
          stats={empty}
          onDetails={() => {}}
          onAccounting={() => {}}
        />,
      ),
    ).toContain("~0");
    const missing = structuredClone(stats);
    missing.accounting!.by_stage = {};
    expect(
      renderToStaticMarkup(
        <TokenOverview
          stats={missing}
          onDetails={() => {}}
          onAccounting={() => {}}
        />,
      ),
    ).toContain("Unknown");
  });
});
