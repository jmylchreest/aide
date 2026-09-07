import { describe, expect, it } from "vitest";
import { SessionPruningTrackers } from "../core/context-pruning/sessions.js";

describe("session pruning continuity", () => {
  const original = "independent output ".repeat(100);
  it("never deduplicates across sessions or missing identities", () => {
    const sessions = new SessionPruningTrackers();
    sessions.process("a", "1", "Grep", { pattern: "value" }, original);
    expect(
      sessions.process("b", "2", "Grep", { pattern: "value" }, original)
        .modified,
    ).toBe(false);
    expect(
      sessions.process("", "3", "Grep", { pattern: "value" }, original)
        .modified,
    ).toBe(false);
    expect(
      sessions.process("a", "4", "Grep", { pattern: "value" }, original)
        .modified,
    ).toBe(true);
  });
  it("suspends on request, retains history on failure, clears only the completed session", () => {
    const sessions = new SessionPruningTrackers();
    for (const id of ["a", "b"])
      sessions.process(id, "1", "Grep", { pattern: "value" }, original);
    sessions.pending("a");
    expect(
      sessions.process("a", "2", "Grep", { pattern: "value" }, original)
        .modified,
    ).toBe(false);
    sessions.failed("a");
    expect(
      sessions.process("a", "3", "Grep", { pattern: "value" }, original)
        .modified,
    ).toBe(true);
    sessions.complete("a");
    expect(
      sessions.process("a", "4", "Grep", { pattern: "value" }, original)
        .modified,
    ).toBe(false);
    expect(
      sessions.process("b", "4", "Grep", { pattern: "value" }, original)
        .modified,
    ).toBe(true);
  });
});
