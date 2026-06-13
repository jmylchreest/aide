/**
 * Session checkpoint logic — platform-agnostic.
 *
 * Builds a structured "session checkpoint" before context compaction, so the
 * work-so-far is recoverable after the harness summarises the conversation.
 *
 * Why a checkpoint and not just a flat summary: aide has structured stores of
 * its own (task tree, git state) that a mechanical roll-up can pull from
 * without any LLM. The checkpoint shape mirrors the parts of MiMo-Code's
 * checkpoint that are derivable without LLM judgment — task state, work
 * accomplished, files touched, live resources — and is intentionally bounded
 * (capped lists) rather than "compressed", since the hook has no model access.
 *
 * The sections that genuinely need LLM judgment (verbatim user intent,
 * cross-task discovered knowledge) are deliberately omitted here — fabricating
 * them mechanically would be worse than leaving them out.
 */

import { execFileSync } from "child_process";
import { debug } from "../lib/logger.js";

const SOURCE = "session-checkpoint";

/** Structured inputs for a checkpoint. All optional sections are omitted when empty. */
export interface CheckpointInput {
  sessionId: string;
  /** Raw partial-memory content strings for this session (from partial-memory). */
  partials: string[];
  /** Git oneline commit subjects made during the session. */
  commits: string[];
  /** Pre-rendered task-tree lines (icon + summary). See getTaskTree. */
  taskTree?: string[];
  /** One-line live runtime state, e.g. "branch main · 3 uncommitted file(s)". */
  liveState?: string;
}

interface TaskEntry {
  id?: string;
  title?: string;
  description?: string;
  status?: string;
}

/** Map a task status to a MiMo-style status icon. */
function statusIcon(status: string | undefined): string {
  switch ((status ?? "").toLowerCase()) {
    case "done":
    case "complete":
    case "completed":
      return "✅";
    case "claimed":
    case "in_progress":
    case "in-progress":
      return "🔄";
    case "blocked":
      return "🟡";
    case "abandoned":
      return "❌";
    default:
      return "🔵"; // pending / open / unknown
  }
}

/**
 * Render the current task tree from the swarm task store.
 *
 * Returns one line per task (`<icon> <summary> [status]`), capped. Empty array
 * when no tasks exist (the common case for solo, non-swarm sessions) or when
 * the task store can't be read — the checkpoint simply omits the section.
 */
export function getTaskTree(binary: string, cwd: string): string[] {
  try {
    const output = execFileSync(binary, ["task", "list", "--json"], {
      cwd,
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
      timeout: 5000,
    }).trim();

    if (!output || output === "[]" || output === "null") return [];

    const tasks: TaskEntry[] = JSON.parse(output);
    if (!Array.isArray(tasks)) return [];

    return tasks.slice(0, 15).map((t) => {
      const label = t.title || t.description || t.id || "task";
      const status = t.status ? ` [${t.status}]` : "";
      return `${statusIcon(t.status)} ${label}${status}`;
    });
  } catch (err) {
    debug(SOURCE, `Failed to read task tree (non-fatal): ${err}`);
    return [];
  }
}

/**
 * Describe volatile runtime state — current branch and uncommitted file count.
 * Returns undefined when not a git repo or git is unavailable.
 */
export function getGitLiveState(cwd: string): string | undefined {
  try {
    const branch = execFileSync(
      "git",
      ["rev-parse", "--abbrev-ref", "HEAD"],
      { cwd, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"], timeout: 3000 },
    ).trim();

    const porcelain = execFileSync("git", ["status", "--porcelain"], {
      cwd,
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
      timeout: 3000,
    }).trim();

    const dirtyCount = porcelain ? porcelain.split("\n").filter((l) => l.trim()).length : 0;
    const parts: string[] = [];
    if (branch) parts.push(`branch ${branch}`);
    parts.push(`${dirtyCount} uncommitted file(s)`);
    return parts.join(" · ");
  } catch {
    return undefined;
  }
}

/**
 * Build a structured session checkpoint from already-gathered inputs.
 *
 * Pure function (no IO) so it is unit-testable. Sections with no content are
 * omitted entirely. Returns null when the result carries no substantive
 * content (mirrors the >=50-char guard used elsewhere).
 */
export function buildSessionCheckpoint(input: CheckpointInput): string | null {
  // Categorise partials into files / commands / completed tasks.
  const fileChanges = new Set<string>();
  const commands: string[] = [];
  const completed: string[] = [];
  const other: string[] = [];

  for (const p of input.partials) {
    if (p.startsWith("Created file: ") || p.startsWith("Edited file: ")) {
      fileChanges.add(p.replace(/^(Created|Edited) file: /, ""));
    } else if (p.startsWith("Ran command: ")) {
      commands.push(p.replace("Ran command: ", ""));
    } else if (p.startsWith("Completed task: ")) {
      completed.push(p.replace("Completed task: ", ""));
    } else {
      other.push(p);
    }
  }

  const sections: string[] = [
    "# Session checkpoint",
    "_Structured snapshot written before context compaction. Use it to resume work; the verbatim conversation is the ground truth where they disagree._",
  ];

  if (input.taskTree && input.taskTree.length > 0) {
    sections.push(`## Task state\n${input.taskTree.map((t) => `- ${t}`).join("\n")}`);
  }

  if (completed.length > 0) {
    sections.push(
      `## Work completed\n${completed.slice(0, 10).map((t) => `- ${t}`).join("\n")}`,
    );
  }

  if (input.commits.length > 0) {
    sections.push(`## Commits\n${input.commits.map((c) => `- ${c}`).join("\n")}`);
  }

  if (fileChanges.size > 0) {
    const files = Array.from(fileChanges).slice(0, 15);
    sections.push(`## Files touched\n${files.map((f) => `- ${f}`).join("\n")}`);
  }

  if (commands.length > 0) {
    sections.push(
      `## Commands run\n${commands.slice(0, 10).map((c) => `- ${c}`).join("\n")}`,
    );
  }

  if (other.length > 0) {
    sections.push(`## Other\n${other.slice(0, 10).map((o) => `- ${o}`).join("\n")}`);
  }

  if (input.liveState) {
    sections.push(`## Live resources\n${input.liveState}`);
  }

  // sections[0..1] are the header + instruction line; require at least one
  // content section before we consider the checkpoint worth persisting.
  if (sections.length <= 2) return null;

  const checkpoint = sections.join("\n\n");
  return checkpoint.length >= 50 ? checkpoint : null;
}
