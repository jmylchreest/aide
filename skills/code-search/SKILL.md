---
name: code-search
description: Search code symbols, find function calls, and analyze codebase
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
  - show me the
---

# Code Search

**Recommended model tier:** balanced (sonnet) - this skill performs straightforward operations

Search for code symbols (functions, classes, methods, types) and find their call sites.

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

Find all places where a symbol is called/used.

**Example usage:**

```
Who calls "getUserById"?
→ Uses code_references tool
→ Returns: all call sites with file:line and context
```

### 3. List File Symbols (`mcp__plugin_aide_aide__code_symbols`)

List all symbols defined in a specific file.

**Example usage:**

```
What functions are in src/auth.ts?
→ Uses code_symbols tool
→ Returns: all functions, classes, types in that file
```

### 4. File Outline (`mcp__plugin_aide_aide__code_outline`)

Get a collapsed structural outline of a file — signatures preserved, bodies replaced with `{ ... }`.
Uses ~5-15% of the tokens of the full file. **Use this before reading a file** to understand its
structure, then use `Read` with offset/limit for specific sections.

**Example usage:**

```
Outline src/auth.ts
→ Uses code_outline tool
→ Returns: collapsed view with signatures, line ranges, bodies collapsed
```

### 5. Check Index Status (`mcp__plugin_aide_aide__code_stats`)

Check if the codebase has been indexed.

**Example usage:**

```
Is the code indexed?
→ Uses code_stats tool
→ Returns: file count, symbol count, reference count
```

## Workflow

1. **First, check if codebase is indexed:**
   - Use `code_stats` to verify indexing
   - If not indexed, tell user to run: `./.aide/bin/aide code index`

**Binary location:** The aide binary is at `.aide/bin/aide`. If it's on your `$PATH`, you can use `aide` directly.

2. **Search for symbols:**
   - Use `code_search` with the symbol name or pattern
   - Filter by kind (function, method, class, interface, type)
   - Filter by language (typescript, javascript, go, python)

3. **Find call sites:**
   - Use `code_references` to find where a symbol is used
   - Shows all callers with context

4. **Explore specific files:**
   - Use `code_symbols` to list all definitions in a file

## Example Session

**User:** "Where is the authentication function?"

**Assistant action:**

1. Use `code_search` with query "auth" or "authenticate"
2. Show matching functions with file locations

**User:** "Who calls authenticateUser?"

**Assistant action:**

1. Use `code_references` with symbol "authenticateUser"
2. Show all call sites grouped by file

## What These Tools Cover

`code_search` and `code_references` work from the tree-sitter symbol index. They find:

- Function, method, class, interface, type definitions by name
- Symbol signatures (parameter types, return types)
- Doc comments attached to definitions
- Call sites for a specific symbol name (via `code_references`)

For anything else — patterns inside function bodies, method call chains, string literals,
SQL queries, imports, variable declarations — use **Grep**, which searches code content directly.

In short: `code_search` finds _where things are defined_; Grep finds _patterns in code content_.

## Notes

- Code must be indexed first: `./.aide/bin/aide code index`
- Indexing is incremental - only changed files are re-parsed
- Supports: TypeScript, JavaScript, Go, Python, Rust, and more
- For file watching: `AIDE_CODE_WATCH=1 claude`
