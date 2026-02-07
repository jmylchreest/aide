#!/usr/bin/env node
/**
 * Memory Capture Hook (PostToolUse, Stop)
 *
 * Watches for <aide-memory> and <aide-decision> tags in assistant responses.
 * Also captures session summaries on Stop.
 *
 * Triggers:
 * - <aide-memory category="..." tags="...">...</aide-memory>
 * - <aide-decision topic="...">...</aide-decision>
 *
 * Storage:
 * - Uses `aide memory add` for memories
 * - Uses `aide decision set` for decisions
 */

import { execFileSync } from "child_process";
import { readFileSync, existsSync } from "fs";
import { join } from "path";
import { debug, setDebugCwd } from "../lib/logger.js";
import { readStdin, findAideBinary } from "../lib/hook-utils.js";

const SOURCE = "memory-capture";

// Safety limits to prevent regex catastrophic backtracking
const MAX_INPUT_LENGTH = 100000;
const MIN_MEMORY_LENGTH = 10;
const MIN_DECISION_LENGTH = 10;

interface HookInput {
  hook_event_name: string;
  session_id: string;
  cwd: string;
  tool_name?: string;
  tool_input?: Record<string, unknown>;
  tool_response?: string;
  transcript_path?: string;
  stop_hook_active?: boolean;
}

interface MemoryMatch {
  category: string;
  tags: string[];
  content: string;
}

interface DecisionMatch {
  topic: string;
  decision: string;
  rationale: string;
}

/**
 * Extract <aide-memory> blocks from text
 */
function extractMemories(text: string): MemoryMatch[] {
  const memories: MemoryMatch[] = [];

  // Safety check to prevent regex catastrophic backtracking
  if (text.length > MAX_INPUT_LENGTH) {
    debug(
      SOURCE,
      `Input too large for safe regex parsing (${text.length} chars)`,
    );
    return memories;
  }

  // Match <aide-memory ...>...</aide-memory> with attributes in any order
  const memoryRegex = /<aide-memory\s+([^>]*)>([\s\S]*?)<\/aide-memory>/gi;

  let match;
  while ((match = memoryRegex.exec(text)) !== null) {
    const attrs = match[1] || "";
    const categoryMatch = attrs.match(/category="([^"]*)"/);
    const tagsMatch = attrs.match(/tags="([^"]*)"/);
    const category = categoryMatch ? categoryMatch[1] : "learning";
    const tagsStr = tagsMatch ? tagsMatch[1] : "";
    const content = match[2]?.trim() || "";

    if (content.length > MIN_MEMORY_LENGTH) {
      memories.push({
        category,
        tags: tagsStr
          .split(",")
          .map((t) => t.trim())
          .filter(Boolean),
        content,
      });
    }
  }

  return memories;
}

/**
 * Extract <aide-decision> blocks from text
 */
function extractDecisions(text: string): DecisionMatch[] {
  const decisions: DecisionMatch[] = [];

  // Safety check to prevent regex catastrophic backtracking
  if (text.length > MAX_INPUT_LENGTH) {
    debug(
      SOURCE,
      `Input too large for safe regex parsing (${text.length} chars)`,
    );
    return decisions;
  }

  // Match <aide-decision topic="...">...</aide-decision>
  const regex =
    /<aide-decision\s+topic="([^"]+)">([\s\S]*?)<\/aide-decision>/gi;

  let match;
  while ((match = regex.exec(text)) !== null) {
    const topic = match[1]?.trim();
    const content = match[2]?.trim() || "";

    if (!topic || content.length < MIN_DECISION_LENGTH) continue;

    // Parse the content to extract decision and rationale
    // Expected format:
    // ## Decision
    // <decision text>
    // ## Rationale
    // <rationale text>
    // ## Alternatives Considered (optional)
    // ...

    let decision = "";
    let rationale = "";

    // Try to extract structured sections
    const decisionMatch = content.match(
      /##\s*Decision\s*\n([\s\S]*?)(?=##|$)/i,
    );
    const rationaleMatch = content.match(
      /##\s*Rationale\s*\n([\s\S]*?)(?=##|$)/i,
    );

    if (decisionMatch) {
      decision = decisionMatch[1].trim();
    } else {
      // If no ## Decision header, use the first line or paragraph
      const lines = content.split("\n").filter((l) => l.trim());
      decision = lines[0] || content.slice(0, 200);
    }

    if (rationaleMatch) {
      rationale = rationaleMatch[1].trim();
    }

    if (decision) {
      decisions.push({ topic, decision, rationale });
    }
  }

  return decisions;
}

/**
 * Store a memory using aide CLI
 */
function storeMemory(
  cwd: string,
  memory: MemoryMatch,
  sessionId: string,
): boolean {
  const binary = findAideBinary(cwd);
  if (!binary) {
    debug(SOURCE, "aide binary not found, cannot store memory");
    return false;
  }

  const dbPath = join(cwd, ".aide", "memory", "store.db");
  const env = { ...process.env, AIDE_MEMORY_DB: dbPath };

  // Build tags array
  const allTags = [...memory.tags, `session:${sessionId.slice(0, 8)}`];

  try {
    const args = [
      "memory",
      "add",
      `--category=${memory.category}`,
      `--tags=${allTags.join(",")}`,
      memory.content,
    ];

    execFileSync(binary, args, {
      env,
      stdio: "pipe",
      timeout: 5000,
    });

    debug(
      SOURCE,
      `Stored memory: ${memory.category}, tags: ${allTags.join(",")}`,
    );
    return true;
  } catch (err) {
    debug(SOURCE, `Failed to store memory: ${err}`);
    return false;
  }
}

/**
 * Store a decision using aide CLI
 */
function storeDecision(cwd: string, decision: DecisionMatch): boolean {
  const binary = findAideBinary(cwd);
  if (!binary) {
    debug(SOURCE, "aide binary not found, cannot store decision");
    return false;
  }

  const dbPath = join(cwd, ".aide", "memory", "store.db");
  const env = { ...process.env, AIDE_MEMORY_DB: dbPath };

  try {
    const args = ["decision", "set", decision.topic, decision.decision];

    if (decision.rationale) {
      args.push(`--rationale=${decision.rationale}`);
    }

    execFileSync(binary, args, {
      env,
      stdio: "pipe",
      timeout: 5000,
    });

    debug(SOURCE, `Stored decision: ${decision.topic}`);
    return true;
  } catch (err) {
    debug(SOURCE, `Failed to store decision: ${err}`);
    return false;
  }
}

/**
 * Generate and store a session summary from transcript
 */
function captureSessionSummary(
  cwd: string,
  sessionId: string,
  transcriptPath: string,
): boolean {
  const binary = findAideBinary(cwd);
  if (!binary) {
    debug(SOURCE, "aide binary not found, cannot capture session summary");
    return false;
  }

  if (!existsSync(transcriptPath)) {
    debug(SOURCE, `Transcript not found: ${transcriptPath}`);
    return false;
  }

  try {
    // Read transcript
    const transcript = readFileSync(transcriptPath, "utf-8");
    const lines = transcript.split("\n").filter((l) => l.trim());

    if (lines.length < 5) {
      debug(SOURCE, "Transcript too short for summary");
      return false;
    }

    // Parse transcript entries
    // Transcript format is JSONL with various entry types
    interface TranscriptEntry {
      type?: string;
      tool_name?: string;
      tool_input?: { file_path?: string; [key: string]: unknown };
      content?:
        | string
        | {
            text?: string;
            tool_use?: { name?: string; input?: { file_path?: string } };
          };
    }

    const entries: TranscriptEntry[] = [];
    for (const line of lines) {
      try {
        const entry = JSON.parse(line) as TranscriptEntry;
        entries.push(entry);
      } catch {
        // Skip malformed lines
      }
    }

    // Extract key information
    const filesModified = new Set<string>();
    const toolsUsed = new Set<string>();
    const userMessages: string[] = [];

    for (const entry of entries) {
      // Track tool usage
      const contentObj =
        typeof entry.content === "object" ? entry.content : null;
      if (
        entry.type === "tool_use" ||
        (entry.type === "assistant" && contentObj?.tool_use)
      ) {
        const toolName = entry.tool_name || contentObj?.tool_use?.name;
        if (toolName) toolsUsed.add(toolName);

        // Track file modifications
        const toolInput = entry.tool_input || contentObj?.tool_use?.input;
        if (toolInput?.file_path && toolName) {
          if (["Write", "Edit"].includes(toolName)) {
            filesModified.add(toolInput.file_path);
          }
        }
      }

      // Track user messages
      if (entry.type === "human" || entry.type === "user") {
        const text =
          typeof entry.content === "string" ? entry.content : contentObj?.text;
        if (text && text.length > 10 && text.length < 500) {
          userMessages.push(text.slice(0, 200));
        }
      }
    }

    // Only create summary if there was meaningful activity
    if (filesModified.size === 0 && toolsUsed.size < 3) {
      debug(SOURCE, "Not enough activity for session summary");
      return false;
    }

    // Build summary
    const summaryParts: string[] = [];

    if (userMessages.length > 0) {
      summaryParts.push(
        `## Tasks\n${userMessages
          .slice(0, 3)
          .map((m) => `- ${m}`)
          .join("\n")}`,
      );
    }

    if (filesModified.size > 0) {
      const files = Array.from(filesModified).slice(0, 10);
      summaryParts.push(
        `## Files Modified\n${files.map((f) => `- ${f}`).join("\n")}`,
      );
    }

    if (toolsUsed.size > 0) {
      summaryParts.push(`## Tools Used\n${Array.from(toolsUsed).join(", ")}`);
    }

    const summary = summaryParts.join("\n\n");

    if (summary.length < 50) {
      debug(SOURCE, "Summary too short, skipping");
      return false;
    }

    // Store as memory
    const dbPath = join(cwd, ".aide", "memory", "store.db");
    const env = { ...process.env, AIDE_MEMORY_DB: dbPath };
    const tags = `session-summary,session:${sessionId.slice(0, 8)}`;

    execFileSync(
      binary,
      ["memory", "add", "--category=session", `--tags=${tags}`, summary],
      {
        env,
        stdio: "pipe",
        timeout: 5000,
      },
    );

    debug(SOURCE, `Stored session summary for ${sessionId.slice(0, 8)}`);
    return true;
  } catch (err) {
    debug(SOURCE, `Failed to capture session summary: ${err}`);
    return false;
  }
}

/**
 * Get project name from git remote or directory
 */
function getProjectName(cwd: string): string {
  try {
    const gitConfig = join(cwd, ".git", "config");
    if (existsSync(gitConfig)) {
      const content = readFileSync(gitConfig, "utf-8");
      const match = content.match(/url\s*=\s*.*[/:]([^/]+?)(?:\.git)?$/m);
      if (match) return match[1];
    }
  } catch {
    /* ignore */
  }

  // Fallback to directory name
  return cwd.split("/").pop() || "unknown";
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
    const sessionId = data.session_id || "unknown";

    setDebugCwd(cwd);
    debug(SOURCE, `Hook triggered: ${data.hook_event_name}`);

    // For PostToolUse, check if the tool response contains memories or decisions
    if (data.hook_event_name === "PostToolUse" && data.tool_response) {
      // Extract and store memories
      const memories = extractMemories(data.tool_response);
      if (memories.length > 0) {
        debug(SOURCE, `Found ${memories.length} memories in tool response`);
        for (const memory of memories) {
          storeMemory(cwd, memory, sessionId);
        }
      }

      // Extract and store decisions
      const decisions = extractDecisions(data.tool_response);
      if (decisions.length > 0) {
        debug(SOURCE, `Found ${decisions.length} decisions in tool response`);
        for (const decision of decisions) {
          storeDecision(cwd, decision);
        }
      }
    }

    // For Stop hook, capture session summary
    if (data.hook_event_name === "Stop" && data.transcript_path) {
      // Don't capture if stop hook is already active (avoid recursion)
      if (!data.stop_hook_active) {
        debug(SOURCE, "Stop hook - capturing session summary");
        captureSessionSummary(cwd, sessionId, data.transcript_path);
      }
    }

    console.log(JSON.stringify({ continue: true }));
  } catch (err) {
    debug(SOURCE, `Error: ${err}`);
    console.log(JSON.stringify({ continue: true }));
  }
}


process.on("uncaughtException", () => {
  try { console.log(JSON.stringify({ continue: true })); } catch { console.log('{"continue":true}'); }
  process.exit(0);
});
process.on("unhandledRejection", () => {
  try { console.log(JSON.stringify({ continue: true })); } catch { console.log('{"continue":true}'); }
  process.exit(0);
});

main();
