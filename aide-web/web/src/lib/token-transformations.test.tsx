import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
  TokenTransformationSummary,
  TokenTransformationWindows,
} from "../components/shared/TokenTransformations";

describe("paired output evidence", () => {
  it("keeps unavailable stages unknown and shows signed overhead", () => {
    const html = renderToStaticMarkup(
      <TokenTransformationSummary
        report={{
          by_stage: {
            adapter_change: {
              before_bytes: 30,
              after_bytes: 90,
              delta_bytes: -60,
              estimated_token_delta: -20,
              events: 1,
            },
          },
          windows: [],
          windows_limited: false,
          unwindowed_events: 0,
          invalid_events: 0,
        }}
      />,
    );
    expect(html).toContain("Adapter changes");
    expect(html).toContain("Proposed rewrites");
    expect(html).toContain("Unknown");
    expect(html).toContain("60 bytes added");
    expect(html).toContain("20 estimated tokens added");
    expect(html).not.toContain("saved");
  });
  it("keeps per-window evidence in a drilldown with missing coverage explicit", () => {
    const html = renderToStaticMarkup(
      <TokenTransformationWindows
        report={{
          by_stage: {},
          windows: [
            {
              host: "opencode",
              session_id: "session",
              actor_id: "actor",
              epoch: "epoch",
              stage: "adapter_change",
              first: "2026-09-07T10:00:00Z",
              last: "2026-09-07T10:01:00Z",
              change: {
                before_bytes: 90,
                after_bytes: 30,
                delta_bytes: 60,
                estimated_token_delta: 20,
                events: 1,
              },
            },
          ],
          windows_limited: true,
          unwindowed_events: 2,
          invalid_events: 1,
        }}
      />,
    );
    expect(html).toContain("epoch");
    expect(html).toContain("actor");
    expect(html).toContain("Additional windows omitted");
    expect(html).toContain("2 without an active window");
  });
});
