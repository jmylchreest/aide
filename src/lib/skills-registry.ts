/**
 * Skills Registry (skills.sh integration)
 *
 * Manages installation and updates of skills from the skills.sh marketplace.
 * Skills are markdown files with YAML frontmatter that define triggers and behaviors.
 */

import { execFileSync } from "child_process";
import {
  existsSync,
  readFileSync,
  writeFileSync,
  mkdirSync,
  readdirSync,
  copyFileSync,
} from "fs";
import { join, basename } from "path";
import { homedir } from "os";

export interface SkillMetadata {
  name: string;
  version: string;
  source: string;
  installedAt: string;
  updatedAt?: string;
}

export interface SkillsRegistry {
  installed: SkillMetadata[];
  autoUpdate: boolean;
  syncInterval: string;
  lastSync?: string;
}

const REGISTRY_FILE = ".aide/skills/registry.json";
const SKILLS_DIR = ".aide/skills";
const GLOBAL_SKILLS_DIR = join(homedir(), ".aide", "skills");

/**
 * Validate and sanitize a URL for safe fetching
 * Prevents command injection by ensuring only valid http/https URLs
 */
function validateUrl(url: string): string | null {
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return null;
    }
    // Return the properly parsed URL (sanitized)
    return parsed.toString();
  } catch {
    return null;
  }
}

/**
 * Safely fetch content from a URL using curl
 * Uses execFileSync with argument array to prevent command injection
 */
function safeFetch(url: string): string | null {
  const validUrl = validateUrl(url);
  if (!validUrl) {
    console.error(`Invalid URL: ${url}`);
    return null;
  }
  try {
    return execFileSync("curl", ["-sL", validUrl], {
      encoding: "utf-8",
      timeout: 30000,
    });
  } catch (error) {
    console.error(`Failed to fetch ${validUrl}: ${error}`);
    return null;
  }
}

/**
 * Load skills registry
 */
export function loadRegistry(cwd: string): SkillsRegistry {
  const registryPath = join(cwd, REGISTRY_FILE);
  if (existsSync(registryPath)) {
    try {
      return JSON.parse(readFileSync(registryPath, "utf-8"));
    } catch {
      // Return default
    }
  }
  return {
    installed: [],
    autoUpdate: true,
    syncInterval: "24h",
  };
}

/**
 * Save skills registry
 */
export function saveRegistry(cwd: string, registry: SkillsRegistry): void {
  const skillsDir = join(cwd, SKILLS_DIR);
  if (!existsSync(skillsDir)) {
    mkdirSync(skillsDir, { recursive: true });
  }
  writeFileSync(join(cwd, REGISTRY_FILE), JSON.stringify(registry, null, 2));
}

/**
 * Install a skill from skills.sh or a URL
 *
 * Formats:
 *   - skills.sh/<author>/<skill>
 *   - https://github.com/<owner>/<repo>/blob/main/<path>.md
 *   - Local file path
 */
export async function installSkill(
  cwd: string,
  source: string,
  options: { global?: boolean } = {},
): Promise<SkillMetadata | null> {
  const targetDir = options.global ? GLOBAL_SKILLS_DIR : join(cwd, SKILLS_DIR);

  // Ensure target directory exists
  if (!existsSync(targetDir)) {
    mkdirSync(targetDir, { recursive: true });
  }

  let content: string;
  let name: string;
  let version = "1.0.0";

  // Handle different source formats
  if (
    source.startsWith("skills.sh/") ||
    source.startsWith("https://skills.sh/")
  ) {
    // skills.sh marketplace format: skills.sh/author/skill
    const skillPath = source
      .replace("skills.sh/", "")
      .replace("https://skills.sh/", "");
    const url = `https://skills.sh/api/skills/${skillPath}/raw`;

    const fetched = safeFetch(url);
    if (!fetched) {
      console.error(`Failed to fetch skill from skills.sh`);
      return null;
    }
    content = fetched;
    name = skillPath.split("/").pop() || "unknown";
  } else if (source.startsWith("https://github.com/")) {
    // GitHub raw file
    const rawUrl = source
      .replace("github.com", "raw.githubusercontent.com")
      .replace("/blob/", "/");

    const fetched = safeFetch(rawUrl);
    if (!fetched) {
      console.error(`Failed to fetch skill from GitHub`);
      return null;
    }
    content = fetched;
    name = basename(source, ".md");
  } else if (source.startsWith("https://") || source.startsWith("http://")) {
    // Direct URL
    const fetched = safeFetch(source);
    if (!fetched) {
      console.error(`Failed to fetch skill`);
      return null;
    }
    content = fetched;
    name = basename(source, ".md");
  } else if (existsSync(source)) {
    // Local file
    content = readFileSync(source, "utf-8");
    name = basename(source, ".md");
  } else {
    console.error(`Invalid skill source: ${source}`);
    return null;
  }

  // Parse skill to extract metadata
  const meta = parseSkillFrontmatter(content);
  if (meta) {
    name = meta.name || name;
    version = meta.version || version;
  }

  // Write skill file
  const skillPath = join(targetDir, `${name}.md`);
  writeFileSync(skillPath, content);

  // Update registry
  const registry = loadRegistry(cwd);
  const existing = registry.installed.findIndex((s) => s.name === name);

  const metadata: SkillMetadata = {
    name,
    version,
    source,
    installedAt: new Date().toISOString(),
  };

  if (existing >= 0) {
    metadata.installedAt = registry.installed[existing].installedAt;
    metadata.updatedAt = new Date().toISOString();
    registry.installed[existing] = metadata;
  } else {
    registry.installed.push(metadata);
  }

  saveRegistry(cwd, registry);

  console.log(`Installed skill: ${name} (${version})`);
  return metadata;
}

/**
 * Uninstall a skill
 */
export function uninstallSkill(cwd: string, name: string): boolean {
  const registry = loadRegistry(cwd);
  const index = registry.installed.findIndex((s) => s.name === name);

  if (index < 0) {
    console.error(`Skill not installed: ${name}`);
    return false;
  }

  // Remove file
  const skillPath = join(cwd, SKILLS_DIR, `${name}.md`);
  if (existsSync(skillPath)) {
    try {
      require("fs").unlinkSync(skillPath);
    } catch {
      // Ignore
    }
  }

  // Update registry
  registry.installed.splice(index, 1);
  saveRegistry(cwd, registry);

  console.log(`Uninstalled skill: ${name}`);
  return true;
}

/**
 * List installed skills
 */
export function listSkills(cwd: string): SkillMetadata[] {
  const registry = loadRegistry(cwd);
  return registry.installed;
}

/**
 * Update all skills (if autoUpdate is enabled)
 */
export async function updateSkills(cwd: string): Promise<void> {
  const registry = loadRegistry(cwd);

  if (!registry.autoUpdate) {
    console.log("Auto-update is disabled");
    return;
  }

  console.log("Updating skills...");

  for (const skill of registry.installed) {
    if (
      skill.source.startsWith("skills.sh/") ||
      skill.source.startsWith("https://")
    ) {
      await installSkill(cwd, skill.source);
    }
  }

  registry.lastSync = new Date().toISOString();
  saveRegistry(cwd, registry);

  console.log("Skills updated");
}

/**
 * Search for skills (placeholder - would query skills.sh API)
 */
export async function searchSkills(
  query: string,
): Promise<Array<{ name: string; description: string; author: string }>> {
  // In a real implementation, this would query the skills.sh API
  console.log(`Searching for skills matching: ${query}`);
  console.log("Note: skills.sh integration requires API access");

  return [];
}

/**
 * Parse YAML frontmatter from skill content
 */
function parseSkillFrontmatter(
  content: string,
): { name?: string; version?: string; description?: string } | null {
  const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---/);
  if (!match) return null;

  const yaml = match[1];
  const result: { name?: string; version?: string; description?: string } = {};

  const nameMatch = yaml.match(/^name:\s*(.+)$/m);
  if (nameMatch) result.name = nameMatch[1].trim();

  const versionMatch = yaml.match(/^version:\s*(.+)$/m);
  if (versionMatch) result.version = versionMatch[1].trim();

  const descMatch = yaml.match(/^description:\s*(.+)$/m);
  if (descMatch) result.description = descMatch[1].trim();

  return result;
}
