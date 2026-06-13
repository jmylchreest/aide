/**
 * Tests for the structured session checkpoint builder.
 *
 * Run with: npx vitest run src/test/session-checkpoint.test.ts
 */

import { describe, it, expect } from "vitest";
import { buildSessionCheckpoint } from "../core/session-checkpoint-logic.js";

describe("buildSessionCheckpoint", () => {
  it("returns null when there is no substantive content", () => {
    expect(
      buildSessionCheckpoint({ sessionId: "s1", partials: [], commits: [] }),
    ).toBeNull();
  });

  it("categorises partials into files, commands, and completed work", () => {
    const cp = buildSessionCheckpoint({
      sessionId: "s1",
      partials: [
        "Created file: src/a.ts",
        "Edited file: src/b.ts",
        "Ran command: go build ./...",
        "Completed task: wire up parser",
      ],
      commits: [],
    });
    expect(cp).not.toBeNull();
    expect(cp).toContain("## Files touched");
    expect(cp).toContain("- src/a.ts");
    expect(cp).toContain("- src/b.ts");
    expect(cp).toContain("## Commands run");
    expect(cp).toContain("- go build ./...");
    expect(cp).toContain("## Work completed");
    expect(cp).toContain("- wire up parser");
  });

  it("dedupes files touched across multiple edits", () => {
    const cp = buildSessionCheckpoint({
      sessionId: "s1",
      partials: [
        "Created file: src/a.ts",
        "Edited file: src/a.ts",
        "Edited file: src/a.ts",
      ],
      commits: [],
    });
    expect(cp).not.toBeNull();
    const occurrences = cp!.split("- src/a.ts").length - 1;
    expect(occurrences).toBe(1);
  });

  it("includes the task tree and live resources sections when provided", () => {
    const cp = buildSessionCheckpoint({
      sessionId: "s1",
      partials: ["Ran command: ls"],
      commits: ["abc123 fix: thing"],
      taskTree: ["🔄 implement X [claimed]", "🔵 review Y [pending]"],
      liveState: "branch feat/x · 2 uncommitted file(s)",
    });
    expect(cp).not.toBeNull();
    expect(cp).toContain("## Task state");
    expect(cp).toContain("🔄 implement X [claimed]");
    expect(cp).toContain("## Commits");
    expect(cp).toContain("- abc123 fix: thing");
    expect(cp).toContain("## Live resources");
    expect(cp).toContain("branch feat/x · 2 uncommitted file(s)");
  });

  it("always carries the checkpoint header and resume instruction", () => {
    const cp = buildSessionCheckpoint({
      sessionId: "s1",
      partials: ["Created file: x.ts"],
      commits: [],
    });
    expect(cp).toContain("# Session checkpoint");
    expect(cp!.toLowerCase()).toContain("resume");
  });
});
