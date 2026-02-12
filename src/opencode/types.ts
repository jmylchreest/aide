/**
 * OpenCode plugin type definitions.
 *
 * These types represent the OpenCode plugin interface without requiring
 * @opencode-ai/plugin as a dependency. They match the shapes from:
 * https://github.com/sst/opencode/blob/dev/packages/plugin/src/index.ts
 *
 * When building the standalone npm package, these can be replaced with
 * proper imports from @opencode-ai/plugin.
 */

// =============================================================================
// Plugin Input (provided by OpenCode on plugin init)
// =============================================================================

export interface PluginInput {
  /** OpenCode SDK client for API interactions */
  client: OpenCodeClient;
  /** Current project information */
  project: { name?: string; directory?: string };
  /** Current working directory */
  directory: string;
  /** Git worktree root */
  worktree: string;
  /** URL of the running OpenCode server */
  serverUrl: URL;
  /** Bun shell for command execution */
  $: unknown;
}

/** Minimal SDK client interface — only the parts we use */
export interface OpenCodeClient {
  app: {
    log(opts: {
      body: { service: string; level: string; message: string };
    }): Promise<void>;
  };
  session: {
    create(opts: { body: { title?: string } }): Promise<{ id: string }>;
    prompt(opts: {
      path: { id: string };
      body: {
        parts: Array<{ type: string; text: string }>;
        model?: { providerID: string; modelID: string };
      };
    }): Promise<unknown>;
  };
  event: {
    subscribe(): Promise<{
      stream: AsyncIterable<OpenCodeEvent>;
    }>;
  };
}

// =============================================================================
// Events
// =============================================================================

/** Session object as returned in session.created/updated/deleted events */
export interface OpenCodeSession {
  id: string;
  projectID: string;
  directory: string;
  parentID?: string;
  title: string;
  version: string;
  time: {
    created: number;
    updated: number;
    compacting?: number;
  };
}

/** Text part with session context */
export interface OpenCodeTextPart {
  id: string;
  sessionID: string;
  messageID: string;
  type: "text";
  text: string;
  synthetic?: boolean;
  ignored?: boolean;
}

/** Generic part — may be text, tool, step, etc. */
export type OpenCodePart =
  | OpenCodeTextPart
  | {
      id: string;
      sessionID: string;
      messageID: string;
      type: string;
      [key: string]: unknown;
    };

export interface OpenCodeEvent {
  type: string;
  properties: Record<string, unknown>;
}

/** Typed event shapes matching the OpenCode SDK */
export interface EventSessionCreated {
  type: "session.created";
  properties: { info: OpenCodeSession };
}

export interface EventSessionDeleted {
  type: "session.deleted";
  properties: { info: OpenCodeSession };
}

export interface EventSessionIdle {
  type: "session.idle";
  properties: { sessionID: string };
}

export interface EventMessagePartUpdated {
  type: "message.part.updated";
  properties: { part: OpenCodePart; delta?: string };
}

// =============================================================================
// OpenCode Config (command registration)
// =============================================================================

export interface OpenCodeConfig {
  command?: {
    [key: string]: {
      template: string;
      description?: string;
      agent?: string;
      model?: string;
      subtask?: boolean;
    };
  };
  [key: string]: unknown;
}

// =============================================================================
// Hook signatures
// =============================================================================

export interface Hooks {
  /** Generic event listener for all OpenCode events */
  event?: (input: { event: OpenCodeEvent }) => Promise<void>;

  /** Modify OpenCode config (register commands, etc.) */
  config?: (input: OpenCodeConfig) => Promise<void>;

  /** Intercept command execution (slash commands) */
  "command.execute.before"?: (
    input: { command: string; sessionID: string; arguments: string },
    output: {
      parts: Array<{ type: string; text: string; [key: string]: unknown }>;
    },
  ) => Promise<void>;

  /** Modify tool arguments before execution */
  "tool.execute.before"?: (
    input: { tool: string; sessionID: string; callID: string },
    output: { args: Record<string, unknown> },
  ) => Promise<void>;

  /** React after tool completes */
  "tool.execute.after"?: (
    input: { tool: string; sessionID: string; callID: string },
    output: {
      title: string;
      output: string;
      metadata: Record<string, unknown>;
    },
  ) => Promise<void>;

  /** Modify system prompt */
  "experimental.chat.system.transform"?: (
    input: {
      sessionID?: string;
      model: { providerID: string; modelID: string };
    },
    output: { system: string[] },
  ) => Promise<void>;

  /** Inject context during compaction */
  "experimental.session.compacting"?: (
    input: { sessionID: string },
    output: { context: string[]; prompt?: string },
  ) => Promise<void>;

  /** Permission control */
  "permission.ask"?: (
    input: { tool: string; permission: string; patterns: string[] },
    output: { status: "ask" | "deny" | "allow" },
  ) => Promise<void>;

  /** Shell environment injection */
  "shell.env"?: (
    input: { cwd: string },
    output: { env: Record<string, string> },
  ) => Promise<void>;
}

export type Plugin = (input: PluginInput) => Promise<Hooks>;
