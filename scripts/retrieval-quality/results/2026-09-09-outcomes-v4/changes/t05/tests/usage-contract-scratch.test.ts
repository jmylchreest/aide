import { describe, expect, spyOn, test } from "bun:test";
import * as childProcess from "node:child_process";
import { recordModelUsage, createOpenCodeUsageRecorder } from "../src/core/model-usage.ts";
import type { UsageWriteRequest, UsageWriteResult } from "../src/core/model-usage.ts";

const events = [{ kind: "session", name: "model_usage", session: "s" }];
const part = { type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 } };

describe("usage request/result contract", () => {
  test("empty batches acknowledge without invoking the binary", () => {
    expect(recordModelUsage({ binary: "/does-not-exist", cwd: process.cwd(), events: [] }))
      .toEqual({ status: "acknowledged", recorded: 0 });
  });

  test("accepts only full acknowledgments and preserves JSONL and cwd", () => {
    const exec = spyOn(childProcess, "execFileSync");
    const cwd = "/cwd", binary = "bin";
    try {
      for (const [response, acknowledged] of [
        ["Recorded 1 event(s)\n", true],
        ["  rEcOrDeD 1 EvEnT(s), skipped 0 \n", true],
        ["", false], ["bad", false], ["Recorded 0 event(s)", false],
        ["Recorded 2 event(s)", false], ["Recorded 1 event(s), skipped 1", false],
        ["Recorded 1 event(s)\nextra", false],
      ] as const) {
        exec.mockReturnValue(Buffer.from(response));
        expect(recordModelUsage({ binary, cwd, events })).toEqual(acknowledged
          ? { status: "acknowledged", recorded: 1 }
          : { status: "unacknowledged", reason: "invalid-ack" });
        expect(exec).toHaveBeenLastCalledWith(binary, ["observe", "record", "--stdin"], {
          cwd, input: JSON.stringify(events[0]) + "\n", timeout: 10000,
          stdio: ["pipe", "pipe", "pipe"],
        });
      }
      exec.mockReturnValue(undefined as any);
      expect(recordModelUsage({ binary, cwd, events }))
        .toEqual({ status: "unacknowledged", reason: "invalid-ack" });
    } finally { exec.mockRestore(); }
  });

  test("write and serialization exceptions report write-error", () => {
    const exec = spyOn(childProcess, "execFileSync");
    try {
      exec.mockImplementation(() => { throw new Error("write failed"); });
      const request = { binary: "bin", cwd: process.cwd(), events };
      expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
      exec.mockClear();
      exec.mockReturnValue(Buffer.from("Recorded 1 event(s)"));
      const cyclic: any = { ...events[0] };
      cyclic.self = cyclic;
      expect(recordModelUsage({ ...request, events: [cyclic] }))
        .toEqual({ status: "unacknowledged", reason: "write-error" });
      expect(exec).not.toHaveBeenCalled();
    } finally { exec.mockRestore(); }
  });

  test("both unacknowledged results retry and changed counters forward", () => {
    for (const reason of ["invalid-ack", "write-error"] as const) {
      const requests: UsageWriteRequest[] = [];
      const record = createOpenCodeUsageRecorder((request): UsageWriteResult => {
        requests.push(request);
        return requests.length === 1 ? { status: "unacknowledged", reason }
          : { status: "acknowledged", recorded: request.events.length };
      });
      const request = { binary: "bin", cwd: "/cwd", part };
      record(request); record(request); record(request);
      record({ ...request, part: { ...part, tokens: { input: 2 } } });
      expect(requests).toHaveLength(3);
      expect(requests[0]).toMatchObject({ binary: "bin", cwd: "/cwd", events: [{ name: "model_usage", attrs: { uncached_input_tokens: "1" } }] });
    }
  });

  test("filters invalid parts, scopes cwd and keeps instances independent", () => {
    const requests: UsageWriteRequest[] = [];
    const write = (request: UsageWriteRequest): UsageWriteResult => {
      requests.push(request);
      return { status: "acknowledged", recorded: request.events.length };
    };
    const record = createOpenCodeUsageRecorder(write);
    const request = { binary: "bin", cwd: "/first", part };
    record({ ...request, part: null });
    record({ ...request, part: { ...part, type: "text" } });
    expect(requests).toHaveLength(0);
    record(request); record(request);
    record({ ...request, cwd: "/second" });
    createOpenCodeUsageRecorder(write)(request);
    expect(requests).toHaveLength(3);
  });

  test("evicts oldest acknowledgments at 1,024 without refreshing duplicates", () => {
    let calls = 0;
    const record = createOpenCodeUsageRecorder(({ events }) => {
      calls++;
      return { status: "acknowledged", recorded: events.length };
    });
    const send = (id: number) => record({ binary: "bin", cwd: "/cwd", part: { ...part, id: String(id) } });
    for (let i = 0; i < 1024; i++) send(i);
    send(0);
    expect(calls).toBe(1024);
    send(1024); send(1);
    expect(calls).toBe(1025);
    send(0);
    expect(calls).toBe(1026);
  });
});
