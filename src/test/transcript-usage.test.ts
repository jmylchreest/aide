import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mkdtempSync, rmSync, writeFileSync, appendFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  catchUpTranscriptUsage,
  registerChildTranscript,
  usageTranscriptNamespace,
} from "../core/transcript-usage.js";

let cwd: string;
const session = "session-a";
const host = "claude-code" as const;
let rows: { key: string; agent: string; value: string }[];
let now: number;
const run = vi.fn();
const write = vi.fn();
const deps = { run, write, now: () => now };
function row(id: string, sessionId = session) {
  return (
    JSON.stringify({
      type: "assistant",
      sessionId,
      message: {
        id,
        model: "real",
        usage: {
          input_tokens: 2,
          cache_read_input_tokens: 3,
          cache_creation_input_tokens: 4,
        },
      },
    }) + "\n"
  );
}
function file(name: string, text = row(name)) {
  const path = join(cwd, name);
  writeFileSync(path, text);
  return path;
}
function register(path: string) {
  return registerChildTranscript("aide", cwd, host, session, path, deps);
}
function scan(path?: string) {
  return catchUpTranscriptUsage("aide", cwd, host, session, path, {}, deps);
}
beforeEach(() => {
  cwd = mkdtempSync(join(tmpdir(), "aide-usage-catchup-"));
  rows = [];
  now = 1000;
  run.mockReset().mockImplementation((_binary, _cwd, args: string[]) => {
    const agent = args.find((a) => a.startsWith("--agent="))!.slice(8);
    if (args[1] === "init-bounded") {
      if (rows.length >= 32) return null;
      const next = { key: `agent:${agent}:${args[2]}`, agent, value: args[3] };
      rows.push(next);
      return JSON.stringify(next);
    }
    if (args[1] === "list") return JSON.stringify(rows);
    if (args[1] === "delete") {
      rows = rows.filter((r) => r.key !== `agent:${agent}:${args[2]}`);
      return "Deleted";
    }
    return null;
  });
  write.mockReset().mockReturnValue(true);
});
afterEach(() => rmSync(cwd, { recursive: true, force: true }));

it("catches late child and main rows at a later boundary in one acknowledged batch", () => {
  const child = file("child");
  register(child);
  appendFileSync(child, row("late-child"));
  now++;
  const result = scan(file("main", row("early-main") + row("late-main")));
  expect(result.records).toBe(4);
  expect(result.retired).toBe(1);
  expect(rows).toHaveLength(0);
  expect(write).toHaveBeenCalledTimes(1);
  expect(
    write.mock.calls[0][2].map(
      (e: { attrs: { usage_id: string } }) => e.attrs.usage_id,
    ),
  ).toEqual(["early-main", "late-main", "child", "late-child"]);
  expect(write.mock.calls[0][3].timeout).toBeLessThanOrEqual(1000);
});
it("retains registrations on failed writes and retries with unchanged canonical identities", () => {
  register(file("child"));
  now++;
  write.mockReturnValueOnce(false);
  expect(scan().acknowledged).toBe(false);
  expect(rows).toHaveLength(1);
  expect(scan().retired).toBe(1);
  expect(write.mock.calls[0][2]).toEqual(write.mock.calls[1][2]);
});
it("retains missing, empty, mismatched, malformed and byte-limited evidence", () => {
  for (const path of [
    join(cwd, "missing"),
    file("empty", ""),
    file("wrong", row("x", "other")),
    file("bad", "bad\n" + row("bad")),
    file("large", "x".repeat(140000) + "\n" + row("large")),
  ])
    register(path);
  now++;
  const result = scan();
  expect(result.retired).toBe(0);
  expect(result.limited).toBe(true);
  expect(rows).toHaveLength(5);
});
it("unique registrations cannot be lost when a new completion arrives during a write", () => {
  const path = file("child");
  register(path);
  now++;
  write.mockImplementationOnce(() => {
    register(path);
    return true;
  });
  expect(scan().retired).toBe(1);
  expect(rows).toHaveLength(1);
  now++;
  expect(scan().retired).toBe(1);
});
it("retains a child whose final row flushes while the batch is being acknowledged", () => {
  const path = file("child");
  register(path);
  now++;
  write.mockImplementationOnce(() => {
    appendFileSync(path, row("last"));
    return true;
  });
  expect(scan().retired).toBe(0);
  expect(rows).toHaveLength(1);
  expect(scan().records).toBe(2);
  expect(rows).toHaveLength(0);
});
it("rejects unsupported bounded initialization without an unbounded fallback", () => {
  run.mockReturnValueOnce(null);
  expect(register(file("child"))).toBe(false);
  expect(run).toHaveBeenCalledTimes(1);
  expect(run.mock.calls[0][2][1]).toBe("init-bounded");
});
it("collects only Codex per-response records for the explicitly supplied session", () => {
  const path = file(
    "codex",
    JSON.stringify({
      type: "token_usage_record",
      payload: {
        thread_id: session,
        response_id: "codex-response",
        usage: {
          input_tokens: 10,
          output_tokens: 2,
          cached_input_tokens: 3,
          cache_write_input_tokens: 0,
        },
      },
    }) + "\n",
  );
  const result = catchUpTranscriptUsage(
    "aide",
    cwd,
    "codex",
    session,
    path,
    {},
    deps,
  );
  expect(result.records).toBe(1);
  expect(run).not.toHaveBeenCalled();
  expect(write.mock.calls[0][2][0].attrs.usage_id).toBe("codex-response");
  expect(
    catchUpTranscriptUsage("aide", cwd, "codex", "other", path, {}, deps)
      .records,
  ).toBe(0);
});
it("deduplicates paths but deletes only selected older registration IDs", () => {
  const path = file("child");
  register(path);
  register(path);
  now++;
  const result = scan();
  expect(result.paths).toBe(1);
  expect(result.records).toBe(1);
  expect(result.retired).toBe(2);
});
it("enforces atomic registration cap, explicit absolute paths and exact session/host isolation", () => {
  expect(register("relative")).toBe(false);
  expect(run).not.toHaveBeenCalled();
  for (let i = 0; i < 32; i++)
    expect(register(join(cwd, String(i)))).toBe(true);
  expect(register(join(cwd, "overflow"))).toBe(false);
  expect(run.mock.calls[0][2]).toContain("--max-agent-entries=32");
  expect(usageTranscriptNamespace(host, session)).not.toBe(
    usageTranscriptNamespace("codex", session),
  );
  expect(usageTranscriptNamespace(host, session)).not.toBe(
    usageTranscriptNamespace(host, "other"),
  );
  now++;
  expect(
    catchUpTranscriptUsage("aide", cwd, "codex", session, undefined, {}, deps)
      .paths,
  ).toBe(0);
});
it("never scans same-boundary registrations and never guesses paths on listing failure", () => {
  register(file("child"));
  expect(scan().paths).toBe(0);
  now++;
  run.mockReturnValueOnce(null);
  expect(scan().limited).toBe(true);
  expect(write).not.toHaveBeenCalled();
});
it("bounds total events and every subprocess by remaining hook time", () => {
  const main = file(
    "main",
    Array.from({ length: 300 }, (_, i) => row(String(i))).join(""),
  );
  const result = scan(main);
  expect(result.records).toBe(256);
  expect(result.limited).toBe(true);
  run.mockImplementationOnce(() => {
    now += 2100;
    return "[]";
  });
  write.mockClear();
  expect(scan(main).limited).toBe(true);
  expect(write).not.toHaveBeenCalled();
});
it("does not retire evidence on an unacknowledged delete or malformed list", () => {
  register(file("child"));
  now++;
  run.mockImplementationOnce(() => "{}");
  expect(scan().limited).toBe(true);
  run
    .mockImplementationOnce(() => JSON.stringify(rows))
    .mockImplementationOnce(() => null);
  expect(scan().retired).toBe(0);
  expect(rows).toHaveLength(1);
});

it("rotates four retained children through the three slots left by a main transcript", () => {
  for (let i = 0; i < 4; i++) register(join(cwd, `child-${i}`));
  // The first three registrations remain unreadable. The fourth must still get
  // a later scan even though prepending main consumes one of the four slots.
  const last = [...rows].sort((a, b) => a.key.localeCompare(b.key)).at(-1)!;
  const child = JSON.parse(last.value).path;
  writeFileSync(child, row("reachable-child"));
  const main = file("main", row("main"));
  now = 8000;
  expect(scan(main).records).toBe(1);
  expect(rows).toHaveLength(4);
  now = 10000;
  expect(scan(main).records).toBe(2);
  expect(rows).toHaveLength(3);
  expect(
    write.mock.calls
      .at(-1)![2]
      .map((event: { attrs: { usage_id: string } }) => event.attrs.usage_id),
  ).toContain("reachable-child");
});

it("reserves event capacity for children after a dense main transcript", () => {
  const main = file(
    "dense-main",
    Array.from({ length: 300 }, (_, i) => row(`main-${i}`)).join(""),
  );
  for (let i = 0; i < 3; i++)
    register(file(`child-${i}`, row(`child-final-${i}`)));
  now++;
  const result = scan(main);
  expect(result.paths).toBe(4);
  expect(result.records).toBe(67);
  expect(result.limited).toBe(true);
  expect(result.retired).toBe(3);
  const ids = write.mock.calls[0][2].map(
    (event: { attrs: { usage_id: string } }) => event.attrs.usage_id,
  );
  for (let i = 0; i < 3; i++) expect(ids).toContain(`child-final-${i}`);
  expect(ids.filter((id: string) => id.startsWith("main-"))).toHaveLength(64);
});

it("does not launch a state listing when the budget expires before the first call", () => {
  const clock = vi.fn().mockReturnValueOnce(1000).mockReturnValue(3001);
  const result = catchUpTranscriptUsage(
    "aide",
    cwd,
    host,
    session,
    undefined,
    {},
    { ...deps, now: clock },
  );
  expect(result.limited).toBe(true);
  expect(result.paths).toBe(0);
  expect(run).not.toHaveBeenCalled();
  expect(write).not.toHaveBeenCalled();
});
it("never supplies an unlimited zero timeout while retiring registrations", () => {
  register(file("child"));
  now++;
  const deadline = now + 2000;
  let retiring = false;
  let lateClockReads = 0;
  write.mockImplementationOnce(() => {
    retiring = true;
    return true;
  });
  const clock = () =>
    !retiring ? now : ++lateClockReads === 1 ? deadline - 1 : deadline + 1;
  const result = catchUpTranscriptUsage(
    "aide",
    cwd,
    host,
    session,
    undefined,
    {},
    { ...deps, now: clock },
  );
  expect(result.acknowledged).toBe(true);
  expect(result.limited).toBe(true);
  for (const call of run.mock.calls) expect(call[3]).toBeGreaterThan(0);
});
