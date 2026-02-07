#!/usr/bin/env node
/**
 * Skill Injector Hook (UserPromptSubmit)
 *
 * Dynamically discovers and injects relevant skills based on prompt triggers.
 * Searches both built-in skills and project-local .aide/skills/
 *
 * Features:
 * - Recursive skill discovery
 * - YAML frontmatter parsing for triggers
 * - Caching with file watcher invalidation
 * - Auto-creates .aide directories if needed
 *
 * Debug logging: Set AIDE_DEBUG=1 to enable tracing
 * Logs written to: .aide/_logs/startup.log
 */

import { existsSync, readFileSync, readdirSync, mkdirSync } from 'fs';
import { join, basename, extname } from 'path';
import { homedir } from 'os';
import { Logger, debug, setDebugCwd } from '../lib/logger.js';
import { readStdin } from '../lib/hook-utils.js';

const SOURCE = 'skill-injector';

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  prompt?: string;
  transcript_path?: string;
  permission_mode?: string;
}

interface HookOutput {
  continue: boolean;
  hookSpecificOutput?: {
    additionalContext?: string;
  };
}

interface Skill {
  name: string;
  path: string;
  triggers: string[];
  description?: string;
  content: string;
}

interface SkillCache {
  skills: Skill[];
  lastScan: number;
}

// Cache for discovered skills (in-memory, resets per process)
const skillCache: Map<string, SkillCache> = new Map();
const CACHE_TTL = 60000; // 1 minute

/**
 * Calculate Levenshtein distance between two strings
 */
function levenshtein(a: string, b: string): number {
  const matrix: number[][] = [];

  if (a.length === 0) return b.length;
  if (b.length === 0) return a.length;

  // Initialize matrix
  for (let i = 0; i <= b.length; i++) {
    matrix[i] = [i];
  }
  for (let j = 0; j <= a.length; j++) {
    matrix[0][j] = j;
  }

  // Fill matrix
  for (let i = 1; i <= b.length; i++) {
    for (let j = 1; j <= a.length; j++) {
      if (b.charAt(i - 1) === a.charAt(j - 1)) {
        matrix[i][j] = matrix[i - 1][j - 1];
      } else {
        matrix[i][j] = Math.min(
          matrix[i - 1][j - 1] + 1, // substitution
          matrix[i][j - 1] + 1,     // insertion
          matrix[i - 1][j] + 1      // deletion
        );
      }
    }
  }

  return matrix[b.length][a.length];
}

/**
 * Check if a trigger fuzzy-matches any word sequence in the prompt
 * Returns true if the trigger matches within allowed edit distance
 */
function fuzzyMatchTrigger(promptLower: string, trigger: string, maxDistance: number = 2): boolean {
  const triggerWords = trigger.split(/\s+/);
  const promptWords = promptLower.split(/\s+/);

  // For single-word triggers, check each prompt word
  if (triggerWords.length === 1) {
    for (const word of promptWords) {
      const dist = levenshtein(word, trigger);
      // Allow more tolerance for longer words
      const allowedDist = Math.min(maxDistance, Math.floor(trigger.length / 3));
      if (dist <= Math.max(1, allowedDist)) {
        return true;
      }
    }
    return false;
  }

  // For multi-word triggers, check sliding windows
  for (let i = 0; i <= promptWords.length - triggerWords.length; i++) {
    const window = promptWords.slice(i, i + triggerWords.length).join(' ');
    const dist = levenshtein(window, trigger);
    // Allow ~1 error per 5 characters for phrases
    const allowedDist = Math.min(maxDistance, Math.ceil(trigger.length / 5));
    if (dist <= Math.max(1, allowedDist)) {
      return true;
    }
  }

  return false;
}

// Module-level logger (initialized in main)
let log: Logger | null = null;

// Skill search locations (relative to cwd)
const SKILL_LOCATIONS = [
  '.aide/skills',           // Project-local
  'skills',                 // Plugin skills (when running from aide repo/plugin)
];

const GLOBAL_SKILL_LOCATIONS = [
  join(homedir(), '.aide', 'skills'),  // User global
];

/**
 * Ensure .aide directories exist
 */
function ensureDirectories(cwd: string): void {
  const dirs = [
    join(cwd, '.aide'),
    join(cwd, '.aide', 'skills'),
    join(cwd, '.aide', 'config'),
    join(cwd, '.aide', 'state'),
    join(cwd, '.aide', 'memory'),
  ];

  for (const dir of dirs) {
    if (!existsSync(dir)) {
      try {
        mkdirSync(dir, { recursive: true });
      } catch {
        // Ignore errors (may not have permission)
      }
    }
  }
}

/**
 * Parse YAML frontmatter from skill file
 */
function parseSkillFrontmatter(content: string): { meta: Record<string, unknown>; body: string } | null {
  const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (!match) return null;

  const yamlContent = match[1];
  const body = match[2].trim();

  // Simple YAML parsing (handles name, description, triggers array)
  const meta: Record<string, unknown> = {};

  // Parse name
  const nameMatch = yamlContent.match(/^name:\s*["']?([^"'\n]+)["']?\s*$/m);
  if (nameMatch) meta.name = nameMatch[1].trim();

  // Parse description
  const descMatch = yamlContent.match(/^description:\s*["']?([^"'\n]+)["']?\s*$/m);
  if (descMatch) meta.description = descMatch[1].trim();

  // Parse triggers array
  const triggers: string[] = [];
  const triggerMatch = yamlContent.match(/triggers:\s*\n((?:\s+-\s*.+\n?)*)/);
  if (triggerMatch) {
    const lines = triggerMatch[1].split('\n');
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
function findSkillFiles(dir: string, files: string[] = [], depth: number = 0): string[] {
  if (!existsSync(dir)) {
    log?.debug(`findSkillFiles: directory does not exist: ${dir}`);
    return files;
  }

  try {
    const entries = readdirSync(dir, { withFileTypes: true });
    log?.debug(`findSkillFiles: scanning ${dir} (${entries.length} entries, depth=${depth})`);

    for (const entry of entries) {
      const fullPath = join(dir, entry.name);

      if (entry.isDirectory()) {
        // Recurse into subdirectories
        findSkillFiles(fullPath, files, depth + 1);
      } else if (entry.isFile() && extname(entry.name) === '.md') {
        files.push(fullPath);
      }
    }
  } catch (err) {
    log?.warn(`findSkillFiles: failed to read directory: ${dir}`, err);
  }

  return files;
}

/**
 * Load and parse a skill file
 */
function loadSkill(path: string): Skill | null {
  try {
    log?.debug(`loadSkill: reading ${path}`);
    const content = readFileSync(path, 'utf-8');
    const parsed = parseSkillFrontmatter(content);

    if (!parsed) {
      log?.debug(`loadSkill: no frontmatter in ${path}`);
      return null;
    }

    const { meta, body } = parsed;
    const triggers = (meta.triggers as string[]) || [];

    // Skill must have at least one trigger
    if (triggers.length === 0) {
      log?.debug(`loadSkill: no triggers in ${path}`);
      return null;
    }

    log?.debug(`loadSkill: loaded ${basename(path)} with ${triggers.length} triggers`);
    return {
      name: (meta.name as string) || basename(path, '.md'),
      path,
      triggers,
      description: meta.description as string | undefined,
      content: body,
    };
  } catch (err) {
    log?.warn(`loadSkill: failed to load ${path}`, err);
    return null;
  }
}

/**
 * Discover all skills from configured locations
 */
function discoverSkills(cwd: string): Skill[] {
  // Check cache
  const cached = skillCache.get(cwd);
  if (cached && Date.now() - cached.lastScan < CACHE_TTL) {
    log?.debug(`discoverSkills: cache hit (${cached.skills.length} skills, age=${Math.round((Date.now() - cached.lastScan) / 1000)}s)`);
    return cached.skills;
  }

  log?.start('discoverSkills');
  log?.debug('discoverSkills: cache miss, scanning...');

  const skills: Skill[] = [];
  const seenPaths = new Set<string>();
  let filesScanned = 0;

  // Project-local skills (higher priority)
  log?.start('discoverSkills:local');
  for (const location of SKILL_LOCATIONS) {
    const dir = join(cwd, location);
    const files = findSkillFiles(dir);
    filesScanned += files.length;

    for (const file of files) {
      if (seenPaths.has(file)) continue;
      seenPaths.add(file);

      const skill = loadSkill(file);
      if (skill) skills.push(skill);
    }
  }
  log?.end('discoverSkills:local', { locations: SKILL_LOCATIONS.length, files: filesScanned });

  // Global skills (lower priority)
  log?.start('discoverSkills:global');
  let globalFiles = 0;
  for (const dir of GLOBAL_SKILL_LOCATIONS) {
    const files = findSkillFiles(dir);
    globalFiles += files.length;

    for (const file of files) {
      if (seenPaths.has(file)) continue;
      seenPaths.add(file);

      const skill = loadSkill(file);
      if (skill) skills.push(skill);
    }
  }
  log?.end('discoverSkills:global', { locations: GLOBAL_SKILL_LOCATIONS.length, files: globalFiles });

  // Update cache
  skillCache.set(cwd, { skills, lastScan: Date.now() });

  log?.end('discoverSkills', { totalSkills: skills.length, totalFiles: filesScanned + globalFiles });
  return skills;
}

/**
 * Find skills matching the prompt (supports typos via Levenshtein distance)
 */
function matchSkills(prompt: string, skills: Skill[], maxResults = 3): Skill[] {
  log?.start('matchSkills');

  const promptLower = prompt.toLowerCase();
  const matches: { skill: Skill; score: number }[] = [];

  for (const skill of skills) {
    let score = 0;

    for (const trigger of skill.triggers) {
      const triggerLower = trigger.toLowerCase();

      // First try exact substring match (highest score)
      if (promptLower.includes(triggerLower)) {
        // Longer triggers = more specific = higher score
        score += trigger.length * 2; // Bonus for exact match
        log?.debug(`matchSkills: exact match "${trigger}" in "${skill.name}"`);
      }
      // Fall back to fuzzy match (lower score)
      else if (fuzzyMatchTrigger(promptLower, triggerLower)) {
        score += trigger.length; // Lower score for fuzzy match
        log?.debug(`matchSkills: fuzzy match "${trigger}" in "${skill.name}"`);
      }
    }

    if (score > 0) {
      matches.push({ skill, score });
      log?.debug(`matchSkills: matched "${skill.name}" (score=${score})`);
    }
  }

  // Sort by score descending, take top N
  const result = matches
    .sort((a, b) => b.score - a.score)
    .slice(0, maxResults)
    .map(m => m.skill);

  log?.end('matchSkills', { checked: skills.length, matched: matches.length, returned: result.length });
  return result;
}

/**
 * Format skills for injection into context
 */
function formatSkillsContext(skills: Skill[]): string {
  const lines = [
    '<aide-skills>',
    '',
    '## Matching Skills',
    '',
  ];

  for (const skill of skills) {
    lines.push(`### ${skill.name}`);
    if (skill.description) {
      lines.push(`*${skill.description}*`);
    }
    lines.push('');
    lines.push(skill.content);
    lines.push('');
    lines.push('---');
    lines.push('');
  }

  lines.push('</aide-skills>');
  return lines.join('\n');
}

// Debug helper - writes to debug.log (not stderr)
function debugLog(msg: string): void {
  debug(SOURCE, msg);
}

// Ensure we always output valid JSON, even on catastrophic errors
function outputContinue(): void {
  try {
    console.log(JSON.stringify({ continue: true }));
  } catch {
    // Last resort - raw JSON string
    console.log('{"continue":true}');
  }
}

// Global error handlers to prevent hook crashes without JSON output
process.on('uncaughtException', (err) => {
  debugLog(`UNCAUGHT EXCEPTION: ${err}`);
  outputContinue();
  process.exit(0);
});

process.on('unhandledRejection', (reason) => {
  debugLog(`UNHANDLED REJECTION: ${reason}`);
  outputContinue();
  process.exit(0);
});

async function main(): Promise<void> {
  const hookStart = Date.now();
  debugLog(`Hook started at ${new Date().toISOString()}`);

  try {
    debugLog('Reading stdin...');
    const input = await readStdin();
    debugLog(`Stdin read complete (${Date.now() - hookStart}ms)`);

    if (!input.trim()) {
      debugLog('Empty input, exiting');
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const prompt = data.prompt || '';
    const cwd = data.cwd || process.cwd();

    // Switch debug logging to project-local logs
    setDebugCwd(cwd);

    debugLog(`Parsed input: cwd=${cwd}, prompt=${prompt.length} chars`);

    // Initialize logger
    log = new Logger('skill-injector', cwd);
    log.start('total');
    log.debug(`Prompt length: ${prompt.length} chars`);
    debugLog(`Logger initialized, enabled=${log.isEnabled()}`);

    // Ensure .aide directories exist
    debugLog('ensureDirectories starting...');
    log.start('ensureDirectories');
    ensureDirectories(cwd);
    log.end('ensureDirectories');
    debugLog(`ensureDirectories complete (${Date.now() - hookStart}ms)`);

    if (!prompt) {
      debugLog('No prompt provided, exiting');
      log.info('No prompt provided');
      log.end('total');
      log.flush();
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    // Discover and match skills
    debugLog('discoverSkills starting...');
    const skills = discoverSkills(cwd);
    debugLog(`discoverSkills complete: ${skills.length} skills (${Date.now() - hookStart}ms)`);

    debugLog('matchSkills starting...');
    const matched = matchSkills(prompt, skills);
    debugLog(`matchSkills complete: ${matched.length} matches (${Date.now() - hookStart}ms)`);

    log.end('total');

    if (matched.length > 0) {
      log.info(`Injecting ${matched.length} skills: ${matched.map(s => s.name).join(', ')}`);
      debugLog(`Flushing logs...`);
      log.flush();
      debugLog(`Hook complete (${Date.now() - hookStart}ms total)`);

      const output: HookOutput = {
        continue: true,
        hookSpecificOutput: {
          additionalContext: formatSkillsContext(matched),
        },
      };
      console.log(JSON.stringify(output));
    } else {
      log.info('No matching skills');
      debugLog(`Flushing logs...`);
      log.flush();
      debugLog(`Hook complete (${Date.now() - hookStart}ms total)`);
      console.log(JSON.stringify({ continue: true }));
    }
  } catch (error) {
    debugLog(`ERROR: ${error}`);
    // Log error if logger is available
    if (log) {
      log.error('Skill injection failed', error);
      log.flush();
    }
    // On error, allow continuation
    console.log(JSON.stringify({ continue: true }));
  }
}

main();
