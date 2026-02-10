/**
 * Comment checker logic — platform-agnostic.
 *
 * Detects excessive or obvious comments in code written by AI agents.
 * Injected as a warning after Write/Edit tool calls to nudge the agent
 * toward cleaner, human-quality code output.
 *
 * Used by both Claude Code hooks (PostToolUse) and OpenCode plugin (tool.execute.after).
 *
 * Philosophy: LLMs over-comment because training data rewards explanation.
 * The single most visible "AI slop" tell is unnecessary comments like
 * "// Initialize the variable" above `let x = 0`. This checker catches
 * those and nudges the agent to remove them without blocking the tool call.
 */

import { debug } from "../lib/logger.js";

const SOURCE = "comment-checker";

/** File extensions we know how to check for comments */
const CHECKABLE_EXTENSIONS = new Set([
  ".ts",
  ".tsx",
  ".js",
  ".jsx",
  ".go",
  ".py",
  ".rs",
  ".java",
  ".c",
  ".cpp",
  ".h",
  ".hpp",
  ".cs",
  ".rb",
  ".swift",
  ".kt",
  ".scala",
  ".php",
  ".vue",
  ".svelte",
]);

/** Extensions that use # for comments */
const HASH_COMMENT_EXTENSIONS = new Set([".py", ".rb", ".sh", ".yaml", ".yml"]);

/** Comment patterns to SKIP (not flag) — these are legitimate */
const LEGITIMATE_PATTERNS = [
  // Directives and pragmas
  /^\s*\/\/\s*(eslint|prettier|ts-ignore|ts-expect-error|ts-nocheck|@ts-|TODO|FIXME|HACK|BUG|XXX|NOTE|IMPORTANT|WARNING)/i,
  /^\s*\/\/\s*noinspection/i,
  /^\s*\/\*\s*(eslint|prettier|global|jshint)/i,
  /^\s*#\s*(type:\s*ignore|noqa|pragma|pylint|mypy)/i,
  /^\s*\/\/\s*region\b/i,
  /^\s*\/\/\s*endregion\b/i,
  // Shebangs
  /^\s*#!/,
  // License/copyright headers
  /^\s*\/\/\s*(copyright|license|SPDX)/i,
  /^\s*#\s*(copyright|license|SPDX)/i,
  // Encoding declarations
  /^\s*#.*coding[:=]/,
  // BDD/test descriptions (describe, it, test blocks in comments)
  /^\s*\/\/\s*(describe|it|test|expect|should|given|when|then)\b/i,
  // JSDoc/docstring openings (/** and """)
  /^\s*\/\*\*/,
  /^\s*"""/,
  /^\s*'''/,
  // Go build tags
  /^\s*\/\/go:build/,
  /^\s*\/\/\+build/,
  // Rust attributes in comments
  /^\s*\/\/!/,
  /^\s*\/\/\s*#\[/,
];

/** Patterns that indicate an OBVIOUS/UNNECESSARY comment */
const OBVIOUS_PATTERNS = [
  // Restating the code in English
  /^\s*\/\/\s*(initialize|initialise|set|create|declare|define|assign|update|increment|decrement|return|import|export|get|fetch|call|invoke|add|remove|delete|check|validate)\s+(the\s+)?\w+/i,
  /^\s*#\s*(initialize|initialise|set|create|declare|define|assign|update|increment|decrement|return|import|export|get|fetch|call|invoke|add|remove|delete|check|validate)\s+(the\s+)?\w+/i,
  // Section dividers that add no info
  /^\s*\/\/\s*[-=*]{3,}\s*$/,
  /^\s*#\s*[-=*]{3,}\s*$/,
  // Empty comments
  /^\s*\/\/\s*$/,
  /^\s*#\s*$/,
  // Repeating the function/variable name
  /^\s*\/\/\s*(function|method|class|interface|type|const|let|var|def|func)\s+/i,
  /^\s*#\s*(function|method|class|interface|type|const|let|var|def|func)\s+/i,
];

export interface CommentCheckResult {
  /** Whether excessive comments were detected */
  hasExcessiveComments: boolean;
  /** Warning message to inject (empty if no issues) */
  warning: string;
  /** Number of suspicious comments found */
  suspiciousCount: number;
  /** Total comments found */
  totalComments: number;
  /** The suspicious comment lines */
  examples: string[];
}

/**
 * Extract the file extension from a path
 */
function getExtension(filePath: string): string {
  const lastDot = filePath.lastIndexOf(".");
  if (lastDot === -1) return "";
  return filePath.slice(lastDot).toLowerCase();
}

/**
 * Check if a line is a legitimate comment (should be kept)
 */
function isLegitimateComment(line: string): boolean {
  return LEGITIMATE_PATTERNS.some((p) => p.test(line));
}

/**
 * Check if a line is an obvious/unnecessary comment
 */
function isObviousComment(line: string): boolean {
  return OBVIOUS_PATTERNS.some((p) => p.test(line));
}

/**
 * Extract comment lines from code content.
 * Returns only single-line comments (// or #), not block comments or docstrings.
 */
function extractCommentLines(
  content: string,
  ext: string,
): { line: string; lineNumber: number }[] {
  const lines = content.split("\n");
  const comments: { line: string; lineNumber: number }[] = [];
  const usesHash = HASH_COMMENT_EXTENSIONS.has(ext);
  let inBlockComment = false;
  let inDocstring = false;

  for (let i = 0; i < lines.length; i++) {
    const trimmed = lines[i].trim();

    // Track block comments (/* ... */)
    if (!inDocstring) {
      if (trimmed.startsWith("/*")) {
        inBlockComment = true;
        // Single-line block comments
        if (trimmed.includes("*/")) {
          inBlockComment = false;
        }
        continue;
      }
      if (inBlockComment) {
        if (trimmed.includes("*/")) {
          inBlockComment = false;
        }
        continue;
      }
    }

    // Track Python/Ruby docstrings
    if (ext === ".py" || ext === ".rb") {
      const tripleQuoteCount = (trimmed.match(/"""|'''/g) || []).length;
      if (tripleQuoteCount === 1) {
        inDocstring = !inDocstring;
        continue;
      }
      if (inDocstring) continue;
    }

    // Single-line comments
    if (trimmed.startsWith("//")) {
      comments.push({ line: lines[i], lineNumber: i + 1 });
    } else if (
      usesHash &&
      trimmed.startsWith("#") &&
      !trimmed.startsWith("#!")
    ) {
      comments.push({ line: lines[i], lineNumber: i + 1 });
    }
  }

  return comments;
}

/**
 * Check code content for excessive or obvious comments.
 *
 * @param filePath - Path to the file being written/edited
 * @param content - The full file content after the write/edit, OR the new content being written
 * @param isNewContent - If true, content is only the new/changed portion (Edit newString)
 */
export function checkComments(
  filePath: string,
  content: string,
  isNewContent = false,
): CommentCheckResult {
  const ext = getExtension(filePath);

  // Skip non-checkable files
  if (!CHECKABLE_EXTENSIONS.has(ext)) {
    return {
      hasExcessiveComments: false,
      warning: "",
      suspiciousCount: 0,
      totalComments: 0,
      examples: [],
    };
  }

  const comments = extractCommentLines(content, ext);
  const suspicious: { line: string; lineNumber: number }[] = [];

  for (const comment of comments) {
    // Skip legitimate comments
    if (isLegitimateComment(comment.line)) continue;

    // Flag obvious/unnecessary comments
    if (isObviousComment(comment.line)) {
      suspicious.push(comment);
    }
  }

  // For full file content: also check comment density
  // (>30% comment lines in non-test files is suspicious)
  const totalLines = content
    .split("\n")
    .filter((l) => l.trim().length > 0).length;
  const isTestFile =
    filePath.includes(".test.") ||
    filePath.includes(".spec.") ||
    filePath.includes("__tests__") ||
    filePath.includes("_test.");
  const commentRatio = totalLines > 0 ? comments.length / totalLines : 0;
  const highDensity =
    !isNewContent && !isTestFile && commentRatio > 0.3 && comments.length > 5;

  const hasExcessiveComments = suspicious.length >= 2 || highDensity;

  if (!hasExcessiveComments) {
    return {
      hasExcessiveComments: false,
      warning: "",
      suspiciousCount: suspicious.length,
      totalComments: comments.length,
      examples: [],
    };
  }

  // Build warning message
  const examples = suspicious.slice(0, 5).map((s) => s.line.trim());
  const parts: string[] = [
    `[aide:comment-checker] Detected ${suspicious.length} potentially unnecessary comment${suspicious.length === 1 ? "" : "s"} in ${filePath}.`,
  ];

  if (highDensity) {
    parts.push(
      `Comment density is ${Math.round(commentRatio * 100)}% (${comments.length}/${totalLines} non-empty lines).`,
    );
  }

  if (examples.length > 0) {
    parts.push("Examples:");
    for (const ex of examples) {
      parts.push(`  ${ex}`);
    }
  }

  parts.push("");
  parts.push(
    "Clean code should be self-documenting. Remove comments that merely restate the code. " +
      "Keep only: TODOs, complex logic explanations, non-obvious workarounds, API docs, and regulatory/compliance notes.",
  );

  return {
    hasExcessiveComments: true,
    warning: parts.join("\n"),
    suspiciousCount: suspicious.length,
    totalComments: comments.length,
    examples,
  };
}

/**
 * Determine if a tool call is a write/edit that should be checked.
 * Returns the file path if checkable, null otherwise.
 */
export function getCheckableFilePath(
  toolName: string,
  toolInput: Record<string, unknown>,
): string | null {
  const toolLower = toolName.toLowerCase();
  if (
    toolLower !== "write" &&
    toolLower !== "edit" &&
    toolLower !== "multiedit" &&
    toolLower !== "notebookedit"
  ) {
    return null;
  }

  const filePath =
    (toolInput.filePath as string) ||
    (toolInput.file_path as string) ||
    (toolInput.path as string);

  if (!filePath) return null;

  const ext = getExtension(filePath);
  if (!CHECKABLE_EXTENSIONS.has(ext)) return null;

  return filePath;
}

/**
 * Get the content to check from a tool's input.
 * For Write: the full content. For Edit: the newString.
 * Returns [content, isNewContent] tuple.
 */
export function getContentToCheck(
  toolName: string,
  toolInput: Record<string, unknown>,
): [string, boolean] | null {
  const toolLower = toolName.toLowerCase();

  if (toolLower === "write") {
    const content = toolInput.content as string;
    return content ? [content, false] : null;
  }

  if (toolLower === "edit") {
    const newString = (toolInput.newString || toolInput.new_string) as string;
    return newString ? [newString, true] : null;
  }

  if (toolLower === "multiedit") {
    // Concatenate all new strings for checking
    const edits = toolInput.edits as
      | Array<{ new_string?: string; newString?: string }>
      | undefined;
    if (!edits || edits.length === 0) return null;
    const combined = edits
      .map((e) => e.new_string || e.newString || "")
      .join("\n");
    return combined ? [combined, true] : null;
  }

  return null;
}

debug(SOURCE, "Comment checker core loaded");
