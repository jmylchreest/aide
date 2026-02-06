#!/usr/bin/env node
/**
 * Permission Handler Hook (PermissionRequest)
 *
 * Validates Bash commands before permission prompts are shown.
 * Can auto-approve safe commands or block dangerous ones.
 *
 * PermissionRequest data from Claude Code:
 * - tool_name: "Bash"
 * - tool_input: { command: "...", ... }
 * - cwd, session_id
 *
 * Returns:
 * - { allow: true } to auto-approve
 * - { allow: false, reason: "..." } to block
 * - { continue: true } to show normal permission prompt
 */

import { readStdin, findAideMemory, runAideMemory, shellEscape } from '../lib/hook-utils.js';

interface PermissionRequestInput {
  event: 'PermissionRequest';
  tool_name: string;
  tool_input: {
    command?: string;
    [key: string]: unknown;
  };
  cwd: string;
  session_id: string;
}

interface PermissionResponse {
  allow?: boolean;
  reason?: string;
  continue?: boolean;
}

// Commands that are always safe to auto-approve
const SAFE_COMMANDS = [
  /^ls\b/,
  /^pwd$/,
  /^echo\b/,
  /^cat\b/,
  /^head\b/,
  /^tail\b/,
  /^wc\b/,
  /^grep\b/,
  /^find\b/,
  /^which\b/,
  /^git\s+(status|log|diff|branch|show)\b/,
  /^git\s+stash\s+list\b/,
  /^npm\s+(list|ls|outdated|view)\b/,
  /^yarn\s+(list|info)\b/,
  /^pnpm\s+(list|outdated)\b/,
  /^node\s+--version$/,
  /^npm\s+--version$/,
  /^python\s+--version$/,
  /^go\s+version$/,
  /^cargo\s+--version$/,
];

// Commands that should be blocked without prompting
const BLOCKED_COMMANDS = [
  /rm\s+-rf\s+[\/~]/,           // rm -rf / or ~
  /rm\s+-rf\s+\*/,              // rm -rf *
  /dd\s+.*of=\/dev\//,          // dd to device
  /mkfs\./,                     // format filesystem
  /:\(\)\{:\|:&\};:/,          // fork bomb
  />\s*\/dev\/sd[a-z]/,        // write to disk device
  /curl.*\|\s*(ba)?sh/,        // curl pipe to shell
  /wget.*\|\s*(ba)?sh/,        // wget pipe to shell
];

/**
 * Log permission decision to aide-memory
 */
function logPermission(cwd: string, command: string, decision: string): void {
  if (!findAideMemory(cwd)) return;

  const safeCommand = shellEscape(command).slice(0, 200);
  runAideMemory(cwd, [
    'message',
    'send',
    `${decision}: ${safeCommand}`,
    '--from=system',
    '--type=permission',
  ]);
}

/**
 * Check if command matches any patterns
 */
function matchesPatterns(command: string, patterns: RegExp[]): boolean {
  return patterns.some((pattern) => pattern.test(command));
}

async function main(): Promise<void> {
  try {
    const input = await readStdin();
    if (!input.trim()) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const data: PermissionRequestInput = JSON.parse(input);

    // Only handle Bash permissions
    if (data.tool_name !== 'Bash' || !data.tool_input?.command) {
      console.log(JSON.stringify({ continue: true }));
      return;
    }

    const command = data.tool_input.command;
    const cwd = data.cwd || process.cwd();

    // Check for blocked commands first
    if (matchesPatterns(command, BLOCKED_COMMANDS)) {
      logPermission(cwd, command, 'BLOCKED');
      const response: PermissionResponse = {
        allow: false,
        reason: 'This command has been blocked for safety. It matches a dangerous pattern.',
      };
      console.log(JSON.stringify(response));
      return;
    }

    // Check for safe commands to auto-approve
    if (matchesPatterns(command, SAFE_COMMANDS)) {
      logPermission(cwd, command, 'AUTO-APPROVED');
      const response: PermissionResponse = {
        allow: true,
      };
      console.log(JSON.stringify(response));
      return;
    }

    // Default: show normal permission prompt
    console.log(JSON.stringify({ continue: true }));
  } catch (error) {
    // On error, show normal permission prompt
    console.log(JSON.stringify({ continue: true }));
  }
}

main();
