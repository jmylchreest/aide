/**
 * Skill discovery and matching — platform-agnostic.
 *
 * Extracted from src/hooks/skill-injector.ts.
 * Finds skill files, parses frontmatter, and fuzzy-matches triggers.
 */

import { existsSync, readFileSync, readdirSync } from "fs";
import { join, basename, extname } from "path";
import { homedir } from "os";
import type { Skill, SkillMatchResult } from "./types.js";

// Skill search locations relative to cwd
const SKILL_LOCATIONS = [
  ".aide/skills",
  "skills",
];

const GLOBAL_SKILL_LOCATIONS = [
  join(homedir(), ".aide", "skills"),
];

/**
 * Calculate Levenshtein distance between two strings
 */
export function levenshtein(a: string, b: string): number {
  const matrix: number[][] = [];

  if (a.length === 0) return b.length;
  if (b.length === 0) return a.length;

  for (let i = 0; i <= b.length; i++) {
    matrix[i] = [i];
  }
  for (let j = 0; j <= a.length; j++) {
    matrix[0][j] = j;
  }

  for (let i = 1; i <= b.length; i++) {
    for (let j = 1; j <= a.length; j++) {
      if (b.charAt(i - 1) === a.charAt(j - 1)) {
        matrix[i][j] = matrix[i - 1][j - 1];
      } else {
        matrix[i][j] = Math.min(
          matrix[i - 1][j - 1] + 1,
          matrix[i][j - 1] + 1,
          matrix[i - 1][j] + 1,
        );
      }
    }
  }

  return matrix[b.length][a.length];
}

/**
 * Check if a trigger fuzzy-matches any word sequence in the prompt
 */
export function fuzzyMatchTrigger(
  promptLower: string,
  trigger: string,
  maxDistance: number = 2,
): boolean {
  const triggerWords = trigger.split(/\s+/);
  const promptWords = promptLower.split(/\s+/);

  if (triggerWords.length === 1) {
    for (const word of promptWords) {
      const dist = levenshtein(word, trigger);
      const allowedDist = Math.min(maxDistance, Math.floor(trigger.length / 3));
      if (dist <= Math.max(1, allowedDist)) {
        return true;
      }
    }
    return false;
  }

  for (let i = 0; i <= promptWords.length - triggerWords.length; i++) {
    const window = promptWords.slice(i, i + triggerWords.length).join(" ");
    const dist = levenshtein(window, trigger);
    const allowedDist = Math.min(maxDistance, Math.ceil(trigger.length / 5));
    if (dist <= Math.max(1, allowedDist)) {
      return true;
    }
  }

  return false;
}

/**
 * Parse YAML frontmatter from skill file
 */
export function parseSkillFrontmatter(
  content: string,
): { meta: Record<string, unknown>; body: string } | null {
  const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (!match) return null;

  const yamlContent = match[1];
  const body = match[2].trim();

  const meta: Record<string, unknown> = {};

  const nameMatch = yamlContent.match(/^name:\s*["']?([^"'\n]+)["']?\s*$/m);
  if (nameMatch) meta.name = nameMatch[1].trim();

  const descMatch = yamlContent.match(
    /^description:\s*["']?([^"'\n]+)["']?\s*$/m,
  );
  if (descMatch) meta.description = descMatch[1].trim();

  const triggers: string[] = [];
  const triggerMatch = yamlContent.match(/triggers:\s*\n((?:\s+-\s*.+\n?)*)/);
  if (triggerMatch) {
    const lines = triggerMatch[1].split("\n");
    for (const line of lines) {
      const itemMatch = line.match(/^\s+-\s*["']?([^"'\n]+)["']?\s*$/);
      if (itemMatch) triggers.push(itemMatch[1].trim().toLowerCase());
    }
  }
  meta.triggers = triggers;

  return { meta, body };
}

/**
 * Recursively find all skill files in a directory
 */
export function findSkillFiles(dir: string, files: string[] = []): string[] {
  if (!existsSync(dir)) return files;

  try {
    const entries = readdirSync(dir, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = join(dir, entry.name);
      if (entry.isDirectory()) {
        findSkillFiles(fullPath, files);
      } else if (entry.isFile() && extname(entry.name) === ".md") {
        files.push(fullPath);
      }
    }
  } catch {
    // Ignore errors
  }

  return files;
}

/**
 * Load and parse a skill file
 */
export function loadSkill(path: string): Skill | null {
  try {
    const content = readFileSync(path, "utf-8");
    const parsed = parseSkillFrontmatter(content);

    if (!parsed) return null;

    const { meta, body } = parsed;
    const triggers = (meta.triggers as string[]) || [];

    if (triggers.length === 0) return null;

    return {
      name: (meta.name as string) || basename(path, ".md"),
      path,
      triggers,
      description: meta.description as string | undefined,
      content: body,
    };
  } catch {
    return null;
  }
}

/**
 * Discover all skills from configured locations
 *
 * @param cwd - Project working directory
 * @param pluginRoot - Optional plugin root for finding bundled skills
 */
export function discoverSkills(cwd: string, pluginRoot?: string): Skill[] {
  const skills: Skill[] = [];
  const seenPaths = new Set<string>();

  // Project-local skills (higher priority)
  for (const location of SKILL_LOCATIONS) {
    const dir = join(cwd, location);
    const files = findSkillFiles(dir);
    for (const file of files) {
      if (seenPaths.has(file)) continue;
      seenPaths.add(file);
      const skill = loadSkill(file);
      if (skill) skills.push(skill);
    }
  }

  // Plugin-bundled skills (if pluginRoot provided)
  if (pluginRoot) {
    const pluginSkillDir = join(pluginRoot, "skills");
    const files = findSkillFiles(pluginSkillDir);
    for (const file of files) {
      if (seenPaths.has(file)) continue;
      seenPaths.add(file);
      const skill = loadSkill(file);
      if (skill) skills.push(skill);
    }
  }

  // Global skills (lower priority)
  for (const dir of GLOBAL_SKILL_LOCATIONS) {
    const files = findSkillFiles(dir);
    for (const file of files) {
      if (seenPaths.has(file)) continue;
      seenPaths.add(file);
      const skill = loadSkill(file);
      if (skill) skills.push(skill);
    }
  }

  return skills;
}

/**
 * Find skills matching the prompt (supports typos via Levenshtein distance)
 */
export function matchSkills(
  prompt: string,
  skills: Skill[],
  maxResults = 3,
): Skill[] {
  const promptLower = prompt.toLowerCase();
  const matches: SkillMatchResult[] = [];

  for (const skill of skills) {
    let score = 0;

    for (const trigger of skill.triggers) {
      const triggerLower = trigger.toLowerCase();

      if (promptLower.includes(triggerLower)) {
        score += trigger.length * 2;
      } else if (fuzzyMatchTrigger(promptLower, triggerLower)) {
        score += trigger.length;
      }
    }

    if (score > 0) {
      matches.push({ skill, score });
    }
  }

  return matches
    .sort((a, b) => b.score - a.score)
    .slice(0, maxResults)
    .map((m) => m.skill);
}

/**
 * Format skills for injection into context
 */
export function formatSkillsContext(skills: Skill[]): string {
  const lines = ["<aide-skills>", "", "## Matching Skills", ""];

  for (const skill of skills) {
    lines.push(`### ${skill.name}`);
    if (skill.description) {
      lines.push(`*${skill.description}*`);
    }
    lines.push("");
    lines.push(skill.content);
    lines.push("");
    lines.push("---");
    lines.push("");
  }

  lines.push("</aide-skills>");
  return lines.join("\n");
}
