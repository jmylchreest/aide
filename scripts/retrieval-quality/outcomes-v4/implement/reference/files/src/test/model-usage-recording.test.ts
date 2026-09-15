import { describe, expect, it, vi } from "vitest";
const exec = vi.hoisted(() => vi.fn());
vi.mock("node:child_process", () => ({ execFileSync: exec }));
import { recordModelUsage } from "../core/model-usage.js";

describe("usage batch acknowledgment", () => {
  const events = [{ kind: "session", name: "model_usage", session: "s" }];
  it("requires a matching explicit recorded count, not merely exit zero", () => {
    for (const response of [
      "",
      "Recorded 0 event(s)",
      "Recorded 1 event(s), skipped 1",
      "Recorded 2 event(s)",
    ]) {
      exec.mockReturnValue(Buffer.from(response));
      expect(recordModelUsage({ binary: "bin", cwd: "/cwd", events })).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
    }
    exec.mockReturnValue(Buffer.from("Recorded 1 event(s)\n"));
    expect(recordModelUsage({ binary: "bin", cwd: "/cwd", events })).toEqual({ status: "acknowledged", recorded: 1 });
  });
});
