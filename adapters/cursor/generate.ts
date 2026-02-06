#!/usr/bin/env node
/**
 * Cursor Adapter Generator
 *
 * Reads aide agents/skills and generates .cursorrules file.
 * Cursor uses a single rules file that influences AI behavior.
 *
 * Usage: npx ts-node adapters/cursor/generate.ts [--output=path]
 */

import { readFileSync, readdirSync, writeFileSync, existsSync } from 'fs';
import { join, basename } from 'path';

interface Agent {
  name: string;
  description: string;
  prompt: string;
}

interface Skill {
  name: string;
  description: string;
  triggers: string[];
  content: string;
}

const ROOT = join(__dirname, '..', '..');
const AGENTS_DIR = join(ROOT, 'src', 'agents');
const SKILLS_DIR = join(ROOT, 'src', 'skills');

/**
 * Parse YAML frontmatter from markdown
 */
function parseFrontmatter(content: string): { meta: Record<string, unknown>; body: string } | null {
  const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (!match) return null;

  const yaml = match[1];
  const body = match[2].trim();
  const meta: Record<string, unknown> = {};

  const nameMatch = yaml.match(/^name:\s*(.+)$/m);
  if (nameMatch) meta.name = nameMatch[1].trim();

  const descMatch = yaml.match(/^description:\s*(.+)$/m);
  if (descMatch) meta.description = descMatch[1].trim();

  const triggers: string[] = [];
  const triggerMatch = yaml.match(/triggers:\s*\n((?:\s+-\s*.+\n?)*)/);
  if (triggerMatch) {
    const lines = triggerMatch[1].split('\n');
    for (const line of lines) {
      const itemMatch = line.match(/^\s+-\s*(.+)$/);
      if (itemMatch) triggers.push(itemMatch[1].trim());
    }
  }
  meta.triggers = triggers;

  return { meta, body };
}

/**
 * Load all agents
 */
function loadAgents(): Agent[] {
  if (!existsSync(AGENTS_DIR)) return [];

  const agents: Agent[] = [];
  const files = readdirSync(AGENTS_DIR).filter(f => f.endsWith('.md'));

  for (const file of files) {
    const content = readFileSync(join(AGENTS_DIR, file), 'utf-8');
    const parsed = parseFrontmatter(content);
    if (!parsed) continue;

    agents.push({
      name: (parsed.meta.name as string) || basename(file, '.md'),
      description: (parsed.meta.description as string) || '',
      prompt: parsed.body,
    });
  }

  return agents;
}

/**
 * Load all skills
 */
function loadSkills(): Skill[] {
  if (!existsSync(SKILLS_DIR)) return [];

  const skills: Skill[] = [];
  const files = readdirSync(SKILLS_DIR).filter(f => f.endsWith('.md'));

  for (const file of files) {
    const content = readFileSync(join(SKILLS_DIR, file), 'utf-8');
    const parsed = parseFrontmatter(content);
    if (!parsed) continue;

    skills.push({
      name: (parsed.meta.name as string) || basename(file, '.md'),
      description: (parsed.meta.description as string) || '',
      triggers: (parsed.meta.triggers as string[]) || [],
      content: parsed.body,
    });
  }

  return skills;
}

/**
 * Generate .cursorrules content
 */
function generateCursorRules(agents: Agent[], skills: Skill[]): string {
  const lines: string[] = [
    '# AIDE - AI Development Environment',
    '# Generated cursor rules from aide agents and skills',
    '',
    '## Core Behaviors',
    '',
  ];

  // Add agent behaviors as conditional rules
  lines.push('### Available Personas');
  lines.push('');
  lines.push('When the user requests a specific persona, adopt that behavior:');
  lines.push('');

  for (const agent of agents) {
    lines.push(`#### ${agent.name}`);
    if (agent.description) {
      lines.push(`*${agent.description}*`);
    }
    lines.push('');
    // Include condensed version of agent prompt
    const condensed = agent.prompt
      .split('\n')
      .filter(l => l.startsWith('#') || l.startsWith('-') || l.startsWith('1.'))
      .slice(0, 10)
      .join('\n');
    lines.push(condensed);
    lines.push('');
  }

  // Add skill triggers
  lines.push('### Skill Triggers');
  lines.push('');
  lines.push('Activate these behaviors when matching keywords are detected:');
  lines.push('');

  for (const skill of skills) {
    if (skill.triggers.length > 0) {
      lines.push(`- **${skill.name}**: Triggered by: ${skill.triggers.join(', ')}`);
      if (skill.description) {
        lines.push(`  - ${skill.description}`);
      }
    }
  }

  lines.push('');
  lines.push('## General Guidelines');
  lines.push('');
  lines.push('- Delegate complex tasks to appropriate personas');
  lines.push('- Use model tiers: fast (simple), balanced (standard), smart (complex)');
  lines.push('- Verify work before claiming completion');
  lines.push('- Keep code changes minimal and focused');

  return lines.join('\n');
}

function main() {
  const args = process.argv.slice(2);
  const outputArg = args.find(a => a.startsWith('--output='));
  const outputPath = outputArg
    ? outputArg.split('=')[1]
    : join(__dirname, '.cursorrules');

  console.log('Loading aide agents and skills...');
  const agents = loadAgents();
  const skills = loadSkills();

  console.log(`Found ${agents.length} agents and ${skills.length} skills`);

  const rules = generateCursorRules(agents, skills);

  writeFileSync(outputPath, rules);
  console.log(`Generated Cursor rules: ${outputPath}`);
  console.log('\nCopy to your project root as .cursorrules');
}

main();
