import { afterEach, expect, spyOn, test } from "bun:test";
import * as childProcess from "node:child_process";
import {
  createOpenCodeUsageRecorder,
  recordModelUsage,
  type UsageWriteRequest,
  type UsageWriteResult,
} from "../src/core/model-usage.ts";

let restore: (() => void) | undefined;
afterEach(() => { restore?.(); restore = undefined; });
function mockExec() {
  const exec = spyOn(childProcess, "execFileSync");
  restore = () => exec.mockRestore();
  return exec;
}
const events = [{ kind: "session", name: "model_usage", session: "s" }];
const request = { binary: "bin", cwd: "/cwd", events };

test("writer accepts only full acknowledgments and preserves subprocess options", () => {
  const exec = mockExec();
  for (const output of [undefined, null, "", "bad", "Recorded 0 event(s)",
    "Recorded 2 event(s)", "Recorded 1 event(s), skipped 1", "Recorded 1 event(s)\njunk"]) {
    exec.mockReturnValue(output as any);
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
  }
  for (const output of ["Recorded 1 event(s)\n", " \nReCoRdEd 1 EvEnT(s), SkIpPeD 0\t "]) {
    exec.mockReturnValue(Buffer.from(output));
    expect(recordModelUsage(request)).toEqual({ status: "acknowledged", recorded: 1 });
  }
  expect(exec).toHaveBeenLastCalledWith("bin", ["observe", "record", "--stdin"], {
    cwd: "/cwd", input: JSON.stringify(events[0]) + "\n", timeout: 10000,
    stdio: ["pipe", "pipe", "pipe"],
  });
  exec.mockReturnValue(Buffer.from("Recorded 2 event(s)"));
  expect(recordModelUsage({ ...request, events: [...events, ...events] })).toEqual({ status: "acknowledged", recorded: 2 });
});

test("writer skips empty batches and reports write and serialization errors", () => {
  const exec = mockExec();
  expect(recordModelUsage({ ...request, events: [] })).toEqual({ status: "acknowledged", recorded: 0 });
  expect(exec).not.toHaveBeenCalled();
  const circular: any = { ...events[0] };
  circular.attrs = circular;
  expect(recordModelUsage({ ...request, events: [circular] })).toEqual({ status: "unacknowledged", reason: "write-error" });
  expect(exec).not.toHaveBeenCalled();
  exec.mockImplementation(() => { throw new Error("write failed"); });
  expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
  expect(exec).toHaveBeenCalledTimes(1);
});

const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 } };
test("recorder retries unacknowledged results, forwards changes, and scopes its cache", () => {
  const calls: UsageWriteRequest[] = [];
  let result: UsageWriteResult = { status: "unacknowledged", reason: "invalid-ack" };
  const write = (request: UsageWriteRequest): UsageWriteResult => { calls.push(request); return result; };
  const record = createOpenCodeUsageRecorder(write);
  const request = { binary: "bin", cwd: "/cwd", part };
  expect(record({ ...request, part: null })).toBeUndefined();
  expect(calls).toHaveLength(0);
  record(request);
  record(request);
  result = { status: "acknowledged", recorded: 1 };
  record(request);
  record(request);
  record({ ...request, part: { ...part, tokens: { input: 2 } } });
  record({ ...request, cwd: "/other" });
  createOpenCodeUsageRecorder(write)(request);
  expect(calls).toHaveLength(6);
  expect(calls[0]).toMatchObject({ binary: "bin", cwd: "/cwd", events: [{ name: "model_usage", attrs: { uncached_input_tokens: "1" } }] });
});

test("recorder evicts the oldest acknowledged entry after 1024", () => {
  let calls = 0;
  const record = createOpenCodeUsageRecorder(({ events }) => {
    calls++;
    return { status: "acknowledged", recorded: events.length };
  });
  const request = { binary: "bin", cwd: "/cwd", part };
  for (let i = 0; i < 1024; i++) record({ ...request, part: { ...part, id: String(i) } });
  record({ ...request, part: { ...part, id: "0" } });
  expect(calls).toBe(1024);
  record({ ...request, part: { ...part, id: "1024" } });
  record({ ...request, part: { ...part, id: "1" } });
  expect(calls).toBe(1025);
  record({ ...request, part: { ...part, id: "0" } });
  expect(calls).toBe(1026);
});
