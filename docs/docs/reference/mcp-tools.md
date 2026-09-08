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
| `decision_list`    | List recorded decisions (`origin` to widen) |
| `decision_history` | Full chronological history for a topic |

### decision_get

Returns the latest (current) decision for a topic. Decisions are append-only — the most recent entry supersedes all previous versions.

**Parameters:** `topic` (string, kebab-case)

### decision_list

Returns a summary of this project's decision topics with their current values. Call this first to discover what topics exist.

**Parameters:** `origin` (string, optional) — `parent`, `peer`, or `all`

By default only this project's own decisions are returned. Pass `origin` to also
list rules in force here but stored upstream: `parent` for anchor-chain
ancestors, `peer` for subscriptions, `all` for both. Inherited decisions are
reported in a separate section with their source, and never shadow a local
decision on the same topic. They are read-only from here — use `decision_adopt`
to copy one into this project.

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

Returns a collapsed file outline with signatures preserved and function/method/class bodies replaced by `{ ... }`. Output size depends on file structure and grammar support. Line numbers are preserved for targeted reads. The outline is parsed and rendered from the same source snapshot.

**Parameters:** `file` (string), `keep_comments` (optional boolean)

### code_read_symbol

Reads current definitions by name. Without `file`, the index locates candidate files; each candidate is parsed from the bytes used to render the result. Duplicate names return an error listing candidates rather than choosing the first match. An explicit `file` works without an index; add `start_line` to distinguish definitions within that file.

**Parameters:** `symbol` (string), `symbols` (optional batch of up to 10 names), `kind` (optional), `file` (optional exact path), `start_line` (optional current definition line; requires `file`).

Outline and symbol observations carry `source_references`: exact file byte sizes and SHA-256 hashes of the retrieved snapshots. A batch records each reference file once. These are conditional full-file comparisons, not proof of avoided reads or provider savings, and do not populate the historical `tokens_saved` field. The result also carries an `aide/retrieval` protocol metadata receipt with a unique ID, tool name, source references and a checksum of its text. Host observers attach this evidence to their real invocation/session/window only when the receipt survives and the returned text matches. Hosts may drop metadata or transform output, so a missing match remains unverified; the receipt is not added to the textual result. OpenCode protocol results are observed before any subsequent host formatting or truncation.

For reproducible text measurements, source-evidence checks and the limits of
those results, see [Retrieval experiments](./retrieval-experiments.md).

### code_top_references

Ranks symbols by how many times they are referenced across the codebase. Useful for finding core APIs, shared utilities, and high-impact change targets.

**Parameters:** `kind` (optional: function, method, class, interface, type), `limit` (optional, default 25)

### code_read_check

Checks whether a file is indexed and whether its modification time matches the index. This does not prove that its content is unchanged or that the agent has read the current version. Returns index status and a calibrated full-file token estimate.

**Parameters:** `file` (string)

**Response fields:** `indexed`, `fresh`, `symbols`, `outline_available`, `estimated_tokens`

## Token Tools (Experimental)

| Tool           | Purpose                            |
| -------------- | ---------------------------------- |
| `token_stats`  | Get estimated token usage statistics |

### token_stats

Returns observed UTF-8 text accounting and historical token estimates. The versioned `accounting` object separates host/server observations and generated arguments, identifies the token estimator, and reports legacy events and missing evidence. Stages can overlap; do not sum them. Missing accounting means an older server. These observations do not establish final delivery, avoided calls or provider savings.

**Parameters:** `session_id` (optional, filter by session)

**Response fields:** `accounting`, `total_read`, `total_saved`, `event_count`, `by_tool`, `by_saving_type`, `sessions`. Existing totals remain compatibility estimates; `total_saved` and related saved fields are explicitly legacy comparison estimates, not verified savings.

Native tool event attributes include `context_status` and, when known, `context_epoch` and `context_continuity`. Windows are isolated by host, session and actor within the project. Confirmed clear/compaction starts a new window; pending compaction suspends prior-read hints. Resume without verified continuity starts a new observation window labelled `unknown`, without asserting that context was lost. Cache expiry alone does not reset it. Hosts without the required lifecycle evidence retain unknown coverage.

`accounting.transformations` contains paired text changes, separated into `rewrite_candidate` (proposed Claude-compatible replacements) and `adapter_change` (changes made by aide's OpenCode adapter). Each pair contributes its measured before/after bytes and centrally estimated token delta once. Negative reductions preserve annotation overhead. Window details are bounded to 64 groups, with omitted-window and missing-evidence indicators; stage totals cover all selected pairs. The same data reaches CLI `token stats --details` and web Details/Accounting. Final delivery, provider savings and inferred avoided calls remain unverified. `accounting.retrievals` separately reports conditional full-file comparisons grouped by context window, with source versions counted once and result costs counted once per call. Gaps and clipped windows suppress the comparison; see the CLI token reference for scope and limits.

`accounting.work` (version 1) reports recorded MCP server operations by tool, with explicit returned/error/unknown outcomes, measured elapsed milliseconds, returned text and missing-measurement counts. Host observations and background activity are excluded from this subtotal. Text overlaps existing server-stage accounting; elapsed time is tool wall time, not CPU consumption or model time saved. A returned response is not proof of task quality. Session filters exclude unattributed calls, and absent older-server work data remains unavailable. See [CLI token accounting](./cli.md#token-experimental) for the report's measurement boundaries.

Measured-text MCP results also carry an `aide/work` metadata receipt (version, operation ID, tool and text SHA-256). When a host observer preserves that receipt with matching text and complete invocation identity, reports can attribute the server operation to its session. Missing or conflicting evidence stays unknown; text observations at host and server boundaries remain separate. The receipt does not change the model-facing text or establish successful task completion. `accounting.by_stage.aide_context` separately exposes prepared context source/appended-text measurements, not complete prompt usage or verified delivery.

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
