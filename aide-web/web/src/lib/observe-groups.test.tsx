import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
  createObserveEventBatcher,
  groupObserveEvents,
  mergeObserveEvents,
} from "./observe-groups";
import { ModelUsageEventGroup } from "../components/shared/ModelUsageEventGroup";
import type { ObserveEventItem } from "./types";

function usage(
  id: string,
  overrides: Partial<ObserveEventItem> = {},
): ObserveEventItem {
  // Shape emitted by src/core/model-usage.ts:event and projected by the web API.
  return {
    id,
    timestamp: "2026-09-12T10:00:01Z",
    kind: "session",
    name: "model_usage",
    session_id: "session-a",
    attrs: {
      model_usage_version: "1",
      host: "claude-code",
      usage_source: "claude.assistant_usage.v1",
      usage_id: id,
      usage_time_basis: "source",
      usage_coverage: "partial",
      usage_source_time: "2026-09-12T10:00:01Z",
    },
    ...overrides,
  };
}

describe("observe usage groups", () => {
  it("recognizes actual session usage events and leaves similarly named hooks alone", () => {
    const rows = groupObserveEvents([
      usage("response-a"),
      usage("response-b"),
      usage("hook-a", { kind: "hook" }),
      usage("hook-b", { kind: "hook" }),
    ]);
    expect(rows.map((row) => row.type)).toEqual(["usage", "event", "event"]);
    expect(rows[0].events.map((event) => event.attrs?.usage_id)).toEqual([
      "response-a",
      "response-b",
    ]);
    expect(rows[0].events.every((event) => event.kind === "session")).toBe(
      true,
    );
  });
  it("batches burst renders, bounds pending records and cancels on query changes", () => {
    vi.useFakeTimers();
    try {
      const publish = vi.fn();
      const batcher = createObserveEventBatcher(publish, 2);
      batcher.push(usage("a"));
      batcher.push(usage("b"));
      batcher.push(usage("c"));
      batcher.push(usage("c", { error: "updated" }));
      expect(publish).not.toHaveBeenCalled();
      vi.advanceTimersByTime(100);
      expect(publish).toHaveBeenCalledTimes(1);
      expect(
        publish.mock.calls[0][0].map((e: ObserveEventItem) => e.id),
      ).toEqual(["c", "b"]);
      expect(publish.mock.calls[0][0][0].error).toBe("updated");
      batcher.push(usage("stale"));
      batcher.cancel();
      vi.advanceTimersByTime(100);
      expect(publish).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
  it("collapses usage bursts while preserving every raw record and other events", () => {
    const events = [
      usage("a"),
      usage("tool", { name: "Read", kind: "tool_call" }),
      usage("b"),
    ];
    const rows = groupObserveEvents(events);
    expect(rows).toHaveLength(2);
    expect(rows[0].events.map((e) => e.id)).toEqual(["a", "b"]);
    expect(rows[1].events).toEqual([events[1]]);
    expect(rows[0].type).toBe("usage");
  });
  it("never groups across session, host, source, context window or minute boundaries", () => {
    const events = [
      usage("a"),
      usage("session", { session_id: "other" }),
      usage("host", {
        attrs: { host: "codex", usage_source: "claude.assistant_usage.v1" },
      }),
      usage("source", {
        attrs: { host: "claude-code", usage_source: "other" },
      }),
      usage("window", {
        attrs: { ...usage("a").attrs, context_window_id: "reset-1" },
      }),
      usage("minute", { timestamp: "2026-09-12T10:01:00Z" }),
    ];
    expect(groupObserveEvents(events)).toHaveLength(events.length);
    expect(
      groupObserveEvents([
        usage("missing", { session_id: undefined }),
        usage("missing2", { session_id: undefined }),
      ]),
    ).toHaveLength(2);
    expect(
      groupObserveEvents([
        usage("bad", { timestamp: "invalid" }),
        usage("bad2", { timestamp: "invalid" }),
      ]),
    ).toHaveLength(2);
  });
  it("deduplicates before bounding and preserves latest changed payload", () => {
    const old = usage("a");
    const changed = usage("a", { error: "conflicting evidence" });
    expect(
      mergeObserveEvents([changed, changed], [old, usage("b")], 2),
    ).toEqual([changed, usage("b")]);
    expect(groupObserveEvents([changed, old])[0].events).toEqual([changed]);
  });
  it("groups only records supplied by the filter, without invented totals", () => {
    const rows = groupObserveEvents(
      [usage("a"), usage("b")].filter((e) => e.id === "b"),
    );
    expect(rows[0].events).toHaveLength(1);
    const html = renderToStaticMarkup(
      <ModelUsageEventGroup
        row={groupObserveEvents([usage("a"), usage("b")])[0]}
      />,
    );
    expect(html).toContain("2 usage records");
    expect(html).toContain("Grouped by event minute");
    expect(html).not.toContain("observation time");
    expect(html.match(/<details/g)).toHaveLength(3);
    expect(html).toContain("&quot;id&quot;: &quot;a&quot;");
    expect(html).toContain("&quot;id&quot;: &quot;b&quot;");
    expect(html).not.toContain("tokens saved");
  });
});
