#!/usr/bin/env node
/**
 * Agent Signals Hook (PreToolUse)
 *
 * Fires before every tool call. In subagent contexts (session has a parent
 * recorded by SubagentStart) it checks for halt/pause flags and unread
 * high-priority messages, blocking or injecting context accordingly.
 *
 * In orchestrator / solo sessions this hook is a no-op — gated on the
 * session having a registered parent. Cost when gated out: one cheap
 * `aide agent identify` call (~10ms).
 */

import { execFileSync } from "child_process";
import { Logger } from "../lib/logger.js";
import {
  readStdin,
  emitHookResult,
  installHookSafetyNet,
} from "../lib/hook-utils.js";
import { findAideBinary } from "../core/aide-client.js";
import { emitInjectionEvent } from "../core/read-tracking.js";

const SOURCE = "agent-signals";

interface PreToolUseInput {
  hook_event_name: "PreToolUse";
  session_id: string;
  tool_name: string;
  cwd: string;
}

interface HookOutput {
  continue: boolean;
  message?: string;
  hookSpecificOutput?: {
    hookEventName: string;
    additionalContext?: string;
  };
}

interface SignalsResponse {
  agent: string;
  halt?: boolean;
  paused?: boolean;
  reason?: string;
  deadline?: string;
  deadline_remaining_sec?: number;
  high_priority_messages?: Array<{
    id: number;
    from: string;
    content: string;
    type?: string;
  }>;
}

let log: Logger | null = null;

function passThrough(): void {
  emitHookResult({ continue: true });
}

function block(message: string): void {
  const out: HookOutput = { continue: false, message };
  emitHookResult(out);
}

function injectContext(context: string): void {
  const out: HookOutput = {
    continue: true,
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      additionalContext: context,
    },
  };
  emitHookResult(out);
}

function runAide(binary: string, cwd: string, args: string[]): string | null {
  try {
    return execFileSync(binary, args, {
      cwd,
      stdio: ["pipe", "pipe", "pipe"],
      timeout: 2000,
    }).toString();
  } catch (err) {
    log?.debug(`aide ${args.join(" ")} failed: ${err}`);
    return null;
  }
}

// Tools the subagent is still allowed to call while paused — limited to
// communication primitives so it can report back / receive instruction.
const PAUSE_ALLOWLIST = new Set([
  "mcp__plugin_aide_aide__message_send",
  "mcp__plugin_aide_aide__message_list",
  "mcp__plugin_aide_aide__message_ack",
  "mcp__plugin_aide_aide__state_get",
]);

async function main(): Promise<void> {
  try {
    const raw = await readStdin();
    if (!raw.trim()) {
      passThrough();
      return;
    }
    const data: PreToolUseInput = JSON.parse(raw);
    const cwd = data.cwd || process.cwd();
    log = new Logger("agent-signals", cwd);

    const binary = findAideBinary({
      cwd,
      pluginRoot:
        process.env.AIDE_PLUGIN_ROOT || process.env.CLAUDE_PLUGIN_ROOT,
    });
    if (!binary) {
      passThrough();
      return;
    }

    // Gate: is this session a registered subagent? identify returns JSON
    // with parent_session if and only if SubagentStart recorded one.
    const idRaw = runAide(binary, cwd, [
      "agent",
      "identify",
      `--agent=${data.session_id}`,
    ]);
    if (!idRaw) {
      passThrough();
      return;
    }
    let identity: Record<string, string>;
    try {
      identity = JSON.parse(idRaw);
    } catch {
      passThrough();
      return;
    }
    if (!identity.parent_session) {
      // Not a subagent — orchestrator/solo session. No-op.
      passThrough();
      return;
    }

    // Pull signal snapshot.
    const sigRaw = runAide(binary, cwd, [
      "agent",
      "signals",
      `--agent=${data.session_id}`,
    ]);
    if (!sigRaw) {
      passThrough();
      return;
    }
    let sig: SignalsResponse;
    try {
      sig = JSON.parse(sigRaw);
    } catch {
      passThrough();
      return;
    }

    // 1. Halt — hard stop with reason surfaced to model.
    if (sig.halt) {
      const reason = sig.reason || "halted by orchestrator";
      log?.info(`halt active for ${data.session_id}: ${reason}`);
      block(
        `[aide] This subagent has been halted by the orchestrator. Reason: ${reason}\n` +
          `Stop work. You may still send a final status message via aide message send --to=<orchestrator>.`,
      );
      return;
    }

    // 2. Pause — block all tools except the comm allowlist.
    if (sig.paused && !PAUSE_ALLOWLIST.has(data.tool_name)) {
      log?.info(`paused — blocking ${data.tool_name}`);
      block(
        `[aide] This subagent is paused by the orchestrator. Only messaging tools are allowed until resumed.`,
      );
      return;
    }

    // 3. Deadline approaching — inject warning at < 20% remaining.
    let deadlineWarn = "";
    if (sig.deadline_remaining_sec !== undefined) {
      if (sig.deadline_remaining_sec <= 0) {
        block(
          `[aide] Soft deadline reached (${sig.deadline}). Halting per orchestrator policy.`,
        );
        return;
      }
      if (sig.deadline_remaining_sec < 60 * 5) {
        deadlineWarn = `[aide] Deadline ${sig.deadline} — ~${sig.deadline_remaining_sec}s remaining.`;
      }
    }

    // 4. High-priority messages — inject + ack so we don't re-inject next call.
    const parts: string[] = [];
    if (deadlineWarn) parts.push(deadlineWarn);
    if (sig.high_priority_messages && sig.high_priority_messages.length > 0) {
      parts.push("[aide] Mid-flight messages from orchestrator:");
      for (const m of sig.high_priority_messages) {
        parts.push(`- (${m.from}${m.type ? `, ${m.type}` : ""}): ${m.content}`);
        runAide(binary, cwd, [
          "message",
          "ack",
          String(m.id),
          `--agent=${data.session_id}`,
        ]);
      }
    }

    if (parts.length > 0) {
      const ctx = parts.join("\n");
      try {
        emitInjectionEvent(binary, cwd, {
          source: SOURCE,
          subtype: "signal",
          content: ctx,
          sessionId: data.session_id,
          attrs: {
            tool: data.tool_name,
            ...(sig.deadline ? { deadline: sig.deadline } : {}),
            high_priority_messages: String(
              sig.high_priority_messages?.length ?? 0,
            ),
          },
        });
      } catch {
        // Non-fatal
      }
      injectContext(ctx);
      return;
    }

    passThrough();
  } catch (err) {
    log?.error(`agent-signals failed: ${err}`);
    passThrough();
  }
}

installHookSafetyNet(SOURCE);

main();
