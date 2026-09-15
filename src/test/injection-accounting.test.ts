import { describe, expect, it, vi } from "vitest";

vi.mock("child_process", () => ({ execFileSync: vi.fn() }));
import { execFileSync } from "child_process";
import {
  injectionBatchEvent,
  emitInjectionEvent,
  recordObserveEventsBatch,
} from "../core/read-tracking.js";

describe("prepared context accounting", () => {
  for (const host of ["claude-code", "codex", "opencode"]) {
    it(`measures exact UTF-8 source content for ${host}, not its preview`, () => {
      const content = "é🦊".repeat(1000);
      const event = injectionBatchEvent({
        source: "test",
        subtype: "memory",
        content,
        sessionId: "s",
        attrs: { host },
      });
      expect(event.tokens).toBeUndefined();
      expect(event.attrs).toMatchObject({
        accounting_version: "1",
        observation_stage: "aide_context",
        content_boundary: "source_text",
        payload_bytes: String(Buffer.byteLength(content, "utf8")),
        host,
      });
      expect(event.attrs?.content_preview?.length).toBeLessThan(content.length);
      expect(event.session).toBe("s");
    });
  }
  it("preserves measured empty content and protects accounting fields from metadata overrides", () => {
    const event = injectionBatchEvent({
      source: "test",
      subtype: "memory",
      content: "",
      attrs: {
        accounting_version: "legacy",
        observation_stage: "host_result",
        payload_bytes: "99",
        content_boundary: "appended_text",
        source_id: "memory-id",
      },
    });
    expect(event.attrs).toMatchObject({
      accounting_version: "1",
      observation_stage: "aide_context",
      payload_bytes: "0",
      content_boundary: "source_text",
      source_id: "memory-id",
    });
    expect(event.session).toBeUndefined();
  });
  it("uses the same measured fields in individual and batch emitters", () => {
    const opts = {
      source: "test",
      subtype: "skill",
      content: "漢字",
      sessionId: "s",
    };
    emitInjectionEvent("aide", "/project", opts);
    expect(execFileSync).toHaveBeenCalledWith(
      "aide",
      expect.arrayContaining([
        "--attr=payload_bytes=6",
        "--attr=observation_stage=aide_context",
      ]),
      expect.any(Object),
    );
    const event = injectionBatchEvent(opts);
    recordObserveEventsBatch("aide", "/project", [event]);
    expect(execFileSync).toHaveBeenLastCalledWith(
      "aide",
      ["observe", "record", "--stdin"],
      expect.objectContaining({ input: JSON.stringify(event) + "\n" }),
    );
  });
});
