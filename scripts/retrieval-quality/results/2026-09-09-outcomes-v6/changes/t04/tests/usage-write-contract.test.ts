import { afterEach, describe, expect, spyOn, test } from "bun:test";
import * as childProcess from "node:child_process";
import {
  createOpenCodeUsageRecorder,
  recordModelUsage,
  type UsageWriteRequest,
  type UsageWriteResult,
} from "../src/core/model-usage.ts";

const events = [{ kind: "session", name: "model_usage", session: "s" }];
const request = { binary: "bin", cwd: "/cwd", events };
let exec: ReturnType<typeof spyOn> | undefined;
afterEach(() => { exec?.mockRestore(); exec = undefined; });
function output(value: unknown) {
  exec = spyOn(childProcess, "execFileSync").mockReturnValue(value as Buffer);
}

describe("usage writer request/result contract", () => {
  test("empty batches avoid executing the binary", () => {
    output(undefined);
    expect(recordModelUsage({ ...request, events: [] })).toEqual({ status: "acknowledged", recorded: 0 });
    expect(exec).not.toHaveBeenCalled();
  });
  test("acknowledges case-insensitive full counts with whitespace and explicit skipped zero", () => {
    for (const ack of ["Recorded 1 event(s)\n", " \trEcOrDeD 1 EvEnT(s), SkIpPeD 0 \n"]) {
      output(Buffer.from(ack));
      expect(recordModelUsage(request)).toEqual({ status: "acknowledged", recorded: 1 });
      expect(exec).toHaveBeenCalledWith("bin", ["observe", "record", "--stdin"], {
        cwd: "/cwd", input: JSON.stringify(events[0]) + "\n", timeout: 10000,
        stdio: ["pipe", "pipe", "pipe"],
      });
      exec!.mockRestore();
    }
  });
  test("rejects missing, malformed, partial and skipped acknowledgments", () => {
    for (const ack of [undefined, null, "", "Recorded 0 event(s)", "Recorded 2 event(s)",
      "Recorded 1 event(s), skipped 1", "Recorded 1 event(s) junk", "Recorded 1 event(s)\nRecorded 1 event(s)"]) {
      output(ack);
      expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
      exec!.mockRestore();
    }
  });
  test("acknowledges the complete JSONL batch count", () => {
    output(Buffer.from("Recorded 2 event(s)"));
    expect(recordModelUsage({ ...request, events: [...events, ...events] })).toEqual({ status: "acknowledged", recorded: 2 });
    expect(exec!.mock.calls[0][2].input).toBe((JSON.stringify(events[0]) + "\n").repeat(2));
  });
  test("reports write and serialization errors without retry", () => {
    output(undefined);
    exec!.mockImplementation(() => { throw new Error("write failed"); });
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
    expect(exec).toHaveBeenCalledTimes(1);
    exec!.mockClear();
    const cyclic: any = { ...events[0] };
    cyclic.attrs = cyclic;
    expect(recordModelUsage({ ...request, events: [cyclic] })).toEqual({ status: "unacknowledged", reason: "write-error" });
    expect(exec).not.toHaveBeenCalled();
  });
});

const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 } };
describe("OpenCode recorder request/result contract", () => {
  test("retries unacknowledged results, filters invalid parts, and forwards changed counters and cwd", () => {
    const calls: UsageWriteRequest[] = [];
    const record = createOpenCodeUsageRecorder((request): UsageWriteResult => {
      calls.push(request);
      return calls.length < 3
        ? { status: "unacknowledged", reason: calls.length === 1 ? "invalid-ack" : "write-error" }
        : { status: "acknowledged", recorded: request.events.length };
    });
    record({ binary: "bin", cwd: "/cwd", part: null });
    for (let i = 0; i < 4; i++) expect(record({ binary: "bin", cwd: "/cwd", part })).toBeUndefined();
    record({ binary: "bin", cwd: "/cwd", part: { ...part, tokens: { input: 2 } } });
    record({ binary: "bin", cwd: "/other", part });
    expect(calls).toHaveLength(5);
    expect(calls[0]).toMatchObject({ binary: "bin", cwd: "/cwd", events: [{ attrs: { uncached_input_tokens: "1" } }] });
    expect(calls[3].events[0].attrs?.uncached_input_tokens).toBe("2");
    expect(calls[4].cwd).toBe("/other");
  });
  test("keeps independent recorder caches and evicts the oldest entry at 1024", () => {
    const calls: UsageWriteRequest[] = [];
    const write = (request: UsageWriteRequest): UsageWriteResult => {
      calls.push(request);
      return { status: "acknowledged", recorded: request.events.length };
    };
    const record = createOpenCodeUsageRecorder(write);
    const send = (id: string) => record({ binary: "bin", cwd: "/cwd", part: { ...part, id } });
    for (let i = 0; i < 1024; i++) send(String(i));
    send("0");
    expect(calls).toHaveLength(1024);
    send("1024");
    send("1");
    expect(calls).toHaveLength(1025);
    send("0");
    expect(calls).toHaveLength(1026);
    createOpenCodeUsageRecorder(write)({ binary: "bin", cwd: "/cwd", part: { ...part, id: "0" } });
    expect(calls).toHaveLength(1027);
  });
});
