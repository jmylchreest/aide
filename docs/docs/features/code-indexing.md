---
sidebar_position: 3
---

# Code Indexing

Fast symbol search using [tree-sitter](https://tree-sitter.github.io/). Supports TypeScript, JavaScript, Go, Python, Rust, and many more languages.

## Usage

```bash
aide code index              # Index codebase (incremental)
aide code index --force      # Re-index every file regardless of mtime
aide code search "getUser"   # Search symbols
aide code symbols src/auth.ts  # List file symbols
aide code references getUser   # Find call sites
aide code stats              # Index statistics
aide code clear              # Clear index
```

`aide code index` streams per-file progress (path + symbol count) to stderr and prints a final summary on completion. Progress works whether the daemon is running or not; on large repos the run can take minutes, and the live updates double as a heartbeat that keeps the gRPC stream alive.

## Parallel parsing

Tree-sitter parsing is the dominant cost on large repositories, so the indexer fans parsing out across worker goroutines while keeping the bbolt write transaction and Bleve batch on a single writer goroutine (both are exclusive by design). Defaults to one worker per CPU core, capped at 32.

Override with `AIDE_INDEX_WORKERS=N`:

- unset or `0` → `runtime.NumCPU()` (recommended default)
- positive `N` → that count, clamped at 32
- `1` → effectively single-threaded (useful for benchmarking or debugging)

Progress events arrive in completion order rather than walk order — small files finish first; large files trickle. That tracks real progress more accurately and is the documented contract for the streaming RPC.

## MCP Tools

8 code-related MCP tools are available to the AI:

| Tool                  | Purpose                                                       |
| --------------------- | ------------------------------------------------------------- |
| `code_search`         | Search indexed symbol definitions (functions, classes, types) |
| `code_symbols`        | List all symbols defined in a specific file                   |
| `code_references`     | Find all call sites and usages of a symbol                    |
| `code_stats`          | Get index statistics (files, symbols, references)             |
| `code_outline`        | Get collapsed file outline with signatures and line numbers   |
| `code_top_references` | Rank symbols by reference count across the codebase           |
| `code_read_check`     | Check if a file is indexed, unchanged, and estimate its token cost |
| `token_stats`         | Get estimated token usage and savings statistics              |

## File Watcher

When the MCP server is running, a file watcher automatically re-indexes changed files. On by default; controlled by:

- `code.watch` in `.aide/config/aide.json` or `AIDE_CODE_WATCH=0` to disable
- `--code-watch` flag on `aide mcp`
- `AIDE_CODE_WATCH_DELAY=30s` debounce delay (default 30s)

The watcher also triggers findings analysers on changed files.

## Smart Read Hints

With the file watcher enabled, aide can recognize matching full-file text observed
in a known host/session/actor/context window. When the source still matches and its
index is fresh, a soft hint suggests reusing that text if it remains available, or
retrieving specific missing sections. This is advice, not a recorded avoided read
or a guarantee of token savings. Unknown context windows do not establish reuse.

Valid bounded reads bypass both the large-file advisory and smart-read lookups,
including ranges of 100 lines or more. Unbounded large source reads can receive
conditional outline advice; reading directly remains appropriate when most of the
file is needed. Hints never block the read. The Claude Code hook emits advisory
context; the current OpenCode adapter only debug-logs the smart-read result.

The smart-read hint labels the index's per-language token estimate as estimated
text tokens. It is not measured provider usage.

## Token Estimation (Experimental)

:::note
Token estimates are **experimental**, not exact tokenizer or billing counts.
Recorded text bytes and runtime-reported usage have separate measurement boundaries;
neither should be presented as an inferred saving.
:::

Index metadata retains per-language token estimates, used by read-check and
smart-read hints. These legacy estimates are separate from the central
`utf8-bytes/3-v1` estimator used for measured-text accounting and conditional
retrieval comparisons in the CLI and aide-web. Neither estimate is a live
tokenizer call. Provider/runtime counters, when available, must remain separately
identified rather than combined with estimated text totals.

Recorded tool results and hint injections can be viewed with `aide token stats`
or the aide-web Tokens page. A suggested outline or an absence of a later read does
not itself create a measured saved-token event. See [retrieval experiments](../reference/retrieval-experiments.md)
for the evidence boundaries and independent task comparisons.

## File Exclusions

Create a `.aideignore` file in your project root to exclude files from indexing and analysis. Uses gitignore syntax. Built-in defaults already exclude common generated files, lock files, build artifacts, and directories like `node_modules/`, `.git/`, `vendor/`, etc.

## Skill

Use `/aide:code-search` to search code symbols and find call sites interactively.
