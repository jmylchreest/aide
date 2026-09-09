import { afterEach, describe, expect, spyOn, test } from "bun:test";
import * as childProcess from "node:child_process";
import {
  createOpenCodeUsageRecorder,
  recordModelUsage,
  type UsageWriteRequest,
  type UsageWriteResult,
} from "../src/core/model-usage.ts";

const events = [{ kind: "session", name: "model_usage", session: "s" }];
const request = { binary: "unused-test-binary", cwd: "/cwd", events };
let exec: ReturnType<typeof spyOn> | undefined;
afterEach(() => {
  exec?.mockRestore();
  exec = undefined;
});

describe("usage write request/result contract", () => {
  test("empty batches do not invoke the binary", () => {
    exec = spyOn(childProcess, "execFileSync").mockImplementation(() => {
      throw new Error("must not execute");
    });
    expect(recordModelUsage({ ...request, events: [] })).toEqual({
      status: "acknowledged", recorded: 0,
    });
    expect(exec).not.toHaveBeenCalled();
  });

  test("accepts only complete acknowledgments and preserves invocation options", () => {
    exec = spyOn(childProcess, "execFileSync");
    for (const response of ["Recorded 1 event(s)\n", " \tRECORDED 1 EVENT(S), SKIPPED 0\n"]) {
      exec.mockReturnValue(Buffer.from(response));
      expect(recordModelUsage(request)).toEqual({ status: "acknowledged", recorded: 1 });
    }
    expect(exec).toHaveBeenCalledWith(request.binary, ["observe", "record", "--stdin"], {
      cwd: "/cwd", input: JSON.stringify(events[0]) + "\n", timeout: 10000,
      stdio: ["pipe", "pipe", "pipe"],
    });
    for (const response of [undefined, null, "", "garbage", "Recorded 0 event(s)",
      "Recorded 2 event(s)", "Recorded 1 event(s), skipped 1", "Recorded 1 event(s) extra"]) {
      exec.mockReturnValue(response == null ? response : Buffer.from(response));
      expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "invalid-ack" });
    }
    exec.mockReturnValue(Buffer.from("Recorded 2 event(s), skipped 0"));
    expect(recordModelUsage({ ...request, events: [...events, ...events] })).toEqual({
      status: "acknowledged", recorded: 2,
    });
    expect(exec).toHaveBeenLastCalledWith(request.binary, ["observe", "record", "--stdin"], {
      cwd: "/cwd", input: JSON.stringify(events[0]) + "\n" + JSON.stringify(events[0]) + "\n",
      timeout: 10000, stdio: ["pipe", "pipe", "pipe"],
    });
  });

  test("classifies execution and serialization failures without retrying", () => {
    exec = spyOn(childProcess, "execFileSync").mockImplementation(() => { throw new Error("write failed"); });
    expect(recordModelUsage(request)).toEqual({ status: "unacknowledged", reason: "write-error" });
    expect(exec).toHaveBeenCalledTimes(1);
    const cyclic = { ...events[0], attrs: {} as Record<string, any> };
    cyclic.attrs.self = cyclic;
    expect(recordModelUsage({ ...request, events: [cyclic] })).toEqual({ status: "unacknowledged", reason: "write-error" });
    expect(exec).toHaveBeenCalledTimes(1);
  });
});

const part = {
  type: "step-finish", id: "p", sessionID: "s", messageID: "m", tokens: { input: 1 },
};

describe("OpenCode acknowledgment cache", () => {
  test("failed results retry; changed counters and cwd are forwarded; instances are independent", () => {
    const requests: UsageWriteRequest[] = [];
    const write = (request: UsageWriteRequest): UsageWriteResult => {
      requests.push(request);
      return requests.length <= 2
        ? { status: "unacknowledged", reason: requests.length === 1 ? "invalid-ack" : "write-error" }
        : { status: "acknowledged", recorded: 1 };
    };
    const record = createOpenCodeUsageRecorder(write);
    const input = { binary: "bin", cwd: "/cwd", part };
    expect(record({ ...input, part: { ...part, type: "text" } })).toBeUndefined();
    for (let i = 0; i < 4; i++) expect(record(input)).toBeUndefined();
    expect(requests).toHaveLength(3);
    expect(requests[0]).toMatchObject({ binary: "bin", cwd: "/cwd", events: [{
      name: "model_usage", session: "s", attrs: { usage_id: "p", uncached_input_tokens: "1" },
    }] });
    record({ ...input, part: { ...part, tokens: { input: 2 } } });
    record({ ...input, cwd: "/elsewhere" });
    createOpenCodeUsageRecorder(write)(input);
    expect(requests).toHaveLength(6);
  });

  test("evicts oldest acknowledgments after 1,024 entries without refreshing repeats", () => {
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
});
