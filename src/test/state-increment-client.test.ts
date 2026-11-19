import { beforeEach, expect, it, vi } from "vitest";
vi.mock("child_process", () => ({ execFileSync: vi.fn() }));
vi.mock("../lib/logger.js", () => ({ debug: vi.fn() }));
import { execFileSync } from "child_process";
import { incrementState } from "../core/aide-client.js";
beforeEach(() => vi.clearAllMocks());
it("requests one atomic increment and reads the stored value", () => {
  vi.mocked(execFileSync).mockReturnValue('{"value":"12"}');
  expect(incrementState("aide", "/project", "iterations", "session")).toBe(12);
  expect(execFileSync).toHaveBeenCalledWith(
    "aide",
    ["state", "increment", "iterations", "--json", "--agent=session"],
    expect.objectContaining({ cwd: "/project" }),
  );
});
it.each([
  "{}",
  '{"value":"garbage"}',
  '{"value":"0"}',
  '{"value":"9007199254740993"}',
  "not json",
])("rejects an invalid counter response %s", (output) => {
  vi.mocked(execFileSync).mockReturnValue(output);
  expect(incrementState("aide", "/project", "iterations")).toBeNull();
  expect(execFileSync).toHaveBeenCalledTimes(1);
});
it("does not emulate an increment when an older binary rejects it", () => {
  vi.mocked(execFileSync).mockImplementation(() => {
    throw new Error("unsupported command");
  });
  expect(incrementState("aide", "/project", "iterations")).toBeNull();
  expect(execFileSync).toHaveBeenCalledTimes(1);
});
