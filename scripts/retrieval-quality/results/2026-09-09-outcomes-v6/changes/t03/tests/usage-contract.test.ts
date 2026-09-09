import { afterEach, describe, expect, spyOn, test } from "bun:test";
import * as childProcess from "node:child_process";
import { createOpenCodeUsageRecorder, openCodeUsageEvent, recordModelUsage,
  type UsageWriteRequest, type UsageWriteResult } from "../src/core/model-usage.ts";

let exec: ReturnType<typeof spyOn> | undefined;
afterEach(() => { exec?.mockRestore(); exec = undefined; });
const events = [{ kind: "session", name: "model_usage", session: "s" }];
const request = { binary: "bin", cwd: "/cwd", events };
const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 } };

describe("explicit usage write contract", () => {
  test("empty batches acknowledge without invoking the binary", () => {
    exec = spyOn(childProcess, "execFileSync");
    expect(recordModelUsage({ ...request, events: [] })).toEqual({ status: "acknowledged", recorded: 0 });
    expect(exec).not.toHaveBeenCalled();
  });
  test("full acknowledgments retain syntax and subprocess options", () => {
    exec = spyOn(childProcess, "execFileSync");
    const batch = { ...request, events: [...events, ...events] };
    for (const output of ["Recorded 2 event(s)\n", " \tRECORDED 2 EVENT(S), SKIPPED 0\n "]) {
      exec.mockReturnValue(Buffer.from(output));
      expect(recordModelUsage(batch)).toEqual({ status: "acknowledged", recorded: 2 });
    }
    expect(exec).toHaveBeenLastCalledWith("bin", ["observe", "record", "--stdin"], {
      cwd: "/cwd", input: batch.events.map(e => JSON.stringify(e)).join("\n") + "\n",
      timeout: 10000, stdio: ["pipe", "pipe", "pipe"],
    });
  });
  test("missing, malformed, partial and skipped acknowledgments are invalid", () => {
    exec = spyOn(childProcess, "execFileSync");
    for (const output of [undefined, null, "", "ok", "Recorded 0 event(s)", "Recorded 2 event(s)",
      "Recorded 1 event(s), skipped 1", "Recorded 1 event(s) extra"]) {
      exec.mockReturnValue(output);
      expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
    }
  });
  test("write and serialization exceptions return write-error without retries", () => {
    exec = spyOn(childProcess, "execFileSync").mockImplementation(() => { throw new Error("failed"); });
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
    expect(exec).toHaveBeenCalledTimes(1);
    exec.mockClear();
    const circular = { ...events[0], attrs: {} as any };
    circular.attrs.self = circular;
    expect(recordModelUsage({ ...request, events: [circular] })).toEqual({ status: "unacknowledged", reason: "write-error" });
    expect(exec).not.toHaveBeenCalled();
  });
  test("recorder retries both failure results, filters parts and scopes cached snapshots", () => {
    const calls: UsageWriteRequest[] = [];
    const write = (req: UsageWriteRequest): UsageWriteResult => {
      calls.push(req);
      if (calls.length < 3) return { status: "unacknowledged", reason: calls.length === 1 ? "invalid-ack" : "write-error" };
      return { status: "acknowledged", recorded: req.events.length };
    };
    const record = createOpenCodeUsageRecorder(write);
    const req = { binary: "bin", cwd: "/cwd", part };
    expect(record({ ...req, part: null })).toBeUndefined();
    record({ ...req, part: { ...part, sessionID: "unknown" } });
    expect(calls).toHaveLength(0);
    for (let i = 0; i < 4; i++) expect(record(req)).toBeUndefined();
    expect(calls).toHaveLength(3);
    expect(calls[0]).toEqual({ binary: "bin", cwd: "/cwd", events: [openCodeUsageEvent(part)] });
    record({ ...req, part: { ...part, tokens: { input: 2 } } });
    record({ ...req, cwd: "/other" });
    createOpenCodeUsageRecorder(write)(req);
    expect(calls).toHaveLength(6);
  });
  test("recorder evicts the oldest fingerprint after 1024 entries", () => {
    let writes = 0;
    const record = createOpenCodeUsageRecorder(({ events }) => {
      writes++;
      return { status: "acknowledged", recorded: events.length };
    });
    const recordId = (id: string) => record({ binary: "bin", cwd: "/cwd", part: { ...part, id } });
    for (let i = 0; i < 1024; i++) recordId(String(i));
    recordId("0");
    expect(writes).toBe(1024);
    recordId("1024");
    recordId("1");
    expect(writes).toBe(1025);
    recordId("0");
    expect(writes).toBe(1026);
  });
});
