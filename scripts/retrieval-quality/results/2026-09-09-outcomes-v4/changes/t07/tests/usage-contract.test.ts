import { afterEach, expect, spyOn, test } from "bun:test";
import * as childProcess from "node:child_process";
import {
  createOpenCodeUsageRecorder,
  recordModelUsage,
  type UsageWriteRequest,
  type UsageWriteResult,
} from "../src/core/model-usage.ts";

let exec: ReturnType<typeof spyOn> | undefined;
afterEach(() => exec?.mockRestore());
const request = {
  binary: "bin",
  cwd: "/cwd",
  events: [{ kind: "session", name: "model_usage", session: "s" }],
};

test("writer reports exact acknowledgments and preserves subprocess options", () => {
  exec = spyOn(childProcess, "execFileSync").mockReturnValue(Buffer.from(""));
  expect(recordModelUsage({ ...request, events: [] })).toEqual({ status: "acknowledged", recorded: 0 });
  expect(exec).not.toHaveBeenCalled();
  for (const output of [undefined, null, Buffer.from(""), Buffer.from("garbage"), Buffer.from("Recorded 0 event(s)"), Buffer.from("Recorded 2 event(s)"), Buffer.from("Recorded 1 event(s), skipped 1")]) {
    exec.mockReturnValue(output);
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
  }
  for (const output of ["Recorded 1 event(s)\n", " \n rEcOrDeD 1 event(s), skipped 0 \n"]) {
    exec.mockReturnValue(Buffer.from(output));
    expect(recordModelUsage(request)).toEqual({ status: "acknowledged", recorded: 1 });
  }
  expect(exec).toHaveBeenLastCalledWith("bin", ["observe", "record", "--stdin"], {
    cwd: "/cwd",
    input: JSON.stringify(request.events[0]) + "\n",
    timeout: 10000,
    stdio: ["pipe", "pipe", "pipe"],
  });
});

test("write and serialization errors are best effort", () => {
  exec = spyOn(childProcess, "execFileSync").mockImplementation(() => { throw new Error("failed"); });
  expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
  exec.mockClear();
  const circular: any = { ...request.events[0] };
  circular.self = circular;
  expect(recordModelUsage({ ...request, events: [circular] })).toEqual({ status: "unacknowledged", reason: "write-error" });
  expect(exec).not.toHaveBeenCalled();
});

const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 } };
test("recorder retries unacknowledged objects and scopes normalized snapshots", () => {
  const calls: UsageWriteRequest[] = [];
  const write = (request: UsageWriteRequest): UsageWriteResult => {
    calls.push(request);
    return calls.length === 1 ? { status: "unacknowledged", reason: "invalid-ack" } : { status: "acknowledged", recorded: request.events.length };
  };
  const record = createOpenCodeUsageRecorder(write);
  const input = { binary: "bin", cwd: "/cwd", part };
  expect(record({ ...input, part: null })).toBeUndefined();
  expect(calls).toHaveLength(0);
  record(input);
  record(input);
  record(input);
  expect(calls).toHaveLength(2);
  expect(calls[0]).toMatchObject({ binary: "bin", cwd: "/cwd", events: [{ name: "model_usage", attrs: { uncached_input_tokens: "1" } }] });
  record({ ...input, part: { ...part, tokens: { input: 2 } } });
  record({ ...input, cwd: "/other" });
  createOpenCodeUsageRecorder(write)(input);
  expect(calls).toHaveLength(5);
});

test("recorder evicts oldest acknowledged entry at 1024", () => {
  let writes = 0;
  const record = createOpenCodeUsageRecorder(({ events }) => {
    writes++;
    return { status: "acknowledged", recorded: events.length };
  });
  const input = { binary: "bin", cwd: "/cwd", part };
  for (let i = 0; i < 1024; i++) record({ ...input, part: { ...part, id: String(i) } });
  record({ ...input, part: { ...part, id: "0" } });
  expect(writes).toBe(1024);
  record({ ...input, part: { ...part, id: "1024" } });
  record({ ...input, part: { ...part, id: "1" } });
  expect(writes).toBe(1025);
  record({ ...input, part: { ...part, id: "0" } });
  expect(writes).toBe(1026);
});
