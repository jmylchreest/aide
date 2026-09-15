import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { TokenRetrievalWindows } from "../components/shared/TokenRetrievalWindows";
import type { RetrievalWindow } from "./types";

const window: RetrievalWindow = {
  host: "host",
  session_id: "session",
  actor_id: "actor",
  epoch: "window",
  first: "2026-09-08T12:00:00Z",
  last: "2026-09-08T12:01:00Z",
  boundary: "open_or_unknown",
  events: 2,
  observed: { bytes: 390, estimated_tokens: 130, events: 2 },
  unattributed: { bytes: 0, estimated_tokens: 0, events: 0 },
  reference: { bytes: 300, estimated_tokens: 100, events: 1 },
  comparison: {
    before_bytes: 300,
    after_bytes: 390,
    delta_bytes: -90,
    estimated_token_delta: -30,
    events: 2,
  },
  full_read_events: 1,
  search_events: 0,
  failed_events: 0,
  missing_payload: 0,
  clipped: false,
  issues: [],
  sources: [{ file: "file.ts", sha256: "a".repeat(64), bytes: 300 }],
  steps: [],
  steps_limited: false,
};
describe("retrieval window comparisons", () => {
  it("shows signed overhead as conditional and keeps evidence behind disclosure", () => {
    const html = renderToStaticMarkup(
      <TokenRetrievalWindows
        report={{
          windows: [window],
          windows_limited: false,
          unwindowed_events: 0,
        }}
      />,
    );
    expect(html).toContain("~30 more tokens vs reference");
    expect(html).toContain("Conditional comparison");
    expect(html).toContain("<details");
    expect(html).not.toContain("<details open");
    expect(html).toContain("Full-file reference");
    expect(html).not.toContain("tokens saved");
  });
  it("does not render absent comparisons as zero or invent fallback avoidance", () => {
    const html = renderToStaticMarkup(
      <TokenRetrievalWindows
        report={{
          windows: [
            {
              ...window,
              comparison: undefined,
              full_read_events: 0,
              issues: ["filtered_window"],
              clipped: true,
            },
          ],
          windows_limited: true,
          unwindowed_events: 3,
        }}
      />,
    );
    expect(html).toContain("Comparison unavailable");
    expect(html).not.toContain("~0");
    expect(html).toContain("Date filter clips this window");
    expect(html).toContain("coverage is partial");
    expect(html).toContain("64 windows");
  });
});
