/**
 * TypeScript bindings for aide
 *
 * Provides two modes:
 * 1. CLI mode (default) - spawns aide CLI commands
 * 2. HTTP mode - connects to running aide server
 *
 * FFI mode (native bindings) requires building the shared library:
 *   cd aide && go build -buildmode=c-shared -o libaide.so ./ffi
 */

import { execFileSync, execSync } from "child_process";
import { join } from "path";

export interface Memory {
  id: string;
  category: string;
  content: string;
  tags: string[];
  priority: number;
  plan: string;
  agent: string;
  createdAt: string;
  updatedAt: string;
}

export interface Task {
  id: string;
  title: string;
  description: string;
  status: "pending" | "claimed" | "done" | "blocked";
  claimedBy: string;
  claimedAt: string;
  completedAt: string;
  result: string;
}

export interface Decision {
  topic: string;
  decision: string;
  rationale: string;
  decidedBy: string;
  createdAt: string;
}

export interface Message {
  id: number;
  from: string;
  to: string;
  content: string;
  readBy: string[];
  createdAt: string;
}

export type Category =
  | "learning"
  | "decision"
  | "issue"
  | "discovery"
  | "blocker";

/**
 * AideMemory client - interfaces with aide via CLI or HTTP
 */
export class AideMemory {
  private mode: "cli" | "http";
  private serverUrl?: string;
  private cliPath: string;

  constructor(
    options: {
      mode?: "cli" | "http";
      serverUrl?: string;
      cliPath?: string;
    } = {},
  ) {
    this.mode = options.mode || "cli";
    this.serverUrl = options.serverUrl || "http://localhost:9876";
    this.cliPath = options.cliPath || this.findCli();
  }

  private findCli(): string {
    // Look in common locations
    const locations = [
      join(process.cwd(), "bin", "aide"),
      join(process.cwd(), "aide", "aide"),
      "aide", // PATH
    ];

    for (const loc of locations) {
      try {
        execSync(`${loc} --help`, { stdio: "ignore" });
        return loc;
      } catch {
        continue;
      }
    }

    return "aide";
  }

  // --- Memory Operations ---

  async addMemory(
    content: string,
    options: { category?: Category; tags?: string[]; plan?: string } = {},
  ): Promise<Memory> {
    if (this.mode === "http") {
      return this.httpPost("/api/memories", {
        content,
        category: options.category || "learning",
        tags: options.tags || [],
        plan: options.plan || "",
      });
    }

    const args = ["memory", "add"];
    if (options.category) args.push(`--category=${options.category}`);
    if (options.tags?.length) args.push(`--tags=${options.tags.join(",")}`);
    if (options.plan) args.push(`--plan=${options.plan}`);
    args.push(content);

    const output = this.runCli(args);
    const match = output.match(/Added memory: (\S+)/);
    return {
      id: match?.[1] || "",
      content,
      category: options.category || "learning",
    } as Memory;
  }

  async searchMemories(query: string, limit = 20): Promise<Memory[]> {
    if (this.mode === "http") {
      return this.httpGet(
        `/api/memories/search?q=${encodeURIComponent(query)}&limit=${limit}`,
      );
    }

    // CLI mode - parse output
    const output = this.runCli(["memory", "search", query]);
    return this.parseMemoryList(output);
  }

  async listMemories(
    options: { category?: Category; plan?: string; limit?: number } = {},
  ): Promise<Memory[]> {
    if (this.mode === "http") {
      const params = new URLSearchParams();
      if (options.category) params.set("category", options.category);
      return this.httpGet(`/api/memories?${params}`);
    }

    const args = ["memory", "list"];
    if (options.category) args.push(`--category=${options.category}`);
    if (options.plan) args.push(`--plan=${options.plan}`);

    const output = this.runCli(args);
    return this.parseMemoryList(output);
  }

  // --- Task Operations ---

  async createTask(title: string, description = ""): Promise<Task> {
    if (this.mode === "http") {
      return this.httpPost("/api/tasks", {
        title,
        description,
        status: "pending",
      });
    }

    const args = ["task", "create", title];
    if (description) args.push(`--description=${description}`);

    const output = this.runCli(args);
    const match = output.match(/Created task: (\S+)/);
    return {
      id: match?.[1] || "",
      title,
      description,
      status: "pending",
    } as Task;
  }

  async claimTask(taskId: string, agentId: string): Promise<Task> {
    if (this.mode === "http") {
      return this.httpPost("/api/tasks/claim", { taskId, agentId });
    }

    this.runCli(["task", "claim", taskId, `--agent=${agentId}`]);
    return { id: taskId, claimedBy: agentId, status: "claimed" } as Task;
  }

  async completeTask(taskId: string, result = ""): Promise<void> {
    if (this.mode === "http") {
      await this.httpPatch(`/api/tasks/${taskId}`, { status: "done", result });
      return;
    }

    const args = ["task", "complete", taskId];
    if (result) args.push(`--result=${result}`);
    this.runCli(args);
  }

  async listTasks(status?: string): Promise<Task[]> {
    if (this.mode === "http") {
      const params = status ? `?status=${status}` : "";
      return this.httpGet(`/api/tasks${params}`);
    }

    const args = ["task", "list"];
    if (status) args.push(`--status=${status}`);

    const output = this.runCli(args);
    return this.parseTaskList(output);
  }

  // --- Decision Operations ---

  async setDecision(
    topic: string,
    decision: string,
    rationale = "",
  ): Promise<Decision> {
    if (this.mode === "http") {
      return this.httpPost("/api/decisions", { topic, decision, rationale });
    }

    const args = ["decision", "set", topic, decision];
    if (rationale) args.push(`--rationale=${rationale}`);
    this.runCli(args);

    return { topic, decision, rationale } as Decision;
  }

  async getDecision(topic: string): Promise<Decision | null> {
    if (this.mode === "http") {
      try {
        return await this.httpGet(
          `/api/decisions/${encodeURIComponent(topic)}`,
        );
      } catch {
        return null;
      }
    }

    try {
      const output = this.runCli(["decision", "get", topic]);
      const match = output.match(/^(.+): (.+)$/m);
      if (match) {
        return { topic: match[1], decision: match[2] } as Decision;
      }
    } catch {
      return null;
    }
    return null;
  }

  // --- Message Operations ---

  async sendMessage(
    content: string,
    from: string,
    to?: string,
  ): Promise<Message> {
    if (this.mode === "http") {
      return this.httpPost("/api/messages", { content, from, to: to || "" });
    }

    const args = ["message", "send", content, `--from=${from}`];
    if (to) args.push(`--to=${to}`);
    this.runCli(args);

    return { content, from, to: to || "" } as Message;
  }

  async getMessages(agentId?: string): Promise<Message[]> {
    if (this.mode === "http") {
      const params = agentId ? `?agent=${agentId}` : "";
      return this.httpGet(`/api/messages${params}`);
    }

    const args = ["message", "list"];
    if (agentId) args.push(`--agent=${agentId}`);

    const output = this.runCli(args);
    return this.parseMessageList(output);
  }

  // --- Helper Methods ---

  private runCli(args: string[]): string {
    try {
      // Use execFileSync to avoid shell interpretation of arguments
      // This prevents command injection from user-provided content
      return execFileSync(this.cliPath, args, {
        encoding: "utf-8",
        timeout: 10000,
      });
    } catch (error: any) {
      throw new Error(`aide CLI error: ${error.message}`);
    }
  }

  private async httpGet<T>(path: string): Promise<T> {
    const response = await fetch(`${this.serverUrl}${path}`);
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${await response.text()}`);
    }
    return response.json() as Promise<T>;
  }

  private async httpPost<T>(path: string, body: object): Promise<T> {
    const response = await fetch(`${this.serverUrl}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${await response.text()}`);
    }
    return response.json() as Promise<T>;
  }

  private async httpPatch<T>(path: string, body: object): Promise<T> {
    const response = await fetch(`${this.serverUrl}${path}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${await response.text()}`);
    }
    return response.json() as Promise<T>;
  }

  private parseMemoryList(output: string): Memory[] {
    const memories: Memory[] = [];
    const lines = output.trim().split("\n");
    for (const line of lines) {
      const match = line.match(/\[(\w+)\] (\S+): (.+)/);
      if (match) {
        memories.push({
          category: match[1],
          id: match[2],
          content: match[3],
        } as Memory);
      }
    }
    return memories;
  }

  private parseTaskList(output: string): Task[] {
    const tasks: Task[] = [];
    const lines = output.trim().split("\n");
    for (const line of lines) {
      const match = line.match(/\[(\w+)\] (\S+): (.+)/);
      if (match) {
        tasks.push({
          status: match[1] as Task["status"],
          id: match[2],
          title: match[3],
        } as Task);
      }
    }
    return tasks;
  }

  private parseMessageList(output: string): Message[] {
    const messages: Message[] = [];
    const lines = output.trim().split("\n");
    for (const line of lines) {
      const match = line.match(/\[(.+?)\] (.+)/);
      if (match) {
        messages.push({
          from: match[1],
          content: match[2],
        } as Message);
      }
    }
    return messages;
  }
}

// Default export for convenience
export const aideMemory = new AideMemory();
