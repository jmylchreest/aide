import { afterEach, describe, expect, spyOn, test } from "bun:test";
import * as childProcess from "node:child_process";
import {
  recordModelUsage,
  createOpenCodeUsageRecorder,
  type UsageWriteRequest,
  type UsageWriteResult,
} from "../src/core/model-usage.ts";

const exec = spyOn(childProcess, "execFileSync");
afterEach(() => exec.mockReset());
const request: UsageWriteRequest = {
  binary: "unused-binary",
  cwd: "/cwd",
  events: [{ kind: "session", name: "model_usage", session: "s" }],
};

describe("explicit usage write contract", () => {
  test("empty batches bypass the binary", () => {
    expect(recordModelUsage({ ...request, events: [] })).toEqual({
      status: "acknowledged", recorded: 0,
    });
    expect(exec).not.toHaveBeenCalled();
  });
  test("accepts exact acknowledgments and preserves process options", () => {
    for (const output of ["Recorded 1 event(s)\n", " \nrEcOrDeD 1 event(s), skipped 0\t"]) {
      exec.mockReturnValue(Buffer.from(output));
      expect(recordModelUsage(request)).toEqual({ status: "acknowledged", recorded: 1 });
    }
    expect(exec).toHaveBeenLastCalledWith(request.binary, ["observe", "record", "--stdin"], {
      cwd: request.cwd,
      input: JSON.stringify(request.events[0]) + "\n",
      timeout: 10000,
      stdio: ["pipe", "pipe", "pipe"],
    });
  });
  test("rejects missing, malformed, mismatched and skipped acknowledgments", () => {
    for (const output of [undefined, null, "", "Recorded 0 event(s)", "Recorded 2 event(s)",
      "Recorded 1 event(s), skipped 1", "Recorded 1 event(s) junk"]) {
      exec.mockReturnValue(output as any);
      expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
    }
  });
  test("write and serialization exceptions are best effort failures", () => {
    exec.mockImplementation(() => { throw new Error("write failed"); });
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
    exec.mockClear();
    const circular: any = { ...request.events[0] };
    circular.attrs = circular;
    expect(recordModelUsage({ ...request, events: [circular] })).toEqual({
      status: "unacknowledged", reason: "write-error",
    });
    expect(exec).not.toHaveBeenCalled();
  });
});

const part = { id: "p", sessionID: "s", messageID: "m", type: "step-finish", tokens: { input: 1 } };
test("recorder forwards requests and only caches acknowledgments", () => {
  const calls: UsageWriteRequest[] = [];
  const writer = (request: UsageWriteRequest): UsageWriteResult => {
    calls.push(request);
    return calls.length === 1
      ? { status: "unacknowledged", reason: "invalid-ack" }
      : { status: "acknowledged", recorded: request.events.length };
  };
  const record = createOpenCodeUsageRecorder(writer);
  const input = { binary: "bin", cwd: "/a", part };
  record({ ...input, part: { ...part, type: "text" } });
  expect(calls).toHaveLength(0);
  record(input);
  record(input);
  record(input);
  expect(calls).toHaveLength(2);
  expect(calls[0]).toMatchObject({ binary: "bin", cwd: "/a", events: [{ name: "model_usage", session: "s" }] });
  record({ ...input, part: { ...part, tokens: { input: 2 } } });
  record({ ...input, cwd: "/b" });
  createOpenCodeUsageRecorder(writer)(input);
  expect(calls).toHaveLength(5);
});
test("recorder evicts oldest acknowledgments at 1024 entries", () => {
  let writes = 0;
  const record = createOpenCodeUsageRecorder(({ events }) => {
    writes++;
    return { status: "acknowledged", recorded: events.length };
  });
  const submit = (id: number) => record({ binary: "bin", cwd: "/a", part: { ...part, id: String(id) } });
  for (let id = 0; id < 1024; id++) submit(id);
  submit(0); // Reading an existing entry must not refresh insertion order.
  expect(writes).toBe(1024);
  submit(1024);
  submit(0);
  expect(writes).toBe(1026);
});
