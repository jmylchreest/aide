import { beforeEach, expect, mock, test } from "bun:test";
import type { UsageWriteRequest, UsageWriteResult } from "../src/core/model-usage.js";

const exec = mock((): unknown => Buffer.from("Recorded 1 event(s)"));
mock.module("node:child_process", () => ({ execFileSync: exec }));
mock.module("../src/lib/logger.js", () => ({ debug: () => {} }));
const { recordModelUsage, createOpenCodeUsageRecorder } = await import("../src/core/model-usage.js");
const request = {
  binary: "/fake-aide",
  cwd: "/cwd",
  events: [{ kind: "session", name: "model_usage", session: "s" }],
};
beforeEach(() => {
  exec.mockReset();
  exec.mockReturnValue(Buffer.from("Recorded 1 event(s)"));
});

test("empty batch skips execution and successful writes preserve JSONL/options", () => {
  expect(recordModelUsage({ ...request, events: [] })).toEqual({ status: "acknowledged", recorded: 0 });
  expect(exec).not.toHaveBeenCalled();
  exec.mockReturnValue(Buffer.from(" \nREcorded 1 event(s), skipped 0\t\n"));
  expect(recordModelUsage(request)).toEqual({ status: "acknowledged", recorded: 1 });
  expect(exec).toHaveBeenCalledWith("/fake-aide", ["observe", "record", "--stdin"], {
    cwd: "/cwd", input: JSON.stringify(request.events[0]) + "\n", timeout: 10000,
    stdio: ["pipe", "pipe", "pipe"],
  });
});

test("only an exact full batch acknowledgment succeeds", () => {
  for (const response of [undefined, null, "", "bad", "Recorded 0 event(s)", "Recorded 2 event(s)", "Recorded 1 event(s), skipped 1", "Recorded 1 event(s) extra"]) {
    exec.mockReturnValue(response);
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
  }
  const events = [...request.events, ...request.events];
  exec.mockReturnValue(Buffer.from("Recorded 2 event(s)"));
  expect(recordModelUsage({ ...request, events })).toEqual({ status: "acknowledged", recorded: 2 });
});

test("write and serialization exceptions are best effort failures", () => {
  exec.mockImplementation(() => { throw new Error("write failed"); });
  expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
  expect(exec).toHaveBeenCalledTimes(1);
  exec.mockClear();
  const event = { ...request.events[0], toJSON() { throw new Error("serialize failed"); } };
  expect(recordModelUsage({ ...request, events: [event] })).toEqual({ status: "unacknowledged", reason: "write-error" });
  expect(exec).not.toHaveBeenCalled();
});

const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 } };
test("recorder retries both failure results and scopes acknowledged snapshots", () => {
  const write = mock(({ events }: UsageWriteRequest): UsageWriteResult => ({ status: "acknowledged", recorded: events.length }));
  write.mockReturnValueOnce({ status: "unacknowledged", reason: "invalid-ack" });
  write.mockReturnValueOnce({ status: "unacknowledged", reason: "write-error" });
  const record = createOpenCodeUsageRecorder(write);
  const input = { binary: "/fake-aide", cwd: "/cwd", part };
  record({ ...input, part: { type: "text" } });
  expect(write).not.toHaveBeenCalled();
  for (let i = 0; i < 4; i++) expect(record(input)).toBeUndefined();
  expect(write).toHaveBeenCalledTimes(3);
  expect(write.mock.calls[0][0]).toMatchObject({ binary: "/fake-aide", cwd: "/cwd", events: [{ name: "model_usage", session: "s", attrs: { uncached_input_tokens: "1" } }] });
  record({ ...input, part: { ...part, tokens: { input: 2 } } });
  record({ ...input, cwd: "/other" });
  createOpenCodeUsageRecorder(write)(input);
  expect(write).toHaveBeenCalledTimes(6);
});

test("recorder evicts the oldest of 1024 acknowledged fingerprints", () => {
  const write = mock(({ events }: UsageWriteRequest): UsageWriteResult => ({ status: "acknowledged", recorded: events.length }));
  const record = createOpenCodeUsageRecorder(write);
  const input = { binary: "/fake-aide", cwd: "/cwd", part };
  record(input);
  for (let i = 1; i < 1024; i++) record({ ...input, part: { ...part, id: String(i) } });
  record(input); // a repeat does not refresh insertion order
  expect(write).toHaveBeenCalledTimes(1024);
  record({ ...input, part: { ...part, id: "new" } });
  record({ ...input, part: { ...part, id: "1" } });
  expect(write).toHaveBeenCalledTimes(1025);
  record(input);
  expect(write).toHaveBeenCalledTimes(1026);
});
