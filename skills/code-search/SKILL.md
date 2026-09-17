---
name: code-search
description: Locate definitions, investigate callers and change impact, explore file structure, and read selected source
triggers:
  - find function
  - where is
  - who calls
  - find class
  - find method
  - search code
  - code search
  - find symbol
  - call sites
  - references to
  - what calls
  - change impact
  - read implementation
  - file structure
  - show me the
---

# Code Search

**Recommended model tier:** balanced (sonnet) - this skill performs straightforward operations

Locate definitions and caller candidates during debugging, review, and refactoring,
then inspect the current source needed for the task. Choose tools by the evidence
needed; there is no mandatory navigation step before reading a small file or a
known relevant range.

## Available Tools

### 1. Search Symbols (`mcp__plugin_aide_aide__code_search`)

Find functions, classes, methods, interfaces, and types by name or signature.

**Example usage:**

```
Search for: "getUserById"
→ Uses code_search tool
→ Returns: function signatures, file locations, line numbers
```

### 2. Find References (`mcp__plugin_aide_aide__code_references`)

Find indexed candidates where a symbol is called or used, including type references.
Use these to investigate callers and change impact, then verify current source.

**Example usage:**

```
Who calls "getUserById"?
→ Uses code_references tool
→ Returns: indexed candidate call sites with file:line and context
```

### 3. List File Symbols (`mcp__plugin_aide_aide__code_symbols`)

List parsed definitions in a specific file, with signatures and locations.
Use this for the file's API surface; coverage depends on grammar support.

**Example usage:**

```
What functions are in src/auth.ts?
→ Uses code_symbols tool
→ Returns: supported function, class, and type definitions in that file
```

### 4. File Outline (`mcp__plugin_aide_aide__code_outline`)

Get a collapsed structural outline of a file — signatures preserved, bodies replaced with `{ ... }`.
Use an outline to navigate an unfamiliar large file when its structure will help select what to read.
Read known line ranges directly; read the full file when it is small or most of its contents are needed.
When several symbol names are known, `code_read_symbol` supports a `symbols` batch (up to 10).
Smaller returned text does not guarantee fewer whole-task tokens: an extra navigation round has overhead.

**Example usage:**

```
Outline src/auth.ts
→ Uses code_outline tool
→ Returns: collapsed view with signatures, line ranges, bodies collapsed
```

### 5. Read Symbol Source (`mcp__plugin_aide_aide__code_read_symbol`)

Read current bodies for known function, method, class, or type names. Batch up to
10 names in `symbols` when several bodies are needed, rather than issuing a call
per name. Set `file` to choose an exact file, including one not yet indexed. If a
name occurs multiple times in that file, also set `start_line` to the current
definition line. Ambiguous names return candidates instead of choosing one.

```json
{ "symbols": ["authenticateUser", "validateToken"], "file": "src/auth.ts" }
```

Use a bounded Read for imports or surrounding context outside the selected symbols.
Read directly when the file is small or most of its contents are needed.

### 6. Check Index Status (`mcp__plugin_aide_aide__code_stats`)

Inspect index counts when diagnosing missing results or unavailable indexing.
This is not a required preflight before each query or source read, and counts do
not establish freshness or complete coverage.

**Example usage:**

```
Is the code indexed?
→ Uses code_stats tool
→ Returns: file count, symbol count, reference count
```

### 7. Search Findings (`mcp__plugin_aide_aide__findings_search`)

Search static analysis findings (complexity hotspots, secrets, code clones, coupling issues).

**Example usage:**

```
Any complexity issues in src/auth?
→ Uses findings_search tool with query "auth" or file filter
→ Returns: findings with file, line, severity, description
```

## Workflow

1. **Choose the needed evidence:** `code_search` for definition candidates;
   `code_references` for caller/type-use candidates; `code_symbols` or
   `code_outline` for structure in a known file. Filter by kind, language, or
   file when that narrows the question. Keep Grep for literals, imports, and
   patterns inside function bodies.
2. **Inspect current source:** follow returned `code_read_symbol` selectors for
   missing source, retaining their checkout selector. Do not repeat search or
   outline discovery for a location already identified. Selectors are indexed
   candidates; if a definition moved, retry its file/name without `start_line`.
   Reuse bodies already available in the current context; reread for changes or
   missing context. Batch known names with `code_read_symbol`, using
   `file` to disambiguate. Read known ranges or small/full-relevant files directly.
3. **Verify conclusions:** indexed search/reference results are best-effort
   candidates, not a complete semantic graph. Same-name symbols, dynamic calls,
   unsupported syntax, stale files, and limits can affect results. Verify relevant
   definitions and callers in current source; empty results do not prove absence.
4. **Investigate indexing only when needed:** if results suggest indexing is
   missing or stale, inspect `code_stats` or run `./.aide/bin/aide code index`.
   Continue with current-source reads or Grep when the index cannot answer.

**Binary location:** The aide binary is at `.aide/bin/aide`. If it's on your `$PATH`, you can use `aide` directly.

## Example Session

**User:** "Where is the authentication function?"

**Assistant action:**

1. Use `code_search` with query "auth" or "authenticate"
2. Show matching functions with file locations

**User:** "Who calls authenticateUser?"

**Assistant action:**

1. Use `code_references` with symbol "authenticateUser"
2. Inspect relevant candidate callers in current source and group the findings by file

## What These Tools Cover

`code_search` and `code_references` work from the tree-sitter symbol index. Depending
on grammar support and index state, their results include:

- Function, method, class, interface, type definitions by name
- Symbol signatures (parameter types, return types)
- Doc comments attached to definitions
- Call sites for a specific symbol name (via `code_references`)

For anything else — patterns inside function bodies, method call chains, string literals,
SQL queries, imports, variable declarations — use **Grep**, which searches code content directly.

## Notes

- Indexed searches need an index; exact-file `code_read_symbol` and on-demand file structure tools can inspect source without one
- Indexing is incremental - only changed files are re-parsed
- Supports: TypeScript, JavaScript, Go, Python, Rust, and more
- File watching is on by default; disable with `AIDE_CODE_WATCH=0` or `code.watch: false` in `.aide/config/aide.json`
