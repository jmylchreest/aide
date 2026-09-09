import type { ContextIdentity, ContextWindow } from "../context-window.js";
import type { ObserveBatchEvent } from "../read-tracking.js";

/** Paired text at one boundary. Never asserts acceptance or model delivery. */
export function transformationEvent(
  identity: ContextIdentity,
  window: ContextWindow | null,
  call: string,
  tool: string,
  before: string,
  after: string,
  stage: "rewrite_candidate" | "adapter_change",
  recoveryPath?: string,
): ObserveBatchEvent | null {
  if (
    !identity.host ||
    !identity.sessionId ||
    !identity.actorId ||
    !call ||
    before === after
  )
    return null;
  return {
    kind: "hook",
    name: "output-transform",
    category: "transform",
    session: identity.sessionId,
    attrs: {
      accounting_version: "1",
      observation_stage: stage,
      raw_tool: tool,
      host: identity.host,
      actor_id: identity.actorId,
      invocation_id: call,
      before_bytes: String(Buffer.byteLength(before)),
      after_bytes: String(Buffer.byteLength(after)),
      context_status: window?.status ?? "unknown",
      ...(window ? { context_epoch: window.id } : {}),
      ...(recoveryPath ? { recovery_path: recoveryPath } : {}),
    },
  };
}
