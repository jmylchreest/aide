---
sidebar_label: MCP Tools
sidebar_position: 3
title: MCP Tools
---

# MCP Tools

AIDE exposes 34 MCP tools organized into 10 groups. All tools are prefixed `aide__` when accessed by the AI (e.g., `aide__memory_search`).

## Memory Tools

| Tool            | Purpose                                        |
| --------------- | ---------------------------------------------- |
| `memory_search` | Full-text fuzzy search across memories         |
| `memory_list`   | List memories, optionally filtered by category |

### memory_search

Searches stored memories using Bleve full-text search with:

- Standard word matching (case-insensitive)
- Fuzzy matching for typos (1 edit distance)
- Prefix matching via edge n-grams (2-15 chars)
- Substring matching via n-grams (3-8 chars)

**Parameters:** `query` (string), `category` (optional), `limit` (optional, default 10)

### memory_list

Returns all memories, optionally filtered by category. Results include timestamps — prefer most recent when values conflict.

**Parameters:** `category` (optional: learning, decision, issue, discovery, blocker), `limit` (optional, default 50)

## Decision Tools

| Tool               | Purpose                                |
| ------------------ | -------------------------------------- |
| `decision_get`     | Get the current decision for a topic   |
| `decision_list`    | List all recorded decisions            |
| `decision_history` | Full chronological history for a topic |

### decision_get

Returns the latest (current) decision for a topic. Decisions are append-only — the most recent entry supersedes all previous versions.

**Parameters:** `topic` (string, kebab-case)

### decision_list

Returns a summary of all decision topics with their current values. Call this first to discover what topics exist.

### decision_history

Returns all versions of a decision in chronological order. Useful for understanding why a decision changed.

**Parameters:** `topic` (string)

## State Tools

| Tool         | Purpose                                 |
| ------------ | --------------------------------------- |
| `state_get`  | Get a state value (global or per-agent) |
| `state_list` | List all state values                   |

### state_get

Retrieves a state value. Common keys: `mode`, `modelTier`, `activeSkill`.

**Parameters:** `key` (string), `agent_id` (optional)

### state_list

Returns all current state entries including global state and per-agent state (prefixed with `agent:<id>:`).

**Parameters:** `agent_id` (optional, to filter)

## Message Tools

| Tool           | Purpose                                          |
| -------------- | ------------------------------------------------ |
| `message_send` | Send a message to another agent or broadcast     |
| `message_list` | List messages for an agent (auto-prunes expired) |
| `message_ack`  | Acknowledge a message as read                    |

### message_send

Sends inter-agent messages. Types: `status`, `request`, `response`, `blocker`, `completion`, `handoff`.

**Parameters:** `from` (string), `content` (string, max 2000 chars), `to` (optional, omit for broadcast), `type` (optional), `ttl_seconds` (optional, default 3600)

### message_list

Returns unread messages for an agent. Expired messages (past TTL) are automatically pruned.

**Parameters:** `agent_id` (string), `include_read` (optional boolean)

### message_ack

Marks a message as read so it won't appear in future `message_list` calls.

**Parameters:** `message_id` (integer), `agent_id` (string)

## Code Tools

| Tool                  | Purpose                           |
| --------------------- | --------------------------------- |
| `code_search`         | Search indexed symbol definitions |
| `code_symbols`        | List all symbols in a file        |
| `code_references`     | Find all call sites of a symbol   |
| `code_stats`          | Get index statistics              |
| `code_outline`        | Get collapsed file outline        |
| `code_top_references` | Rank symbols by reference count   |
| `code_read_check`     | Check if a file is indexed and unchanged |

### code_search

Searches symbol definitions (functions, methods, classes, interfaces, types) using Bleve full-text search. Supports filtering by kind, language, and file path.

**Parameters:** `query` (string), `kind` (optional: function, method, class, interface, type), `lang` (optional), `file` (optional), `limit` (optional, default 20)

### code_symbols

Lists all indexed symbols from a specific file. If the file isn't indexed yet, it will be parsed on-demand.

**Parameters:** `file` (string)

### code_references

Finds all call sites and usages of a symbol. Filter by reference kind (`call`, `type_ref`) and file path.

**Parameters:** `symbol` (string), `kind` (optional), `file` (optional), `limit` (optional, default 50)

### code_stats

Returns the number of indexed files, symbols, and references. Use to check if the codebase has been indexed.

### code_outline

Returns a collapsed file outline with signatures preserved and function/method/class bodies replaced by `{ ... }`. Shows ~5-15% of tokens vs the full file. Line numbers are preserved for targeted reads.

**Parameters:** `file` (string), `keep_comments` (optional boolean)

### code_top_references

Ranks symbols by how many times they are referenced across the codebase. Useful for finding core APIs, shared utilities, and high-impact change targets.

**Parameters:** `kind` (optional: function, method, class, interface, type), `limit` (optional, default 25)

### code_read_check

Checks whether a file is indexed and whether its content has changed since last indexing. Returns freshness status and an estimated token count so you can decide whether to use `code_outline` or `code_symbols` instead of re-reading the full file.

**Parameters:** `file` (string)

**Response fields:** `indexed`, `fresh`, `symbols`, `outline_available`, `estimated_tokens`

## Token Tools (Experimental)

| Tool           | Purpose                            |
| -------------- | ---------------------------------- |
| `token_stats`  | Get estimated token usage statistics |

### token_stats

Returns aggregated estimates of tokens consumed and saved by aide features. All values are **estimates** based on calibrated per-language character ratios — use for relative comparisons, not exact cost accounting.

**Parameters:** `session_id` (optional, filter by session)

**Response fields:** `total_read`, `total_saved`, `event_count`, `by_tool`, `by_saving_type`, `sessions`

## Findings Tools

| Tool              | Purpose                          |
| ----------------- | -------------------------------- |
| `findings_search` | Full-text search across findings |
| `findings_list`   | List findings by filter          |
| `findings_stats`  | Codebase health overview         |
| `findings_accept` | Accept (dismiss) findings        |

### findings_search

Full-text search across static analysis findings.

**Parameters:** `query` (string), `limit` (optional)

### findings_list

List findings filtered by analyser, severity, file, or category.

**Parameters:** `analyser` (optional), `severity` (optional), `file` (optional), `category` (optional), `include_accepted` (optional boolean)

### findings_stats

Returns a codebase health overview with counts by analyser and severity.

**Parameters:** `include_accepted` (optional boolean)

### findings_accept

Accepts (dismisses) findings so they're hidden from future output. Can accept by ID or by filter.

**Parameters:** `ids` (optional array), `analyser` (optional), `severity` (optional), `file` (optional), `all` (optional boolean)

## Task Tools

| Tool            | Purpose                 |
| --------------- | ----------------------- |
| `task_create`   | Create a new swarm task |
| `task_get`      | Get full task details   |
| `task_list`     | List tasks by status    |
| `task_claim`    | Atomically claim a task |
| `task_complete` | Mark a task as done     |
| `task_delete`   | Delete a task           |

### task_create

Creates a new task (starts as `pending`).

**Parameters:** `title` (string), `description` (optional string)

### task_get

Returns full task details including status, assigned agent, and result.

**Parameters:** `id` (string)

### task_list

Lists tasks, optionally filtered by status.

**Parameters:** `status` (optional: pending, claimed, done, blocked)

### task_claim

Atomically claims a pending task for an agent. Prevents two agents from claiming the same task.

**Parameters:** `task_id` (string), `agent_id` (string)

### task_complete

Marks a claimed task as complete with a result summary.

**Parameters:** `task_id` (string), `result` (string)

### task_delete

Deletes a task by ID.

**Parameters:** `id` (string)

## Survey Tools

| Tool            | Purpose                                   |
| --------------- | ----------------------------------------- |
| `survey_search` | Full-text search across survey entries    |
| `survey_list`   | Browse entries by analyzer, kind, or file |
| `survey_stats`  | Aggregate counts by analyzer and kind     |
| `survey_run`    | Execute analyzers to populate survey data |
| `survey_graph`  | Build call graph for a symbol             |

### survey_search

Full-text search across codebase survey entries (module names, tech stack, entry points).

**Parameters:** `query` (string), `analyzer` (optional: topology, entrypoints, churn), `kind` (optional: module, entrypoint, dependency, tech_stack, churn, submodule, workspace, arch_pattern), `file` (optional), `limit` (optional, default 20)

### survey_list

Browse survey entries with optional filters. No search query needed.

**Parameters:** `analyzer` (optional), `kind` (optional), `file` (optional), `limit` (optional, default 100)

### survey_stats

Returns total survey entry count with breakdowns by analyzer and kind. Call this first when asked about codebase structure.

### survey_run

Runs survey analyzers to discover codebase structure. Three analyzers: `topology` (modules, workspaces, tech stack), `entrypoints` (main functions, HTTP handlers), `churn` (git history hotspots).

**Parameters:** `analyzer` (optional: topology, entrypoints, churn -- omit to run all)

### survey_graph

Builds a call graph for a symbol showing callers, callees, or both. Uses BFS traversal over the code index.

**Parameters:** `symbol` (string), `direction` (optional: both, callers, callees -- default both), `max_depth` (optional, default 2), `max_nodes` (optional, default 50)

## Instance Tools

| Tool            | Purpose                                  |
| --------------- | ---------------------------------------- |
| `instance_info` | Get identity and config of this instance |

### instance_info

Returns the resolved project root, working directory, version info, database path, gRPC socket path, operating mode, and process ID. Useful for debugging multi-instance or worktree issues.
