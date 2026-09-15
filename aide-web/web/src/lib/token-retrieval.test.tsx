import { renderToStaticMarkup } from "react-dom/server";
import { describe, it, expect } from "vitest";
import { TokenRetrievalEvidence } from "../components/shared/TokenRetrievalEvidence";

describe("retrieval evidence display", () => {
  it("qualifies partial matches and never turns references into savings", () => {
    const html = renderToStaticMarkup(
      <TokenRetrievalEvidence
        attrs={{
          retrieval_status: "range",
          source_verification: "current_range_match",
          delivered_start_line: "2",
          delivered_end_line: "3",
          source_references: JSON.stringify([
            { file: "test.ts", bytes: 900, sha256: "a".repeat(64) },
          ]),
        }}
      />,
    );
    expect(html).toContain("Lines 2–3 matched");
    expect(html).toContain("Undisplayed source version is unverified");
    expect(html).toContain("900 bytes");
    expect(html).toContain("Conditional reference");
    expect(html).not.toContain("saved");
  });
  it("keeps malformed references and unclassified shell coverage unknown", () => {
    const html = renderToStaticMarkup(
      <TokenRetrievalEvidence
        attrs={{
          retrieval_status: "unclassified_shell",
          source_references: "broken",
        }}
      />,
    );
    expect(html).toContain("Shell retrieval coverage unknown");
    expect(html).not.toContain("0 bytes");
    expect(renderToStaticMarkup(<TokenRetrievalEvidence attrs={{}} />)).toBe(
      "",
    );
  });
});
