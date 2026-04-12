---
sidebar_label: Architecture
sidebar_position: 2
title: Architecture
---

# Architecture

AIDE uses a layered architecture where platform-agnostic logic lives in a shared core, and thin adapters map each AI assistant's lifecycle events to core functions.

## System Overview

```
+------------------------------------------------------------------+
|  AI Assistant (Claude Code / OpenCode / ...)                     |
+------------------------------------------------------------------+
          |                              |
  +----------------+            +----------------+
  | Claude Code    |            | OpenCode       |
  | Hooks          |            | Plugin         |
  | (src/hooks/)   |            | (src/opencode/)|
  +----------------+            +----------------+
          |                              |
  +--------------------------------------------------+
  |  Shared Core (src/core/)                         |
  |  session-init, skill-matcher, tool-tracking,     |
  |  tool-enforcement, persistence-logic,            |
  |  session-summary, mcp-sync, write-guard,         |
  |  comment-checker, ...                            |
  +--------------------------------------------------+
          |
  +--------------------------------------------------+
  |  aide binary (Go)                                |
  |  MCP server, BBolt storage, Bleve search,        |
  |  tree-sitter code indexing, static analysis,     |
  |  file watcher, gRPC multiplexing                 |
  +--------------------------------------------------+
```

## Key Principles

### MCP Reads, Hooks Write

AIDE follows a clear separation of concerns:

| Component     | Purpose                                     | Examples                                                    |
| ------------- | ------------------------------------------- | ----------------------------------------------------------- |
| **MCP Tools** | Read state, search, inter-agent messaging   | `memory_search`, `code_search`, `state_get`, `message_send` |
| **Hooks**     | Write state, detect keywords, enforce rules | Session start, keyword detection, tool enforcement          |

This prevents conflicts — the LLM reads via MCP, hooks update state in response to lifecycle events.

### Shared Core

The shared core (`src/core/`) contains all platform-agnostic logic:

- **Session init** — Memory injection, decision loading, state setup
- **Skill matcher** — Fuzzy trigger matching against user prompts
- **Tool tracking** — Monitors which tools are used per session
- **Tool enforcement** — Restricts tools based on agent role (swarm mode)
- **Write guard** — Prevents `Write` tool from overwriting existing files
- **Comment checker** — Detects excessive AI-generated comments
- **Session summary** — Captures end-of-session summaries automatically
- **MCP sync** — Cross-assistant MCP server configuration synchronization

### Platform Adapters

Each AI assistant gets a thin adapter that maps its lifecycle events to core functions:

**Claude Code** (`src/hooks/`):

- Uses Claude's hook system (`PreToolUse`, `PostToolUse`, `UserPromptSubmit`, etc.)
- Generates shell scripts that call into the core
- Native subagent lifecycle support

**OpenCode** (`src/opencode/`):

- Plugin-based integration
- System prompt transformation for skill injection
- Session-based tracking for subagents

## Cross-Assistant MCP Sync

MCP server configurations are automatically synchronized across assistants. When AIDE initializes on any platform, it:

1. Reads MCP configs from Claude Code (`.mcp.json`), OpenCode (`opencode.json`), and the AIDE canonical format (`.aide/config/mcp.json`)
2. Merges all configurations
3. Writes back to each platform's config file

A journal (`.aide/config/mcp-sync.journal.json`) tracks intentional deletions so removed servers aren't re-imported on the next sync.

## The aide Binary

The Go binary (`aide`) provides the heavy-lifting backend:

| Component       | Technology          | Purpose                                         |
| --------------- | ------------------- | ----------------------------------------------- |
| MCP Server      | JSON-RPC over stdio | Exposes 32 tools to the AI                      |
| Storage         | BBolt (embedded)    | Key-value store for all data                    |
| Search          | Bleve (embedded)    | Full-text search with fuzzy matching            |
| Code Indexing   | tree-sitter         | Fast symbol extraction across languages         |
| Static Analysis | Custom analysers    | Complexity, coupling, secrets, clones, security |
| Survey          | BoltDB + Bleve      | Codebase structure, entry points, churn         |
| File Watcher    | fsnotify            | Auto re-index and re-analyse on changes         |
| gRPC            | Multiplexed daemon  | Shared process for multiple sessions            |

The binary is automatically downloaded when the plugin is installed — no separate installation needed. It self-upgrades via `aide upgrade`.
