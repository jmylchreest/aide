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

describe("usage request contract", () => {
  test("empty batches skip execution", () => {
    exec = spyOn(childProcess, "execFileSync");
    expect(recordModelUsage({ ...request, events: [] })).toEqual({ status: "acknowledged", recorded: 0 });
    expect(exec).not.toHaveBeenCalled();
  });

  test("exact acknowledgments preserve syntax and binary options", () => {
    exec = spyOn(childProcess, "execFileSync");
    for (const output of ["Recorded 1 event(s)\n", "  rEcOrDeD 1 EvEnT(s), SkIpPeD 0 \n"]) {
      exec.mockReturnValue(Buffer.from(output));
      expect(recordModelUsage(request)).toEqual({ status: "acknowledged", recorded: 1 });
    }
    expect(exec).toHaveBeenLastCalledWith("bin", ["observe", "record", "--stdin"], {
      cwd: "/cwd", input: JSON.stringify(events[0]) + "\n", timeout: 10000,
      stdio: ["pipe", "pipe", "pipe"],
    });
    exec.mockReturnValue(Buffer.from("Recorded 2 event(s), skipped 0"));
    expect(recordModelUsage({ ...request, events: [...events, ...events] })).toEqual({ status: "acknowledged", recorded: 2 });
  });

  test("missing, malformed, incomplete and skipped acknowledgments fail explicitly", () => {
    exec = spyOn(childProcess, "execFileSync");
    for (const output of [undefined, null, "", "ok", "Recorded 0 event(s)", "Recorded 2 event(s)", "Recorded 1 event(s), skipped 1", "Recorded 1 event(s) extra"]) {
      exec.mockReturnValue(output as any);
      expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
    }
  });

  test("execution and serialization exceptions return write-error without retries", () => {
    exec = spyOn(childProcess, "execFileSync").mockImplementation(() => { throw new Error("write failed"); });
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
    expect(exec).toHaveBeenCalledTimes(1);
    exec.mockClear();
    const circular: any = { ...events[0] };
    circular.attrs = circular;
    expect(recordModelUsage({ ...request, events: [circular] })).toEqual({ status: "unacknowledged", reason: "write-error" });
    expect(exec).not.toHaveBeenCalled();
  });

  test("recorder retries failures, filters invalid parts and scopes accepted revisions", () => {
    const requests: UsageWriteRequest[] = [];
    const writer = (request: UsageWriteRequest): UsageWriteResult => {
      requests.push(request);
      return requests.length === 1
        ? { status: "unacknowledged", reason: "invalid-ack" }
        : { status: "acknowledged", recorded: request.events.length };
    };
    const record = createOpenCodeUsageRecorder(writer);
    const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 } };
    const input = { binary: "bin", cwd: "/cwd", part };
    expect(record({ ...input, part: null })).toBeUndefined();
    record(input);
    record(input);
    record(input);
    record({ ...input, part: { ...part, tokens: { input: 2 } } });
    record({ ...input, cwd: "/other" });
    createOpenCodeUsageRecorder(writer)(input);
    expect(requests).toHaveLength(5);
    expect(requests[0]).toMatchObject({ binary: "bin", cwd: "/cwd", events: [{ name: "model_usage", attrs: { usage_id: "p", uncached_input_tokens: "1" } }] });
  });

  test("recorder evicts the oldest fingerprint after 1024 entries", () => {
    let writes = 0;
    const record = createOpenCodeUsageRecorder(({ events }) => {
      writes++;
      return { status: "acknowledged", recorded: events.length };
    });
    const input = (id: number) => ({ binary: "bin", cwd: "/cwd", part: { type: "step-finish", id: String(id), sessionID: "s", messageID: "m", tokens: { input: 1 } } });
    for (let i = 0; i < 1024; i++) record(input(i));
    record(input(0));
    expect(writes).toBe(1024);
    record(input(1024));
    record(input(1));
    expect(writes).toBe(1025);
    record(input(0));
    expect(writes).toBe(1026);
  });
});
