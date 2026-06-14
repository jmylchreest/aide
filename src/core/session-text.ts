/**
 * Session text helpers — platform-agnostic.
 *
 * Shared building blocks for the session summary / checkpoint builders. These
 * were duplicated across partial-memory.ts, session-summary-logic.ts, and
 * session-checkpoint-logic.ts; centralising them keeps the section formatting
 * and the partial-classification rules in one place.
 */

/** Partials grouped by the kind of action they record. */
export interface CategorizedPartials {
  /** Files created or edited, de-duplicated, first-seen order preserved. */
  files: string[];
  /** Shell commands run. */
  commands: string[];
  /** Completed (sub)task descriptions. */
  tasks: string[];
  /** Anything that didn't match a known prefix. */
  other: string[];
}

/**
 * Classify raw partial-memory content strings (as produced by
 * buildPartialContent) into files / commands / tasks / other.
 */
export function categorizePartials(partials: string[]): CategorizedPartials {
  const files = new Set<string>();
  const commands: string[] = [];
  const tasks: string[] = [];
  const other: string[] = [];

  for (const p of partials) {
    if (p.startsWith("Created file: ") || p.startsWith("Edited file: ")) {
      files.add(p.replace(/^(Created|Edited) file: /, ""));
    } else if (p.startsWith("Ran command: ")) {
      commands.push(p.replace("Ran command: ", ""));
    } else if (p.startsWith("Completed task: ")) {
      tasks.push(p.replace("Completed task: ", ""));
    } else {
      other.push(p);
    }
  }

  return { files: Array.from(files), commands, tasks, other };
}

/**
 * Render a markdown bullet section: `## <heading>` followed by `- <item>`
 * lines. Returns null when there are no items (so callers can omit the
 * section). When `cap` is given, only the first `cap` items are rendered.
 */
export function renderBulletSection(
  heading: string,
  items: string[],
  cap?: number,
): string | null {
  if (items.length === 0) return null;
  const list = cap !== undefined ? items.slice(0, cap) : items;
  return `## ${heading}\n${list.map((i) => `- ${i}`).join("\n")}`;
}
