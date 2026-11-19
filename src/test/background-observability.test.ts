import { describe, expect, it } from "vitest";
import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

describe("background observability", () => {
  it("returns host output immediately while a slow backend finishes in order with captured routing", async () => {
    const dir = mkdtempSync(join(tmpdir(), "aide-observation-"));
    const output = join(dir, "recorded.jsonl");
    const fixture = join(dir, "backend.ts");
    const producer = join(dir, "producer.ts");
    writeFileSync(
      fixture,
      `
      import { appendFileSync } from "node:fs";
      await Bun.sleep(Number(process.argv[3]));
      appendFileSync(process.argv[2], JSON.stringify({ session: process.env.AIDE_SESSION_ID, input: await Bun.stdin.text() }) + "\\n");
    `,
    );
    writeFileSync(
      producer,
      `
      import { enqueueObservation } from ${JSON.stringify(join(import.meta.dirname, "../lib/background-observability.ts"))};
      process.env.AIDE_SESSION_ID = "first";
      enqueueObservation({ kind: "command", binary: process.execPath, cwd: ${JSON.stringify(dir)}, args: [${JSON.stringify(fixture)}, ${JSON.stringify(output)}, "5500"], input: "first payload" });
      enqueueObservation({ kind: "command", binary: "aide-missing-observation-test", cwd: ${JSON.stringify(dir)}, args: [] });
      process.env.AIDE_SESSION_ID = "second";
      enqueueObservation({ kind: "command", binary: process.execPath, cwd: ${JSON.stringify(dir)}, args: [${JSON.stringify(fixture)}, ${JSON.stringify(output)}, "0"], input: "x".repeat(256 * 1024) });
      console.log(JSON.stringify({ continue: true, context: "available now" }));
    `,
    );
    try {
      const started = Date.now();
      const result = spawnSync("bun", [producer], {
        encoding: "utf8",
        timeout: 2000,
        env: { ...process.env, AIDE_DEBUG: "0" },
      });
      expect(result.error).toBeUndefined();
      expect(result.status).toBe(0);
      expect(Date.now() - started).toBeLessThan(2000);
      expect(JSON.parse(result.stdout)).toEqual({
        continue: true,
        context: "available now",
      });
      expect(existsSync(output)).toBe(false);
      await expect
        .poll(
          () =>
            existsSync(output)
              ? readFileSync(output, "utf8").trim().split("\n").length
              : 0,
          { timeout: 9000 },
        )
        .toBe(2);
      expect(
        readFileSync(output, "utf8")
          .trim()
          .split("\n")
          .map((line) => JSON.parse(line)),
      ).toEqual([
        { session: "first", input: "first payload" },
        { session: "second", input: "x".repeat(256 * 1024) },
      ]);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  }, 12_000);

  it("rejects an oversized pending observation without spawning work", () => {
    const module = join(
      import.meta.dirname,
      "../lib/background-observability.ts",
    );
    const code = `import { enqueueObservation } from ${JSON.stringify(module)};
      console.log(enqueueObservation({ kind: "command", binary: "missing", cwd: ".", args: [], input: "x".repeat(9*1024*1024) }));`;
    const result = spawnSync("bun", ["-e", code], {
      encoding: "utf8",
      timeout: 2000,
      env: { ...process.env, AIDE_DEBUG: "0" },
    });
    expect(result.status).toBe(0);
    expect(result.stdout.trim()).toBe("false");
  });
});
