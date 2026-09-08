import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { TokenAccountingSummary } from "../components/shared/TokenAccountingSummary";

describe("token accounting presentation", () => {
  it.each([
    undefined,
    { bytes: 0, estimated_tokens: 0, events: 1 },
    { bytes: 6, estimated_tokens: 2, events: 1 },
  ])(
    "keeps prepared context separate with missing versus zero: %j",
    (quantity) => {
      const html = renderToStaticMarkup(
        <TokenAccountingSummary
          accounting={{
            version: 1,
            estimator: "utf8-bytes/3-v1",
            by_stage: quantity ? { aide_context: quantity } : {},
            arguments: { bytes: 0, estimated_tokens: 0, events: 0 },
            legacy_events: 0,
            missing_payload: 0,
            missing_identity: 0,
          }}
        />,
      );
      expect(html).toContain("Prepared aide context");
      expect(html).toContain("source excerpts");
      expect(html).toContain("does not confirm final delivery");
      expect(html).toContain(quantity ? `${quantity.bytes} bytes` : "Unknown");
    },
  );
  it("keeps unavailable evidence unknown for older servers", () => {
    const html = renderToStaticMarkup(<TokenAccountingSummary />);
    expect(html).toContain("Accounting unavailable");
    expect(html).not.toContain("0 bytes");
  });
  it("shows server and host observations separately, with coverage and estimator", () => {
    const html = renderToStaticMarkup(
      <TokenAccountingSummary
        accounting={{
          version: 1,
          estimator: "utf8-bytes/3-v1",
          by_stage: {
            host_result: { bytes: 0, estimated_tokens: 0, events: 1 },
            server_result: { bytes: 12, estimated_tokens: 4, events: 2 },
          },
          arguments: { bytes: 0, estimated_tokens: 0, events: 0 },
          legacy_events: 3,
          missing_payload: 2,
          missing_identity: 4,
        }}
      />,
    );
    expect(html).toContain("Host result text");
    expect(html).toContain("Server result text");
    expect(html).toContain("0 bytes");
    expect(html).toContain("12 bytes");
    expect(html).toContain("utf8-bytes/3-v1");
    expect(html).toContain("3 legacy");
    expect(html).toContain("2 missing text");
    expect(html).toContain("4 missing identity");
    expect(html).not.toContain("100%");
  });
});
