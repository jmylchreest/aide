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

5 code-related MCP tools are available to the AI:

| Tool              | Purpose                                                       |
| ----------------- | ------------------------------------------------------------- |
| `code_search`     | Search indexed symbol definitions (functions, classes, types) |
| `code_symbols`    | List all symbols defined in a specific file                   |
| `code_references` | Find all call sites and usages of a symbol                    |
| `code_stats`      | Get index statistics (files, symbols, references)             |
| `code_outline`    | Get collapsed file outline with signatures and line numbers   |

## File Watcher

When the MCP server is running, a file watcher automatically re-indexes changed files. Controlled by:

- `--code-watch` flag on `aide mcp`
- `AIDE_CODE_WATCH=1` environment variable
- `AIDE_CODE_WATCH_DELAY=30s` debounce delay (default 30s)

The watcher also triggers findings analysers on changed files.

## File Exclusions

Create a `.aideignore` file in your project root to exclude files from indexing and analysis. Uses gitignore syntax. Built-in defaults already exclude common generated files, lock files, build artifacts, and directories like `node_modules/`, `.git/`, `vendor/`, etc.

## Skill

Use `/aide:code-search` to search code symbols and find call sites interactively.
