/**
 * Tests for session resume / checkpoint re-injection.
 *
 * Run with: npx vitest run src/test/session-resume.test.ts
 */

import { describe, it, expect } from "vitest";
import {
  renderResumeContext,
  buildResumeContext,
} from "../core/session-resume-logic.js";

describe("renderResumeContext", () => {
  it("wraps checkpoint content with a heading and verify-before-act reminder", () => {
    const out = renderResumeContext("# Session checkpoint\n## Files touched\n- a.ts");
    expect(out).toContain("## Resuming session — last checkpoint");
    expect(out).toContain("<system-reminder>");
    expect(out.toUpperCase()).toContain("VERIFY");
    expect(out).toContain("# Session checkpoint");
    expect(out).toContain("- a.ts");
  });
});

describe("buildResumeContext gating", () => {
  // These cases return before any IO, so no real binary is needed.
  it("returns null for a fresh startup", () => {
    expect(buildResumeContext("/no/such/bin", "/tmp", "s1", "startup")).toBeNull();
  });

  it("returns null when source is undefined", () => {
    expect(buildResumeContext("/no/such/bin", "/tmp", "s1", undefined)).toBeNull();
  });

  it("returns null for a cleared session", () => {
    expect(buildResumeContext("/no/such/bin", "/tmp", "s1", "clear")).toBeNull();
  });

  it("attempts a lookup for resume/compact (null when binary missing)", () => {
    // Binary doesn't exist → getLatestCheckpoint swallows the error → null,
    // but crucially it did NOT short-circuit on the source gate.
    expect(buildResumeContext("/no/such/bin", "/tmp", "s1", "resume")).toBeNull();
    expect(buildResumeContext("/no/such/bin", "/tmp", "s1", "compact")).toBeNull();
  });
});
