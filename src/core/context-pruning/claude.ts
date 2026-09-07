import { mkdirSync, readFileSync, writeFileSync, renameSync } from "fs";
import { join } from "path";
import { randomUUID } from "crypto";
import {
  contextScope,
  type ContextIdentity,
  type ContextWindow,
} from "../context-window.js";
import { extractOutputText } from "../tool-observe.js";
import { ContextPruningTracker } from "./tracker.js";
import { recoverablePrune } from "./recovery.js";
import { replacementTarget } from "./replacement.js";
import { transformationEvent } from "./observation.js";
import type { ToolRecord } from "./types.js";

export interface PruningHookInput {
  hook_event_name?: string;
  session_id?: string;
  agent_id?: string;
  cwd?: string;
  tool_name?: string;
  tool_input?: Record<string, unknown>;
  tool_response?: unknown;
  tool_output?: unknown;
  tool_use_id?: string;
}

export function processPruningHook(
  cwd: string,
  identity: ContextIdentity,
  window: ContextWindow | null,
  input: PruningHookInput,
) {
  const scope = contextScope(identity);
  const call = input.tool_use_id;
  const tool = input.tool_name ?? "";
  if (
    !scope ||
    !window ||
    window.status !== "active" ||
    !call ||
    input.hook_event_name === "PostToolUseFailure"
  )
    return null;
  const payload = input.tool_response ?? input.tool_output;
  const target = replacementTarget(tool, payload);
  if (!target || target.text.length < 50) return null;
  const tracker = new ContextPruningTracker(cwd);
  // Hash the epoch too: no raw host-supplied identity enters a path.
  const key = contextScope({
    host: scope,
    sessionId: window.id,
    actorId: "history-v1",
  })!;
  const dir = join(cwd, ".aide", "artifacts", "pruning-state");
  const historyFile = join(dir, `${key}.json`);
  let history: ToolRecord[] = [];
  try {
    const loaded = JSON.parse(readFileSync(historyFile, "utf8"));
    if (loaded.version === 1 && Array.isArray(loaded.history))
      history = loaded.history;
  } catch {
    /* no compatible history */
  }
  tracker.loadHistory(history);
  const prior = history.find((r) => r.callId === call);
  // Hook replays must not turn the first original into a new dedup candidate.
  if (prior) return null;
  const result = tracker.process(
    call,
    tool,
    input.tool_input ?? {},
    target.text,
    recoverablePrune(cwd, key, call, target.text),
  );
  try {
    mkdirSync(dir, { recursive: true, mode: 0o700 });
    const temporary = join(dir, `${randomUUID()}.tmp`);
    writeFileSync(
      temporary,
      JSON.stringify({ version: 1, history: tracker.getHistory() }),
      { mode: 0o600 },
    );
    renameSync(temporary, historyFile);
  } catch {
    /* missing history reduces future dedup only; originals remain available */
  }
  if (!result.modified) return null;
  const replacement = target.replace(result.output);
  const before = extractOutputText(payload);
  const after = extractOutputText(replacement);
  if (before === undefined || after === undefined) return null;
  return {
    replacement,
    event: transformationEvent(
      identity,
      window,
      call,
      tool,
      before,
      after,
      "rewrite_candidate",
      result.recoveryPath,
    ),
  };
}
