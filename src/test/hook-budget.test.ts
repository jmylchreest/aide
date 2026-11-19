import { afterEach, describe, expect, it, vi } from "vitest";
import { spawnSync } from "child_process";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllEnvs();
  vi.resetModules();
});

describe("hook command budget", () => {
  it("leaves non-hook callers' timeouts unchanged", async () => {
    const { hookCommandTimeout } = await import("../lib/hook-budget.js");
    expect(hookCommandTimeout(10000)).toBe(10000);
  });

  it("shares a deadline across repeated calls and refuses work after expiry", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(10000);
    vi.stubEnv("AIDE_HOOK_DEADLINE_MS", "");
    const { startHookBudget, hookCommandTimeout } =
      await import("../lib/hook-budget.js");
    startHookBudget("context-guard");
    expect(hookCommandTimeout(5000)).toBe(1500);
    vi.setSystemTime(11000);
    expect(hookCommandTimeout(5000)).toBe(500);
    vi.setSystemTime(11500);
    expect(() => hookCommandTimeout(5000)).toThrow("budget exhausted");
  });

  it("does not reset the dispatcher's deadline when its child starts", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(10000);
    vi.stubEnv("AIDE_HOOK_DEADLINE_MS", "10500");
    const { startHookBudget, hookCommandTimeout } =
      await import("../lib/hook-budget.js");
    expect(startHookBudget("context-guard")).toBe(10500);
    expect(hookCommandTimeout(5000)).toBe(500);
  });

  it("bounds several real subprocess calls with one budget", async () => {
    const { startHookBudget } = await import("../lib/hook-budget.js");
    const { runAide } = await import("../core/aide-client.js");
    const { setDebugCwd } = await import("../lib/logger.js");
    setDebugCwd(tmpdir());
    vi.stubEnv("AIDE_HOOK_DEADLINE_MS", String(Date.now() + 300));
    startHookBudget("context-guard");
    const start = Date.now();
    for (let i = 0; i < 3; i++) {
      expect(
        runAide(
          process.execPath,
          tmpdir(),
          ["-e", "setTimeout(() => {}, 10000)"],
          { timeout: 5000 },
        ),
      ).toBeNull();
    }
    const elapsed = Date.now() - start;
    expect(elapsed).toBeGreaterThanOrEqual(200);
    expect(elapsed).toBeLessThan(1500);
  });
});

// A synthetic executable fixture; the subprocess budget test above also runs
// on Windows, where a script cannot stand in for an .exe.
it.skipIf(process.platform === "win32")(
  "tool tracking returns valid output before the host kills a stalled hook",
  () => {
    const dir = mkdtempSync(join(tmpdir(), "aide-hook-budget-"));
    try {
      mkdirSync(join(dir, "bin"));
      mkdirSync(join(dir, ".aide"));
      writeFileSync(
        join(dir, "bin/aide"),
        "#!/usr/bin/env bun\nawait Bun.sleep(10000);\n",
        { mode: 0o755 },
      );
      const result = spawnSync(
        "bun",
        [join(import.meta.dirname, "../hooks/tool-tracker.ts")],
        {
          cwd: dir,
          env: {
            ...process.env,
            AIDE_PLUGIN_ROOT: dir,
            AIDE_DEBUG: "0",
            AIDE_HOOK_DEADLINE_MS: String(Date.now() + 600),
          },
          input: JSON.stringify({
            cwd: dir,
            session_id: "budget-test",
            tool_name: "Bash",
            tool_input: { command: "true" },
          }),
          encoding: "utf8",
          timeout: 2000,
        },
      );
      expect(result.error).toBeUndefined();
      expect(result.status).toBe(0);
      expect(JSON.parse(result.stdout)).toEqual({ continue: true });
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  },
);
