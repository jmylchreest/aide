#!/usr/bin/env node
/** Record an explicit host compaction completion without reinjecting context. */
import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
  findAideBinary,
  detectPlatform,
} from "../lib/hook-utils.js";
import { setSessionContext } from "../lib/anchor.js";
import { updateHookContextWindow } from "../core/hook-context.js";
import { recordObserveEvent } from "../core/read-tracking.js";
import { debug } from "../lib/logger.js";

const SOURCE = "post-compact";

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) return;
    const data = JSON.parse(input) as {
      hook_event_name?: string;
      session_id?: string;
      agent_id?: string;
      cwd?: string;
      trigger?: string;
    };
    if (data.hook_event_name !== "PostCompact") return;
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id;
    if (!sessionId || sessionId === "unknown") return;
    setSessionContext(sessionId);
    const binary = findAideBinary(cwd, sessionId);
    if (!binary) return;
    const host = detectPlatform();
    const window = updateHookContextWindow(binary, cwd, host, data, "compact");
    recordObserveEvent(binary, cwd, {
      kind: "session",
      name: SOURCE,
      category: "lifecycle",
      subtype: data.trigger || "compact",
      session: sessionId,
      attrs: {
        host,
        actor_id: data.agent_id || sessionId,
        context_status: window?.status ?? "unknown",
        ...(window ? { context_epoch: window.id } : {}),
      },
    });
  } catch (error) {
    debug(SOURCE, `Hook error: ${error}`);
  } finally {
    emitHookResult();
  }
}

installHookSafetyNet(SOURCE);
main();
