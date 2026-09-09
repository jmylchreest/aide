import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
  ModelUsageAccounting,
  supportedModelUsage,
} from "../components/shared/TokenModelUsage";
import { TokenOverview } from "../components/shared/TokenOverview";
import {
  modelInputSplit,
  TokenModelUsageOverview,
} from "../components/shared/TokenModelUsageOverview";
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
    expect(html).toContain("Model tokens");
    expect(html).toContain("Usage details");
    expect(html).not.toContain("claude.assistant_usage.v1");
    expect(html).toContain("1,234");
    expect(html).toContain("1/2 observations");
    expect(html).toContain("Recorded token activity");
  });

  it("charts only complete, matching input components without adding cache twice", () => {
    const row = {
      ...report.by_source[0],
      counters: {
        input_tokens: { tokens: 1000, observations: 2 },
        uncached_input_tokens: { tokens: 200, observations: 2 },
        cache_read_input_tokens: { tokens: 700, observations: 2 },
        cache_write_input_tokens: { tokens: 100, observations: 2 },
      },
    };
    expect(modelInputSplit(row)?.map((p) => p.percent)).toEqual([20, 70, 10]);
    const html = renderToStaticMarkup(
      <TokenModelUsageOverview
        report={{ ...report, by_source: [row] }}
        onAccounting={() => {}}
      />,
    );
    expect(html).toContain(
      "Input breakdown: Uncached 200, Cached 700, Cache write 100",
    );
    expect(html).not.toContain("1,800");
    for (const key of Object.keys(row.counters)) {
      const partial = structuredClone(row);
      partial.counters[key as keyof typeof partial.counters].observations = 1;
      expect(modelInputSplit(partial)).toBeUndefined();
    }
    expect(
      modelInputSplit({
        ...row,
        counters: {
          ...row.counters,
          input_tokens: { tokens: 999, observations: 2 },
        },
      }),
    ).toBeUndefined();
    expect(modelInputSplit(report.by_source[0])).toBeUndefined();
  });

  it("shows OpenCode raw output without inventing reasoning overlap or cross-source totals", () => {
    const usage: ModelUsage = {
      ...report,
      observations: 4,
      by_source: [
        {
          ...report.by_source[0],
          host: "opencode",
          source: "opencode.step_finish.v1",
          counters: {
            reported_output_tokens: { tokens: 7, observations: 2 },
            reasoning_output_tokens: { tokens: 3, observations: 2 },
            cache_read_input_tokens: { tokens: 0, observations: 2 },
          },
        },
        report.by_source[0],
      ],
    };
    const html = renderToStaticMarkup(
      <TokenModelUsageOverview report={usage} onAccounting={() => {}} />,
    );
    expect(html).toContain("Reported output");
    expect(html).toContain("Reasoning overlap unknown");
    expect(html).toContain("Model usage source");
    expect(html).toContain(">0</dd>");
    expect(html).toContain(">Unknown</dd>");
    expect(html).not.toContain(">10</dd>");
    expect(html).not.toContain("1,234");
    expect(html).not.toContain("Input breakdown:");
  });

  it("shows an honest empty state for missing and zero-observation reports", () => {
    expect(
      renderToStaticMarkup(<TokenModelUsageOverview onAccounting={() => {}} />),
    ).toContain("unavailable");
    expect(
      renderToStaticMarkup(
        <TokenModelUsageOverview
          report={{
            version: 1,
            observations: 0,
            conflicts: 0,
            invalid: 0,
            by_source: [],
          }}
          onAccounting={() => {}}
        />,
      ),
    ).toContain("usage is unknown");
  });
});
