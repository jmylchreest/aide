#!/usr/bin/env node
/**
 * Memory Capture Hook (PostToolUse, Stop)
 *
 * Watches for <aide-memory> tags in assistant responses and stores them.
 * Also captures session summaries on Stop if autocapture is enabled.
 *
 * Triggers:
 * - <aide-memory category="..." tags="...">...</aide-memory>
 *
 * Storage:
 * - Uses `aide memory add` to persist memories
 */

import { execFileSync } from 'child_process';
import { readFileSync, existsSync } from 'fs';
import { join } from 'path';
import { debug, setDebugCwd } from '../lib/logger.js';
import { readStdin, findAide } from '../lib/hook-utils.js';

const SOURCE = 'memory-capture';

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  tool_input?: Record<string, unknown>;
  tool_response?: string;
  transcript_path?: string;
}

interface MemoryMatch {
  category: string;
  tags: string[];
  content: string;
}

/**
 * Extract <aide-memory> blocks from text
 */
function extractMemories(text: string): MemoryMatch[] {
  const memories: MemoryMatch[] = [];

  // Match <aide-memory category="..." tags="...">...</aide-memory>
  const regex = /<aide-memory\s+(?:category="([^"]*)")?\s*(?:tags="([^"]*)")?\s*>([\s\S]*?)<\/aide-memory>/gi;

  let match;
  while ((match = regex.exec(text)) !== null) {
    const category = match[1] || 'learning';
    const tagsStr = match[2] || '';
    const content = match[3]?.trim() || '';

    if (content.length > 10) {
      memories.push({
        category,
        tags: tagsStr.split(',').map(t => t.trim()).filter(Boolean),
        content
      });
    }
  }

  return memories;
}

/**
 * Store a memory using aide CLI
 */
function storeMemory(cwd: string, memory: MemoryMatch, sessionId: string): boolean {
  const binary = findAide(cwd);
  if (!binary) {
    debug(SOURCE, 'aide binary not found, cannot store memory');
    return false;
  }

  const dbPath = join(cwd, '.aide', 'memory', 'store.db');
  const env = { ...process.env, AIDE_MEMORY_DB: dbPath };

  // Build tags array
  const allTags = [
    ...memory.tags,
    `session:${sessionId.slice(0, 8)}`
  ];

  try {
    const args = [
      'memory', 'add',
      `--category=${memory.category}`,
      `--tags=${allTags.join(',')}`,
      memory.content
    ];

    execFileSync(binary, args, {
      env,
      stdio: 'pipe',
      timeout: 5000
    });

    debug(SOURCE, `Stored memory: ${memory.category}, tags: ${allTags.join(',')}`);
    return true;
  } catch (err) {
    debug(SOURCE, `Failed to store memory: ${err}`);
    return false;
  }
}

/**
 * Get project name from git remote or directory
 */
function getProjectName(cwd: string): string {
  try {
    const gitConfig = join(cwd, '.git', 'config');
    if (existsSync(gitConfig)) {
      const content = readFileSync(gitConfig, 'utf-8');
      const match = content.match(/url\s*=\s*.*[/:]([^/]+?)(?:\.git)?$/m);
      if (match) return match[1];
    }
  } catch { /* ignore */ }

  // Fallback to directory name
  return cwd.split('/').pop() || 'unknown';
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: HookInput = JSON.parse(input);
    const cwd = data.cwd || process.cwd();
    const sessionId = data.session_id || 'unknown';

    setDebugCwd(cwd);
    debug(SOURCE, `Hook triggered: ${data.hook_event_name}`);

    // For PostToolUse, check if the tool response contains memories
    if (data.hook_event_name === 'PostToolUse' && data.tool_response) {
      const memories = extractMemories(data.tool_response);

      if (memories.length > 0) {
        debug(SOURCE, `Found ${memories.length} memories in tool response`);

        for (const memory of memories) {
          storeMemory(cwd, memory, sessionId);
        }
      }
    }

    // For Stop hook, we could do transcript-based capture here
    // (if autocapture is enabled)
    if (data.hook_event_name === 'Stop' && data.transcript_path) {
      debug(SOURCE, 'Stop hook - checking for uncaptured memories in transcript');
      // TODO: Parse transcript and extract any <aide-memory> blocks
      // that might not have been captured by PostToolUse
    }

    console.log(JSON.stringify({ continue: true }));
  } catch (err) {
    debug(SOURCE, `Error: ${err}`);
    console.log(JSON.stringify({ continue: true }));
  }
}

main();
