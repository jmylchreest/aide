/**
 * Tests for hook output / safety-net helpers.
 *
 * Run with: npx vitest run src/test/hook-utils.test.ts
 */

import { describe, it, expect, vi, afterEach } from "vitest";
import { emitHookResult, installHookSafetyNet } from "../lib/hook-utils.js";

describe("emitHookResult", () => {
  afterEach(() => vi.restoreAllMocks());

  it("defaults to {continue:true}", () => {
    const spy = vi.spyOn(console, "log").mockImplementation(() => {});
    emitHookResult();
    expect(spy).toHaveBeenCalledTimes(1);
    expect(spy).toHaveBeenCalledWith('{"continue":true}');
  });

  it("emits the caller's exact shape, including {}", () => {
    const spy = vi.spyOn(console, "log").mockImplementation(() => {});
    emitHookResult({});
    expect(spy).toHaveBeenCalledWith("{}");
  });

  it("preserves a decision object verbatim", () => {
    const spy = vi.spyOn(console, "log").mockImplementation(() => {});
    emitHookResult({ decision: "block", reason: "busy" });
    expect(spy).toHaveBeenCalledWith('{"decision":"block","reason":"busy"}');
  });
});

describe("installHookSafetyNet", () => {
  afterEach(() => {
    process.removeAllListeners("uncaughtException");
    process.removeAllListeners("unhandledRejection");
  });

  it("registers uncaughtException and unhandledRejection handlers", () => {
    const before = {
      ue: process.listenerCount("uncaughtException"),
      ur: process.listenerCount("unhandledRejection"),
    };
    installHookSafetyNet("test-hook");
    expect(process.listenerCount("uncaughtException")).toBe(before.ue + 1);
    expect(process.listenerCount("unhandledRejection")).toBe(before.ur + 1);
  });
});
