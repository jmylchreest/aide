/**
 * Write-existing-file guard logic — platform-agnostic.
 *
 * Prevents the Write tool from being used on files that already exist.
 * When a file exists, the agent should use Edit instead, which is surgical
 * and preserves content the agent may have forgotten about.
 *
 * Using Write on existing files is one of the most common destructive
 * failure modes: the agent rewrites the entire file from memory and
 * drops functions, imports, or sections it forgot about.
 *
 * Used by both Claude Code hooks (PreToolUse) and OpenCode plugin (tool.execute.before).
 */

import { existsSync } from "fs";
import { resolve, isAbsolute, normalize } from "path";
import { debug } from "../lib/logger.js";

const SOURCE = "write-guard";

/** Paths that are allowed to be overwritten via Write even if they exist */
const WRITE_ALLOWED_PATTERNS = [
  // aide internal state files
  /\/.aide\//,
  // dotfiles that are typically fully rewritten
  /\/\.[^/]+rc$/,
  /\/\.env/,
  /\/\.gitignore$/,
  // Lock files (often fully generated)
  /\/package-lock\.json$/,
  /\/bun\.lock$/,
  /\/yarn\.lock$/,
  /\/pnpm-lock\.yaml$/,
  /\/go\.sum$/,
  /\/Cargo\.lock$/,
  // Generated config that's typically overwritten whole
  /\/tsconfig\.json$/,
];

export interface WriteGuardResult {
  /** Whether the Write should be allowed */
  allowed: boolean;
  /** Message explaining why the Write was blocked */
  message?: string;
}

/**
 * Check whether a Write tool call should be allowed.
 *
 * @param toolName - The tool being called (only "Write" is checked)
 * @param toolInput - The tool's input arguments
 * @param cwd - Working directory to resolve relative paths
 * @returns Whether the write is allowed and an explanatory message if blocked
 */
export function checkWriteGuard(
  toolName: string,
  toolInput: Record<string, unknown>,
  cwd: string,
): WriteGuardResult {
  // Only guard the Write tool
  if (toolName.toLowerCase() !== "write") {
    return { allowed: true };
  }

  const filePath =
    (toolInput.filePath as string) ||
    (toolInput.file_path as string) ||
    (toolInput.path as string);

  if (!filePath) {
    return { allowed: true };
  }

  // Resolve the full path
  const resolvedPath = normalize(
    isAbsolute(filePath) ? filePath : resolve(cwd, filePath),
  );

  // Check if file exists
  if (!existsSync(resolvedPath)) {
    // File doesn't exist — Write is the correct tool for new files
    return { allowed: true };
  }

  // Check if this path is in the allowed-overwrite list
  for (const pattern of WRITE_ALLOWED_PATTERNS) {
    if (pattern.test(resolvedPath)) {
      debug(
        SOURCE,
        `Allowing Write to existing file (matches allow pattern): ${filePath}`,
      );
      return { allowed: true };
    }
  }

  // File exists and isn't in the allowed list — block with guidance
  debug(SOURCE, `Blocking Write to existing file: ${filePath}`);
  return {
    allowed: false,
    message:
      `File "${filePath}" already exists. Use the Edit tool instead of Write to make changes to existing files. ` +
      `The Edit tool makes surgical replacements and preserves content you haven't explicitly changed. ` +
      `Write overwrites the entire file, which risks losing code you forgot to include.`,
  };
}
