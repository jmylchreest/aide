/**
 * Session resume logic — platform-agnostic.
 *
 * When a session resumes or restarts after a compaction, re-inject the most
 * recent session checkpoint so the agent rebuilds working context instead of
 * relearning it. This is aide's plugin-side equivalent of MiMo-Code's context
 * reconstruction: a plugin can't rebuild the host's conversation, but it can
 * inject the last checkpoint at the SessionStart boundary the host gives us.
 *
 * No LLM is involved — we surface the structured checkpoint verbatim and let
 * the main agent reconcile it against the (authoritative) live conversation.
 */

import { listMemoriesJson, type MemoryEntry } from "./memory-query.js";

/** SessionStart `source` values for which a resume injection is warranted. */
const RESUME_SOURCES = new Set(["resume", "compact"]);

/**
 * Query the newest checkpoint memory for this session.
 *
 * Prefers `checkpoint`-tagged memories (the structured snapshot written by the
 * PreCompact hook); falls back to `session-summary`-tagged memories so this
 * works even where the structured-checkpoint feature isn't present. Prefers an
 * entry tagged for the current session, but falls back to the newest checkpoint
 * overall (a resumed session may be assigned a fresh id by the host).
 */
export function getLatestCheckpoint(
  binary: string,
  cwd: string,
  sessionId: string,
): string | null {
  for (const tag of ["checkpoint", "session-summary"]) {
    const found = queryNewest(binary, cwd, sessionId, tag);
    if (found) return found;
  }
  return null;
}

function queryNewest(
  binary: string,
  cwd: string,
  sessionId: string,
  tag: string,
): string | null {
  // checkpoints are tagged `partial`, so --all is required to see them.
  const memories = listMemoriesJson(binary, cwd, {
    tags: tag,
    all: true,
    limit: 200,
  });
  if (memories.length === 0) return null;

  const byTime = (a: MemoryEntry, b: MemoryEntry) =>
    (b.createdAt ?? "").localeCompare(a.createdAt ?? "");

  const sessionTag = `session:${sessionId}`;
  const scoped = memories
    .filter((m) => m.tags?.includes(sessionTag))
    .sort(byTime);
  const newest = (scoped.length > 0 ? scoped : [...memories].sort(byTime))[0];

  return newest?.content?.trim() || null;
}

/**
 * Render a resume block from checkpoint content. Pure (no IO), unit-testable.
 *
 * The verify-before-act reminder is load-bearing: the checkpoint is a snapshot
 * that may be stale relative to the live conversation, so the agent must treat
 * it as a starting point, not ground truth.
 */
export function renderResumeContext(content: string): string {
  return [
    "## Resuming session — last checkpoint",
    "",
    "<system-reminder>",
    "This is a snapshot from before the session was compacted/resumed. Use it to " +
      "rebuild your bearings, but VERIFY against the current conversation and the " +
      "actual files before acting — re-read a file rather than trusting the snapshot " +
      "where they could disagree.",
    "</system-reminder>",
    "",
    content,
  ].join("\n");
}

/**
 * Build the resume context block to append at SessionStart.
 *
 * Returns null when the source isn't a resume/compact, or when no checkpoint
 * exists — callers append nothing in that case.
 */
export function buildResumeContext(
  binary: string,
  cwd: string,
  sessionId: string,
  source: string | undefined,
): string | null {
  if (!source || !RESUME_SOURCES.has(source)) return null;
  const content = getLatestCheckpoint(binary, cwd, sessionId);
  if (!content) return null;
  return renderResumeContext(content);
}
