---
sidebar_label: CLI Reference
sidebar_position: 4
title: CLI Reference
---

# CLI Reference

Full command reference for the `aide` binary.

## Memory

```bash
aide memory add --category=learning --tags=testing "Prefers vitest"
aide memory search "authentication"
aide memory list --category=learning
aide memory delete <id>
aide memory reindex                      # Rebuild search index
aide memory export --format=markdown     # Export to markdown
```

| Command          | Description                               |
| ---------------- | ----------------------------------------- |
| `memory add`     | Store a new memory with category and tags |
| `memory search`  | Full-text search across memories          |
| `memory list`    | List memories, optionally by category     |
| `memory delete`  | Delete a memory by ID                     |
| `memory reindex` | Rebuild the Bleve search index            |
| `memory export`  | Export memories to markdown               |

## Decisions

```bash
aide decision set auth-strategy "JWT with refresh tokens" --rationale="Stateless"
aide decision get auth-strategy
aide decision list
aide decision list --origin=all
aide decision history auth-strategy
aide decision delete auth-strategy
```

| Command            | Description                                        |
| ------------------ | -------------------------------------------------- |
| `decision set`     | Record a decision for a topic (appends to history) |
| `decision get`     | Get the current (latest) decision                  |
| `decision list`    | List this store's decision topics (`--origin` to widen) |
| `decision history` | Show full history for a topic                      |
| `decision delete`  | Delete a decision topic                            |
| `decision adopt`   | Promote a subscribed peer's decision into this store (see Sync & Subscriptions) |

`decision list` shows this store's own decisions by default. Pass `--origin`
to also list rules that are in force here but stored elsewhere in the estate:

| Value    | Shows additionally                            |
| -------- | --------------------------------------------- |
| `parent` | Ancestors on the anchor chain                 |
| `peer`   | Subscribed peers                              |
| `all`    | Both                                          |

These are additive and never shadow local decisions — a topic decided nearer
always wins (local > parent > peer), matching what `session init` injects. An
`ORIGIN` column appears when a non-local row is present. Inherited entries are
read-only from here; use `decision adopt` to copy one into this store.

## Tasks

```bash
aide task create "Implement user model" --description="Create User struct"
aide task claim <id> --agent=executor-1
aide task complete <id> --result="Done"
aide task list --status=pending
aide task delete <id>
```

| Command         | Description                           |
| --------------- | ------------------------------------- |
| `task create`   | Create a new task (starts as pending) |
| `task claim`    | Atomically claim a task for an agent  |
| `task complete` | Mark a task as done with a result     |
| `task list`     | List tasks, optionally by status      |
| `task delete`   | Delete a task                         |

## Messages

```bash
aide message send "User model ready" --from=executor-1
aide message send "Can you review?" --from=executor-2 --to=executor-1
aide message list --agent=executor-1
aide message ack <id> --agent=executor-1
```

| Command        | Description                            |
| -------------- | -------------------------------------- |
| `message send` | Send a message (broadcast or directed) |
| `message list` | List messages for an agent             |
| `message ack`  | Acknowledge a message as read          |

## State

```bash
aide state set mode autopilot
aide state set mode eco --agent=worker-1
aide state get mode --agent=worker-1
aide state list
aide state clear --agent=worker-1
```

| Command       | Description                             |
| ------------- | --------------------------------------- |
| `state set`   | Set a state value (global or per-agent) |
| `state get`   | Get a state value                       |
| `state list`  | List all state entries                  |
| `state clear` | Clear state for an agent                |

## Code

```bash
aide code index                          # Index codebase (incremental)
aide code search "getUser"               # Search symbols
aide code symbols src/auth.ts            # List file symbols
aide code references getUserById         # Find call sites
aide code read-check src/auth.ts --json  # Check if file is indexed and fresh
aide code stats                          # Index statistics
aide code clear                          # Clear index
```

| Command           | Description                                        |
| ----------------- | -------------------------------------------------- |
| `code index`      | Index the codebase using tree-sitter (incremental) |
| `code search`     | Search symbol definitions                          |
| `code symbols`    | List all symbols in a specific file                |
| `code references` | Find all call sites of a symbol                    |
| `code read-check` | Check if a file is indexed and unchanged           |
| `code stats`      | Show index statistics                              |
| `code clear`      | Clear the code index                               |

## Findings

```bash
aide findings run                         # Run all analysers
aide findings run --analyser=complexity   # Run specific analyser
aide findings search "high complexity"    # Search findings
aide findings list --severity=critical    # List by severity
aide findings list --file=src/auth        # List by file
aide findings stats                       # Health overview
aide findings accept <id1> <id2>          # Accept specific findings
aide findings accept --analyzer=clones    # Accept all clone findings
aide findings accept --all                # Accept all findings
aide findings clear                       # Clear all findings
```

| Command           | Description                                  |
| ----------------- | -------------------------------------------- |
| `findings run`    | Run analysers (all or specific)              |
| `findings search` | Full-text search across findings             |
| `findings list`   | List findings by severity, file, or analyser |
| `findings stats`  | Codebase health overview                     |
| `findings accept` | Accept (dismiss) findings by ID or filter    |
| `findings clear`  | Clear all findings                           |

:::note
Both `--analyser=` and `--analyzer=` spellings are accepted on all findings commands.
:::

## Survey

```bash
aide survey run                          # Run all 3 analyzers
aide survey run --analyzer=topology      # Run specific analyzer
aide survey search "auth"               # Search survey entries
aide survey list --kind=module           # List by entry kind
aide survey list --kind=tech_stack       # Detected technologies
aide survey list --kind=entrypoint       # Entry points
aide survey list --kind=churn            # High-change files
aide survey stats                        # Overview by analyzer and kind
aide survey graph getUserById            # Call graph (callers + callees)
aide survey graph --symbol=main \
    --direction=callers --max-depth=3    # Callers only, deeper traversal
aide survey clear                        # Clear all survey data
aide survey clear --analyzer=churn       # Clear specific analyzer
```

| Command         | Description                                          |
| --------------- | ---------------------------------------------------- |
| `survey run`    | Run analyzers (topology, entrypoints, churn, or all) |
| `survey search` | Full-text search across survey entries               |
| `survey list`   | List entries by analyzer, kind, or file              |
| `survey stats`  | Aggregate counts by analyzer and kind                |
| `survey graph`  | Build call graph for a symbol (callers/callees/both) |
| `survey clear`  | Clear survey data (all or by analyzer)               |

## Grammar

```bash
aide grammar list                        # List all grammars (built-in + available + installed)
aide grammar list --installed            # Only installed grammars
aide grammar install ruby                # Install a specific grammar
aide grammar install --all               # Install all available grammars
aide grammar install                     # Install from lock file
aide grammar remove ruby                 # Remove a downloaded grammar
aide grammar remove --all                # Remove all downloaded grammars
aide grammar scan                        # Detect languages in current project
aide grammar scan --json                 # JSON output
```

| Command           | Description                                    |
| ----------------- | ---------------------------------------------- |
| `grammar list`    | List grammars (built-in, available, installed) |
| `grammar install` | Download and install dynamic grammars          |
| `grammar remove`  | Remove downloaded grammars                     |
| `grammar scan`    | Scan project for languages used                |

## Share

```bash
aide share export                        # Export decisions + memories to .aide/shared/
aide share export --decisions            # Decisions only
aide share import                        # Import from .aide/shared/
aide share import --dry-run              # Preview import
```

| Command        | Description                                      |
| -------------- | ------------------------------------------------ |
| `share export` | Export decisions and memories to `.aide/shared/` |
| `share import` | Import from `.aide/shared/`                      |

## Sync & Subscriptions

```bash
aide sync                                # Fetch all subscribed peer context
aide sync platform-team                  # Fetch one subscription
aide sync --timeout=2m                   # Per-subscription deadline (default 60s)
aide decision adopt api-style --from=platform-team  # Promote a peer decision locally
```

A failing subscription is reported and skipped, the rest still sync, and
the command exits non-zero if any failed — CI can use it as a red light.

Subscriptions name peer context sources in `.aide/config/aide.json`:

```json
{ "subscriptions": [
    { "name": "platform-team", "url": "git@host:platform/context.git", "branch": "main" },
    { "name": "proto-repo",    "path": "../protos" }
] }
```

`aide sync` fetches git subscriptions into `.aide/cache/remotes/<name>/`
(local `path` subscriptions are read in place). Peer records form a
**read-only layer**: their decisions appear in session context labeled
`from peer <name>`, at the lowest precedence (local > ancestors > peers),
and are **never re-exported** — you only publish records you authored or
explicitly adopted. Only decisions cross project boundaries; memories and
state never do. `aide decision adopt TOPIC [--from=PEER]` is the promotion
verb: it copies the peer's current decision into the local store as a new
local decision stamped with adoption provenance. Adopt reads only the
local cache (never the network), and `--from` is required when more than
one peer publishes the topic. Session init refreshes any subscription
cache older than 1 hour, under a single 5-second deadline shared across
subscriptions, silently serving the stale cache when offline; session-end
publishing gets 10 seconds.

A subscription with `"publish": true` is two-way: `aide sync` also writes
this project's own decisions into it — fetch, reset to the remote head,
apply records, commit, push, retrying on a push race. Write-once record
files named by identity make concurrent publishers safe: colliding paths
are structurally impossible, so no merge machinery is needed. Publishing
respects the `share.decisions.export_filter` policy and never includes
memories. An empty repository works as a starting point — the first
publish bootstraps it.

No external scheduler is needed in either direction: the session
lifecycle is the clock. Session **start** refreshes stale subscription
caches (reads); session **end** publishes publish-enabled subscriptions
(writes) — decisions are only ever made inside sessions, so the session
ending is the publish event. Both are bounded and offline-silent; an
unreachable remote never blocks startup or teardown, and unpublished
records simply ship at the next session end. `aide sync` remains the
manual lever for forcing either side immediately.

## Global Flags

```bash
aide --store parent decision set api-style "REST"   # write into the nearest containing project
aide --store top decision set go-version "1.26"     # write into the estate root
aide --project-root /path/to/proj memory list       # run against any store
```

`--store` re-targets the whole invocation onto another member of this
project's anchor chain: `parent` (nearest container), `top` (outermost
ancestor), or an explicit chain-member path. Hard errors, never silent
fallback: the estate root has no parent, non-chain paths are rejected
(unrelated stores are `--project-root`'s job), and the target must already
have a `.aide` store — a write never bootstraps another project's store.

## Decision Cascade

Sessions in a project with ancestors (see `aide anchor`) inherit ancestor
decisions into their injected context, nearest-wins: a topic decided
locally shadows every ancestor version. Inherited entries are labeled
`inherited from parent <name>` with an override hint. Reads go through the
ancestor's daemon socket when live, else a short read-only open — never a
writable open of another project's store. CLI/MCP `decision get` remains
store-local (placement is physical); the cascade is a context-injection
feature. Subscribed peers layer in below ancestors (see Sync &
Subscriptions). Disable both with `AIDE_CASCADE_DISABLED=1`.

## Anchor

```bash
aide anchor                              # Resolved root, provenance, parent scopes
aide anchor --json                       # Full machine-readable payload
aide anchor --cwd=/path/to/dir           # Probe from another directory
```

A read-only resolution probe: prints the project root aide would use, which
marker decided it (`.git` directory/worktree/submodule, `.aide`, env
override), the project identity, and the anchor chain — the project itself
plus VCS-evidenced parent scopes (a submodule's superproject, ancestor
repositories that contain it). It never creates `.aide/` and exits 0 even
when no marker is found, so it is safe to run anywhere — the "what would
happen?" command for worktree, submodule, and nested-repo layouts.

The `--json` payload is the contract consumed by the hook layer (persisted
per session under `~/.aide/anchors/` and `.aide/state/anchor.json`).

## Token (Experimental)

```bash
aide token stats                         # Observed text and paired output changes
aide token stats --details               # Context windows and historical breakdowns
aide token stats --json                  # JSON output
aide token summary                       # Recent token events
aide token summary --limit=20            # Last 20 events
aide token summary --details             # Retrieval status and evidence limits
aide token stats --session=ID --since=24h # Selected session and period
aide token cleanup                       # Remove events older than 90 days (cleanup.token_max_age)
aide token cleanup --max-age=168h        # Custom retention
```

| Command         | Description                                   |
| --------------- | --------------------------------------------- |
| `token stats`   | Show text accounting and evidence coverage    |
| `token summary` | List recent token events                      |
| `token cleanup` | Remove old token events (default 90 days)     |

Token tracking is experimental. New observations record supported UTF-8 text bytes at a host-hook or MCP-server boundary, with token estimates computed centrally using `utf8-bytes/3-v1`. Opaque media and provider framing are excluded. Generated argument text covers edit/write content fields, not all tool arguments. Host and server observations can overlap and must not be summed; neither alone confirms final delivery to the model.

`stats --json` adds a versioned `accounting` object with `by_stage`, `arguments`, `legacy_events`, `missing_payload`, `missing_identity`, and `estimator`. Missing payload is unknown, distinct from a known empty result. Coverage counts describe recorded observations, not the unknown number of unseen calls. An absent accounting object means the server does not support this report.

`accounting.transformations` reports paired before/after UTF-8 text, with central token estimates and signed reductions (negative means added text). `rewrite_candidate` is a Claude-compatible replacement proposal; `adapter_change` measures the output changed by aide's OpenCode adapter, including added annotations. Keep these stages separate. Neither proves host acceptance, final model delivery or provider savings. The report includes at most 64 recently recorded context-window/stage groups; `windows_limited` signals omitted groups while stage totals still cover every selected pair. Missing/pending context identity stays unwindowed. Malformed pairs are counted separately. Transform observations are not counted as additional tool calls or result consumption.

Existing JSON fields remain available. `total_saved`, `saved_by_tool` and `by_saving_type` are legacy comparison estimates, not verified savings or inferred avoided calls. `total_read` includes result estimates from shell/search/network tools; `total_written` covers generated argument text where measured, and historical modification estimates. Historical methods remain mixed. Provider usage and avoided-call inference are not included in this report. Conditional retrieval comparisons are separate from the historical savings fields.

Both `stats` and `summary` accept `--session`, `--since` (RFC3339 timestamp or duration such as `24h`) and `--until` (inclusive RFC3339 timestamp). `summary --last=N` remains a deprecated alias for `--limit=N`: its historical implementation limits events, not sessions.

Retrieval evidence appears in `token summary --details`, event JSON attributes, and expandable web Details rows. Recognised aide outline/symbol calls are also observed at the host hook boundary. A surviving `aide/retrieval` MCP metadata receipt binds source hashes to matching returned text; missing or mismatched metadata stays unverified. Native reads and simple `cat`/`sed -n` shell reads are checked against regular project files up to 8 MiB. A full-file match proves matching text was observed; a range match only verifies those lines against a current snapshot, not the undisplayed version. Recognised `rg`/`grep` searches are search observations, not file delivery. Unsupported shell syntax, command continuations without their original command, missing payloads, failures and pending executions do not establish coverage. These events support conditional retrieval comparisons, not inferred avoided calls.

`accounting.retrievals` groups selected host retrieval observations by host, session, actor and context epoch. Each file/SHA-256 baseline counts once per window, and each whole result (including batch formatting and errors) counts once. Matching full-file results contribute their costs; source ranges cannot mint an unproven file-version baseline. Comparisons require a verified aide source receipt and no detected evidence gaps. Unclassified shell text is reported separately and suppresses the comparison. Missing payloads, unresolved targets, changed-but-unverified ranges, identity gaps, malformed evidence and date filters that clip retained window history also suppress it. Estimated-token differences use the same central per-event/per-file estimator as other accounting. Negative differences preserve overhead.

The default CLI reports window counts; `token stats --details` explains conditional comparisons, and JSON/web Details expose the sequence and source identities. Reports retain at most 64 windows, 256 source versions per window and 64 displayed steps; step limits do not truncate measured window totals. Open windows can change as later calls arrive. A later observed reset or unknown-continuity epoch is a boundary marker, not proof of complete collection. Retention and unsupported host/shell paths still limit coverage. An observation that no full-file result appears in selected records is not a measured avoided call. Cache expiry alone does not split windows, and comparisons are never multiplied by subsequent turns.

In aide-web, **Telemetry → Tokens** opens on an Overview with headline estimates and a compact recorded-activity chart. Details contains tool/source breakdowns and event evidence; Accounting contains collection diagnostics, measurement methods and historical estimates. Date/session filters apply across all three views. Headline totals and the activity chart are aggregated centrally over every selected event, independently of the recent-events page size. Chart gaps mean no measured data; activity is not a savings or context-budget burndown. Host and server results are selectable, never added together. Historical estimates remain separately labelled.

Accounting also shows small before/after bars for each transformation boundary. Details holds the context-window table and per-event recovery paths. The Overview remains compact. Each paired result counts once, without multiplying by subsequent turns or cache reuse.

Before shortening output, aide retains the original under `.aide/artifacts/tool-output/` and includes its path in the replacement. Failed retention leaves the output intact. Error shortening preserves opening context and final diagnostic lines. Claude replacements support documented Bash objects and pure-text MCP results; unsupported native or mixed-media results remain intact. Missing session/invocation/window evidence prevents Claude pruning. Recovery artifacts are separate from token-event retention and are not removed by `token cleanup`.

## Status

```bash
aide status                              # Full dashboard
aide status --json                       # JSON output
```

Shows version, server status, file watcher, code index, findings analysers, MCP tools, stores, and environment variables.

The server line (and `serverState` in `--json`) has three states:

| State                   | Meaning                                                                                                                                                                                                                    |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `running`               | Connected to the daemon over `.aide/aide.sock`                                                                                                                                                                              |
| `not-running`           | No daemon socket; store sections are read directly from disk                                                                                                                                                                |
| `unreachable-sandboxed` | A daemon socket exists but this shell's sandbox denies `connect()` (e.g. Codex sandboxed execs). The daemon is likely running but unverifiable, so direct store reads are skipped — they would stall on the daemon's locks |

## Other Commands

```bash
aide session init                        # Initialize session
aide session end --session=ID            # End session (teardown + metrics)
aide upgrade                             # Self-upgrade binary
aide mcp                                 # Start MCP server (and gRPC primary)
aide version                             # Show version
```

| Command        | Description                                                                                                       |
| -------------- | ----------------------------------------------------------------------------------------------------------------- |
| `session init` | Initialize a new session                                                                                          |
| `session end`  | End a session: broadcast the end message, clear transient state, record metrics (`--session=ID [--duration=MS]`) |
| `upgrade`      | Self-upgrade the aide binary                                                                                      |
| `daemon`       | Start the gRPC daemon                                                                                             |
| `mcp`          | Start the MCP server (stdio)                                                                                      |
| `version`      | Show the installed version                                                                                        |
