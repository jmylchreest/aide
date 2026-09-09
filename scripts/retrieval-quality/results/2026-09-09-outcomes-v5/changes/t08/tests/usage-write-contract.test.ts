import { afterEach, describe, expect, spyOn, test } from "bun:test";
import * as childProcess from "node:child_process";
import {
  recordModelUsage,
  createOpenCodeUsageRecorder,
  type UsageWriteRequest,
  type UsageWriteResult,
} from "../src/core/model-usage.ts";

const events = [{ kind: "session", name: "model_usage", session: "s" }];
const request = { binary: "fake-binary", cwd: "/cwd", events };
let exec: ReturnType<typeof spyOn> | undefined;
afterEach(() => exec?.mockRestore());

describe("usage writer request/result contract", () => {
  test("empty batch needs no process", () => {
    exec = spyOn(childProcess, "execFileSync").mockImplementation(() => {
      throw new Error("must not execute");
    });
    expect(recordModelUsage({ ...request, events: [] })).toEqual({
      status: "acknowledged", recorded: 0,
    });
    expect(exec).not.toHaveBeenCalled();
  });

  test("requires full acknowledgment and preserves process options and JSONL", () => {
    exec = spyOn(childProcess, "execFileSync");
    for (const response of [undefined, null, "", "garbage", "Recorded 0 event(s)",
      "Recorded 2 event(s)", "Recorded 1 event(s), skipped 1"] ) {
      exec.mockReturnValue(response);
      expect(recordModelUsage(request)).toEqual({
        status: "unacknowledged", reason: "invalid-ack",
      });
    }
    for (const response of ["Recorded 1 event(s)", " \nReCoRdEd 1 EvEnT(s), SkIpPeD 0\n "]) {
      exec.mockReturnValue(Buffer.from(response));
      expect(recordModelUsage(request)).toEqual({ status: "acknowledged", recorded: 1 });
    }
    expect(exec).toHaveBeenLastCalledWith("fake-binary", ["observe", "record", "--stdin"], {
      cwd: "/cwd", input: JSON.stringify(events[0]) + "\n",
      timeout: 10000, stdio: ["pipe", "pipe", "pipe"],
    });
    const batch = [events[0], { ...events[0], session: "s2" }];
    exec.mockReturnValue(Buffer.from("Recorded 2 event(s), skipped 0"));
    expect(recordModelUsage({ ...request, events: batch })).toEqual({
      status: "acknowledged", recorded: 2,
    });
    expect(exec.mock.calls.at(-1)?.[2].input).toBe(batch.map(e => JSON.stringify(e)).join("\n") + "\n");
  });

  test("write and serialization exceptions return write-error without retries", () => {
    exec = spyOn(childProcess, "execFileSync").mockImplementation(() => {
      throw new Error("write failed");
    });
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
    expect(exec).toHaveBeenCalledTimes(1);
    const circular = { ...events[0] } as typeof events[0] & { self?: unknown };
    circular.self = circular;
    expect(recordModelUsage({ ...request, events: [circular] })).toEqual({
      status: "unacknowledged", reason: "write-error",
    });
    expect(exec).toHaveBeenCalledTimes(1);
  });
});

const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 } };
test("recorder retries unacknowledged writes, filters invalid parts, and scopes its cache", () => {
  const calls: UsageWriteRequest[] = [];
  const writer = (request: UsageWriteRequest): UsageWriteResult => {
    calls.push(request);
    return calls.length === 1
      ? { status: "unacknowledged", reason: "invalid-ack" }
      : { status: "acknowledged", recorded: request.events.length };
  };
  const record = createOpenCodeUsageRecorder(writer);
  const input = { binary: "bin", cwd: "/cwd", part };
  expect(record({ ...input, part: null })).toBeUndefined();
  expect(calls).toHaveLength(0);
  record(input);
  record(input);
  record(input);
  expect(calls).toHaveLength(2);
  expect(calls[0]).toMatchObject({ binary: "bin", cwd: "/cwd", events: [{ name: "model_usage", session: "s" }] });
  record({ ...input, part: { ...part, tokens: { input: 2 } } });
  record({ ...input, cwd: "/other" });
  createOpenCodeUsageRecorder(writer)(input);
  expect(calls).toHaveLength(5);
});

test("recorder evicts the oldest fingerprint after 1024 acknowledgments", () => {
  const calls: UsageWriteRequest[] = [];
  const record = createOpenCodeUsageRecorder((request) => {
    calls.push(request);
    return { status: "acknowledged", recorded: request.events.length };
  });
  const input = { binary: "bin", cwd: "/cwd", part };
  for (let i = 0; i < 1024; i++) record({ ...input, part: { ...part, id: String(i) } });
  record({ ...input, part: { ...part, id: "0" } });
  expect(calls).toHaveLength(1024);
  record({ ...input, part: { ...part, id: "1024" } });
  record({ ...input, part: { ...part, id: "1" } });
  expect(calls).toHaveLength(1025);
  record({ ...input, part: { ...part, id: "0" } });
  expect(calls).toHaveLength(1026);
});
