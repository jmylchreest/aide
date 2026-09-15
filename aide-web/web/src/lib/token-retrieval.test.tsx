import { renderToStaticMarkup } from "react-dom/server";
import { describe, it, expect } from "vitest";
import { TokenRetrievalEvidence } from "../components/shared/TokenRetrievalEvidence";

describe("retrieval evidence display", () => {
  it.each([
    [
      "full_file",
      "current_rendered_file_match",
      "Rendered full-file content matched",
    ],
    ["range", "current_rendered_range_match", "Rendered source range matched"],
  ])(
    "explains rendered %s verification without equating source and payload bytes",
    (status, verification, label) => {
      const html = renderToStaticMarkup(
        <TokenRetrievalEvidence
          attrs={{
            retrieval_status: status,
            source_verification: verification,
            delivered_start_line: "2",
            delivered_end_line: "3",
          }}
        />,
      );
      expect(html).toContain(label);
      expect(html).toContain("source bytes and delivered bytes may differ");
      expect(html.includes("Lines 2–3 matched")).toBe(status === "range");
      expect(html.includes("Undisplayed source version is unverified")).toBe(
        status === "range",
      );
      expect(html).not.toContain("saved");
    },
  );
  it("does not promote unverified delivery based on a rendered label alone", () => {
    const html = renderToStaticMarkup(
      <TokenRetrievalEvidence
        attrs={{
          retrieval_status: "unverified",
          source_verification: "current_rendered_file_match",
        }}
      />,
    );
    expect(html).toContain("Source delivery unverified");
    expect(html).not.toContain("Rendered content was verified");
  });
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
