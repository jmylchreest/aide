import { afterAll, afterEach, expect, spyOn, test } from "bun:test";
import * as childProcess from "node:child_process";
import {
  createOpenCodeUsageRecorder,
  recordModelUsage,
  type UsageWriteRequest,
  type UsageWriteResult,
} from "../src/core/model-usage.ts";

const exec = spyOn(childProcess, "execFileSync");
afterEach(() => exec.mockReset());
afterAll(() => exec.mockRestore());
const events = [{ kind: "session", name: "model_usage", session: "s" }];
const request = { binary: "bin", cwd: "/cwd", events };

test("empty usage batches succeed without writing", () => {
  expect(recordModelUsage({ ...request, events: [] })).toEqual({
    status: "acknowledged", recorded: 0,
  });
  expect(exec).not.toHaveBeenCalled();
});

test("writer validates acknowledgments and preserves subprocess options", () => {
  for (const output of [undefined, null, "", "oops", "Recorded 0 event(s)",
    "Recorded 2 event(s)", "Recorded 1 event(s), skipped 1"]) {
    exec.mockReturnValue(output as any);
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
  }
  for (const output of ["Recorded 1 event(s)\n", "  rEcOrDeD 1 event(s), skipped 0 \n"]) {
    exec.mockReturnValue(Buffer.from(output));
    expect(recordModelUsage(request)).toEqual({ status: "acknowledged", recorded: 1 });
  }
  expect(exec).toHaveBeenLastCalledWith("bin", ["observe", "record", "--stdin"], {
    cwd: "/cwd", input: JSON.stringify(events[0]) + "\n", timeout: 10000,
    stdio: ["pipe", "pipe", "pipe"],
  });
});

test("write and serialization failures are best effort", () => {
  exec.mockImplementation(() => { throw new Error("write failed"); });
  expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
  expect(exec).toHaveBeenCalledTimes(1);
  exec.mockClear();
  const cyclic: any = { ...events[0] };
  cyclic.self = cyclic;
  expect(recordModelUsage({ ...request, events: [cyclic] })).toEqual({ status: "unacknowledged", reason: "write-error" });
  expect(exec).not.toHaveBeenCalled();
});

const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 } };
test("recorder retries failures and scopes acknowledged snapshots", () => {
  const writes: UsageWriteRequest[] = [];
  const writer = (request: UsageWriteRequest): UsageWriteResult => {
    writes.push(request);
    return writes.length === 1 ? { status: "unacknowledged", reason: "invalid-ack" }
      : { status: "acknowledged", recorded: request.events.length };
  };
  const record = createOpenCodeUsageRecorder(writer);
  const request = { binary: "bin", cwd: "/cwd", part };
  record({ ...request, part: { ...part, type: "text" } });
  expect(writes).toHaveLength(0);
  record(request);
  record(request);
  record(request);
  expect(writes).toHaveLength(2);
  expect(writes[0]).toMatchObject({ binary: "bin", cwd: "/cwd", events: [{ name: "model_usage", session: "s" }] });
  record({ ...request, part: { ...part, tokens: { input: 2 } } });
  record({ ...request, cwd: "/other" });
  createOpenCodeUsageRecorder(writer)(request);
  expect(writes).toHaveLength(5);
});

test("recorder evicts the oldest acknowledgment after 1024 entries", () => {
  const writes: UsageWriteRequest[] = [];
  const record = createOpenCodeUsageRecorder((request) => {
    writes.push(request);
    return { status: "acknowledged", recorded: request.events.length };
  });
  const request = { binary: "bin", cwd: "/cwd", part };
  for (let i = 0; i < 1024; i++) record({ ...request, part: { ...part, id: String(i) } });
  record({ ...request, part: { ...part, id: "0" } });
  expect(writes).toHaveLength(1024);
  record({ ...request, part: { ...part, id: "1024" } });
  record({ ...request, part: { ...part, id: "0" } });
  expect(writes).toHaveLength(1026);
});
