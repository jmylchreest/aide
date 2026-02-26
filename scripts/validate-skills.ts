#!/usr/bin/env bun
/**
 * validate-skills.ts — Build-time skill linter for aide.
 *
 * Validates all SKILL.md files in the skills/ directory:
 *   - YAML frontmatter has required fields (name, description, triggers)
 *   - Triggers array is non-empty
 *   - No duplicate skill names
 *   - Markdown structure is reasonable
 *
 * Usage:
 *   bun run scripts/validate-skills.ts
 *   bun run scripts/validate-skills.ts --fix  # (future: auto-fix minor issues)
 *
 * Exit code 0 on success, 1 on validation errors.
 */

import { readdirSync, readFileSync, existsSync } from "fs";
import { join, relative } from "path";

// ─── Types ────────────────────────────────────────────────────────────────

interface SkillFrontmatter {
  name?: string;
  description?: string;
  triggers?: string[];
  [key: string]: unknown;
}

interface ValidationError {
  file: string;
  message: string;
  line?: number;
}

// ─── YAML frontmatter parser (simple, avoids external dep) ────────────────

function parseFrontmatter(content: string): {
  data: SkillFrontmatter;
  bodyStart: number;
} {
  const lines = content.split("\n");
  if (lines[0]?.trim() !== "---") {
    return { data: {}, bodyStart: 0 };
  }

  let endIdx = -1;
  for (let i = 1; i < lines.length; i++) {
    if (lines[i]?.trim() === "---") {
      endIdx = i;
      break;
    }
  }

  if (endIdx === -1) {
    return { data: {}, bodyStart: 0 };
  }

  const yamlLines = lines.slice(1, endIdx);
  const data: SkillFrontmatter = {};
  let currentKey = "";
  let currentArray: string[] | null = null;

  for (const line of yamlLines) {
    // Array item: "  - value"
    const arrayMatch = line.match(/^\s+-\s+(.+)$/);
    if (arrayMatch && currentKey) {
      if (!currentArray) {
        currentArray = [];
        data[currentKey] = currentArray;
      }
      currentArray.push(arrayMatch[1].trim());
      continue;
    }

    // Key-value: "key: value"
    const kvMatch = line.match(/^(\w+):\s*(.*)$/);
    if (kvMatch) {
      currentKey = kvMatch[1];
      const value = kvMatch[2].trim();
      currentArray = null;

      if (value) {
        data[currentKey] = value;
      }
      continue;
    }
  }

  return { data, bodyStart: endIdx + 1 };
}

// ─── Validators ───────────────────────────────────────────────────────────

function validateSkill(
  filePath: string,
  content: string
): ValidationError[] {
  const errors: ValidationError[] = [];
  const rel = relative(process.cwd(), filePath);

  // 1. Must have frontmatter
  if (!content.startsWith("---")) {
    errors.push({ file: rel, message: "Missing YAML frontmatter (must start with ---)", line: 1 });
    return errors;
  }

  const { data, bodyStart } = parseFrontmatter(content);

  // 2. Required fields
  if (!data.name || typeof data.name !== "string") {
    errors.push({ file: rel, message: "Missing required field: name" });
  }

  if (!data.description || typeof data.description !== "string") {
    errors.push({ file: rel, message: "Missing required field: description" });
  }

  if (!data.triggers || !Array.isArray(data.triggers) || data.triggers.length === 0) {
    errors.push({ file: rel, message: "Missing or empty required field: triggers (must be a non-empty array)" });
  }

  // 3. Name validation
  if (data.name && typeof data.name === "string") {
    if (!/^[a-z][a-z0-9-]*$/.test(data.name)) {
      errors.push({
        file: rel,
        message: `Skill name "${data.name}" must be lowercase alphanumeric with hyphens (e.g., "code-search")`,
      });
    }
  }

  // 4. Triggers validation
  if (data.triggers && Array.isArray(data.triggers)) {
    for (const trigger of data.triggers) {
      if (typeof trigger !== "string" || trigger.trim().length === 0) {
        errors.push({ file: rel, message: `Invalid trigger: "${trigger}" (must be a non-empty string)` });
      }
    }

    // Check for duplicate triggers within the same skill
    const seen = new Set<string>();
    for (const trigger of data.triggers) {
      const normalized = trigger.toLowerCase().trim();
      if (seen.has(normalized)) {
        errors.push({ file: rel, message: `Duplicate trigger: "${trigger}"` });
      }
      seen.add(normalized);
    }
  }

  // 5. Body must have at least one heading
  const bodyLines = content.split("\n").slice(bodyStart);
  const hasHeading = bodyLines.some((line) => /^#\s+/.test(line));
  if (!hasHeading) {
    errors.push({ file: rel, message: "Skill body should contain at least one markdown heading (# ...)" });
  }

  // 6. Description should not be too long
  if (data.description && typeof data.description === "string" && data.description.length > 200) {
    errors.push({
      file: rel,
      message: `Description is too long (${data.description.length} chars, max 200)`,
    });
  }

  return errors;
}

// ─── Main ─────────────────────────────────────────────────────────────────

function main(): void {
  const skillsDir = join(process.cwd(), "skills");

  if (!existsSync(skillsDir)) {
    console.error("Error: skills/ directory not found");
    process.exit(1);
  }

  const skillDirs = readdirSync(skillsDir, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name);

  if (skillDirs.length === 0) {
    console.error("Warning: No skill directories found in skills/");
    process.exit(0);
  }

  const allErrors: ValidationError[] = [];
  const skillNames = new Map<string, string>(); // name -> file path

  let validated = 0;

  for (const dir of skillDirs) {
    const skillFile = join(skillsDir, dir, "SKILL.md");

    if (!existsSync(skillFile)) {
      allErrors.push({
        file: `skills/${dir}/`,
        message: "Missing SKILL.md file",
      });
      continue;
    }

    const content = readFileSync(skillFile, "utf-8");
    const errors = validateSkill(skillFile, content);
    allErrors.push(...errors);

    // Check for duplicate skill names across all skills.
    const { data } = parseFrontmatter(content);
    if (data.name && typeof data.name === "string") {
      const existing = skillNames.get(data.name);
      if (existing) {
        allErrors.push({
          file: `skills/${dir}/SKILL.md`,
          message: `Duplicate skill name "${data.name}" (also defined in ${existing})`,
        });
      } else {
        skillNames.set(data.name, `skills/${dir}/SKILL.md`);
      }
    }

    validated++;
  }

  // Report results.
  if (allErrors.length === 0) {
    console.log(`✓ ${validated} skills validated successfully`);
    process.exit(0);
  } else {
    console.error(`✗ ${allErrors.length} validation error(s) in ${validated} skills:\n`);
    for (const err of allErrors) {
      const lineInfo = err.line ? `:${err.line}` : "";
      console.error(`  ${err.file}${lineInfo}: ${err.message}`);
    }
    process.exit(1);
  }
}

main();
