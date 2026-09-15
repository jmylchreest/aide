#!/usr/bin/env node
/** Retry late transcript flushes only at a later, genuine host boundary. */
import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
  findAideBinary,
  detectPlatform,
} from "../lib/hook-utils.js";
import { debug, setDebugCwd } from "../lib/logger.js";
import { setSessionContext } from "../lib/anchor.js";
import { catchUpTranscriptUsage } from "../core/transcript-usage.js";

installHookSafetyNet("usage-catchup");
async function main(): Promise<void> {
  try {
    const data = JSON.parse(await readStdin());
    const host = detectPlatform();
    if (
      !["SessionStart", "UserPromptSubmit", "SessionEnd"].includes(
        data.hook_event_name,
      ) ||
      (host !== "claude-code" && host !== "codex") ||
      typeof data.session_id !== "string" ||
      typeof data.cwd !== "string"
    )
      return;
    // A SessionEnd without identity cannot safely claim another session's evidence.
    if (!data.session_id.trim() || data.session_id === "unknown") return;
    setSessionContext(data.session_id);
    setDebugCwd(data.cwd);
    const binary = findAideBinary(data.cwd, data.session_id);
    if (!binary) return;
    const result = catchUpTranscriptUsage(
      binary,
      data.cwd,
      host,
      data.session_id,
      data.transcript_path,
      { budgetMs: data.hook_event_name === "SessionEnd" ? 1000 : 2000 },
    );
    debug(
      "usage-catchup",
      JSON.stringify({ boundary: data.hook_event_name, ...result }),
    );
  } catch {
    debug(
      "usage-catchup",
      "Catch-up unavailable; retained evidence can retry at a later boundary",
    );
  } finally {
    emitHookResult({ continue: true });
  }
}
main();
