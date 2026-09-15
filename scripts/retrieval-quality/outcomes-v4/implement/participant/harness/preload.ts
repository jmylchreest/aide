/** EXPERIMENT HARNESS: authored for the offline fixture, not aide source.
 * Replace binary I/O, host input/log output, initialization and summary storage.
 * The usage writer/parser/recorder and host routing remain real source modules.
 */
import { mock } from "bun:test";
import * as childProcess from "node:child_process";

export const harness = {
  input: "", host: "claude-code" as "claude-code" | "codex",
  binary: "/fixture/aide" as string | null,
  response: "Recorded 1 event(s)\n", fail: false,
  calls: [] as Array<{binary: unknown; args: unknown; options: any}>,
  logs: [] as string[], emitted: [] as unknown[], summaries: 0,
};
const execFileSync = (binary: unknown, args: unknown, options: unknown) => {
  harness.calls.push({ binary, args, options });
  if (harness.fail) throw new Error("fixture write failure");
  return Buffer.from(harness.response);
};
mock.module("node:child_process", () => ({ ...childProcess, execFileSync }));
mock.module("child_process", () => ({ ...childProcess, execFileSync }));
// Unused external package boundaries. No installed dependencies are needed.
mock.module("which", () => ({ default: { sync: () => null } }));
mock.module("smol-toml", () => ({ parse: () => ({}), stringify: () => "" }));
const logger = await import("../src/lib/logger.ts");
mock.module("../src/lib/logger.ts", () => ({ ...logger,
  debug: (_source: string, message: string) => harness.logs.push(message),
  setDebugCwd: () => {},
}));
const client = await import("../src/core/aide-client.ts");
mock.module("../src/core/aide-client.ts", () => ({ ...client,
  findAideBinary: () => harness.binary,
  getScopedState: () => null,
}));
const init = await import("../src/core/session-init.ts");
mock.module("../src/core/session-init.ts", () => ({ ...init,
  ensureDirectories: () => ({}), loadConfig: () => ({}),
  cleanupStaleStateFiles: () => ({}), resetHudState: () => {},
  runSessionInit: () => null,
}));
mock.module("../src/core/mcp-sync.ts", () => ({ syncMcpServers: () => {} }));
const summary = await import("../src/core/session-summary-logic.ts");
mock.module("../src/core/session-summary-logic.ts", () => ({ ...summary,
  buildSessionSummary: () => { harness.summaries++; return "fixture summary"; },
  storeSessionSummary: () => true,
}));
const partials = await import("../src/core/partial-memory.ts");
mock.module("../src/core/partial-memory.ts", () => ({ ...partials,
  gatherPartials: () => [], cleanupPartials: () => 0,
}));
const hookUtils = await import("../src/lib/hook-utils.ts");
mock.module("../src/lib/hook-utils.ts", () => ({ ...hookUtils,
  readStdin: async () => harness.input,
  detectPlatform: () => harness.host,
  findAideBinary: () => harness.binary,
  emitHookResult: (value: unknown) => harness.emitted.push(value),
  installHookSafetyNet: () => {},
}));
export function resetHarness() {
  harness.input = ""; harness.host = "claude-code";
  harness.binary = "/fixture/aide"; harness.response = "Recorded 1 event(s)\n";
  harness.fail = false; harness.calls.length = 0; harness.logs.length = 0;
  harness.emitted.length = 0; harness.summaries = 0;
}
