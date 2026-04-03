---
sidebar_position: 3
---

# Code Indexing

Fast symbol search using [tree-sitter](https://tree-sitter.github.io/). Supports TypeScript, JavaScript, Go, Python, Rust, and many more languages.

## Usage

```bash
aide code index              # Index codebase (incremental)
aide code search "getUser"   # Search symbols
aide code symbols src/auth.ts  # List file symbols
aide code references getUser   # Find call sites
aide code stats              # Index statistics
aide code clear              # Clear index
```

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

When the MCP server is running, a file watcher automatically re-indexes changed files. Controlled by:

- `--code-watch` flag on `aide mcp`
- `AIDE_CODE_WATCH=1` environment variable
- `AIDE_CODE_WATCH_DELAY=30s` debounce delay (default 30s)

The watcher also triggers findings analysers on changed files.

## Smart Read Hints

When the file watcher is enabled, aide tracks which files the AI has read during a session. If the AI attempts to re-read a file that hasn't changed, a soft hint suggests using `code_outline`, `code_symbols`, or `code_references` instead. This avoids redundant full-file reads and preserves context window tokens.

The hint includes an estimated token count for the file, based on calibrated per-language character ratios.

## Token Estimation

Each indexed file stores an estimated token count alongside its symbols. Estimates are calibrated against the Anthropic `count_tokens` API with per-language ratios (e.g., Go ~2.8 chars/token, TypeScript ~3.2, Markdown ~3.7). These estimates are used by the smart read hints and the Token Intelligence dashboard in aide-web.

Token events (reads, outline substitutions, avoided re-reads) are recorded in the store and can be viewed with `aide token stats` or the aide-web Tokens page.

## File Exclusions

Create a `.aideignore` file in your project root to exclude files from indexing and analysis. Uses gitignore syntax. Built-in defaults already exclude common generated files, lock files, build artifacts, and directories like `node_modules/`, `.git/`, `vendor/`, etc.

## Skill

Use `/aide:code-search` to search code symbols and find call sites interactively.
