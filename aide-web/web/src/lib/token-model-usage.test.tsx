import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
  ModelUsageAccounting,
  supportedModelUsage,
} from "../components/shared/TokenModelUsage";
import { TokenOverview } from "../components/shared/TokenOverview";
import type { ModelUsage, TokenStats } from "./types";

const report: ModelUsage = {
  version: 1,
  observations: 2,
  conflicts: 1,
  invalid: 1,
  by_source: [
    {
      host: "claude-code",
      source: "claude.assistant_usage.v1",
      observations: 2,
      source_timed: 2,
      observed_timed: 0,
      counters: {
        input_tokens: { tokens: 1234, observations: 1 },
        cache_read_input_tokens: { tokens: 0, observations: 2 },
      },
    },
  ],
};

describe("host-reported model usage", () => {
  it("keeps counter coverage, missing output and reported zero distinct", () => {
    const html = renderToStaticMarkup(<ModelUsageAccounting report={report} />);
    expect(html).toContain("1,234");
    expect(html).toContain("1/2 observations");
    expect(html).toContain("2/2 observations");
    expect(html).toContain("Unknown");
    expect(html).toContain("Conflicting: 1");
    expect(html).toContain("Invalid: 1");
    expect(html).toContain("partial coverage");
    expect(html).toContain("task quality");
    expect(html).not.toContain("~1,234");
  });
  it("does not combine OpenCode reported output with normalized output or cache with input", () => {
    const usage = structuredClone(report);
    usage.by_source[0] = {
      host: "opencode",
      source: "opencode.step_finish.v1",
      observations: 2,
      observed_timed: 2,
      source_timed: 0,
      counters: {
        input_tokens: { tokens: 100, observations: 2 },
        cache_read_input_tokens: { tokens: 50, observations: 2 },
        reported_output_tokens: { tokens: 7, observations: 2 },
        reasoning_output_tokens: { tokens: 3, observations: 2 },
      },
    };
    const html = renderToStaticMarkup(<ModelUsageAccounting report={usage} />);
    expect(html).toContain("Reported output (overlap unknown)");
    expect(html).toContain("Observation time: 2");
    expect(html).not.toContain(">150<");
    expect(html).not.toContain(">10<");
  });
  it("treats empty, unsupported and malformed reports honestly", () => {
    expect(
      renderToStaticMarkup(
        <ModelUsageAccounting
          report={{
            version: 1,
            observations: 0,
            conflicts: 0,
            invalid: 0,
            by_source: [],
          }}
        />,
      ),
    ).toContain("No model usage observations");
    for (const bad of [
      undefined,
      null,
      {},
      { ...report, version: 2 },
      { ...report, observations: 3 },
      { ...report, invalid: -1 },
      {
        ...report,
        by_source: [
          {
            ...report.by_source[0],
            counters: { input_tokens: { tokens: NaN, observations: 1 } },
          },
        ],
      },
      { ...report, by_source: [{ ...report.by_source[0], source_timed: 1 }] },
    ]) {
      expect(supportedModelUsage(bad)).toBeUndefined();
      expect(
        renderToStaticMarkup(
          <ModelUsageAccounting report={bad as ModelUsage} />,
        ),
      ).toContain("unavailable");
    }
  });
  it("keeps the overview compact and sends usage details to Accounting", () => {
    const stats: Pick<TokenStats, "sessions" | "event_count" | "accounting"> = {
      sessions: 1,
      event_count: 0,
      accounting: {
        version: 1,
        estimator: "utf8-bytes/3-v1",
        by_stage: {},
        arguments: { bytes: 0, estimated_tokens: 0, events: 0 },
        legacy_events: 0,
        missing_payload: 0,
        missing_identity: 0,
        model_usage: report,
      },
    };
    const html = renderToStaticMarkup(
      <TokenOverview
        stats={stats}
        onDetails={() => {}}
        onAccounting={() => {}}
      />,
    );
    expect(html).toContain("Model usage · 2 observations · partial");
    expect(html).not.toContain("claude.assistant_usage.v1");
    expect(html).not.toContain("1,234");
    expect(html).toContain("Recorded token activity");
  });
});
