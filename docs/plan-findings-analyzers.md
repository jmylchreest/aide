# Findings & Analyzers Plan

Status: **Approved** — implementation in progress.

## Overview

Add a **Findings Store** and a suite of **static analyzers** to AIDE so that LLM agents
can access precomputed code-health signals (complexity, coupling, secrets, clones) without
burning tokens on manual inspection.

Findings are stored in a separate BoltDB + Bleve database at `.aide/findings/` following
the same pattern as the existing CodeStore. Analyzers write to the store via CLI commands;
MCP tools expose read-only access to LLM agents.

## Architecture

```
┌─────────────────────────────────────────────┐
│              LLM Agent (Claude)              │
│  uses MCP tools: findings_search,           │
│  findings_list, findings_stats              │
└──────────────┬──────────────────────────────┘
               │ stdio (JSON-RPC)
┌──────────────▼──────────────────────────────┐
│         MCPServer (cmd_mcp.go)              │
│  registerFindingsTools() — read-only        │
├─────────────────────────────────────────────┤
│         gRPC Server (server.go)             │
│  FindingsServiceServer — full CRUD          │
├─────────────────────────────────────────────┤
│         FindingsStore (store/findings.go)   │
│  BoltDB: findings.db + Bleve: search.bleve  │
│  Location: .aide/memory/findings/           │
└──────────────┬──────────────────────────────┘
               │ write path (CLI only)
┌──────────────▼──────────────────────────────┐
│         aide findings run <analyzer>        │
│                                             │
│  Analyzers:                                 │
│  - complexity  (cyclomatic complexity)      │
│  - coupling    (import graph + cycles)      │
│  - secrets     (Titus scanner)              │
│  - clones      (Rabin-Karp rolling hash)    │
└─────────────────────────────────────────────┘
```

## Components

### 1. Finding Types (`aide/pkg/findings/types.go`)

```go
type Finding struct {
    ID        string    // ULID
    Analyzer  string    // "complexity", "coupling", "secrets", "clones"
    Severity  string    // "critical", "warning", "info"
    Category  string    // Sub-category within analyzer
    FilePath  string    // Relative file path
    Line      int       // Start line (1-indexed)
    EndLine   int       // End line (0 = single line)
    Title     string    // Short description
    Detail    string    // Extended explanation
    Metadata  map[string]string // Analyzer-specific data
    CreatedAt time.Time
}
```

Severity constants: `SevCritical`, `SevWarning`, `SevInfo`.

### 2. Findings Store (`aide/pkg/store/findings.go`)

Follows CodeStore pattern:

- BoltDB at `.aide/memory/findings/findings.db`
- Bleve index at `.aide/memory/findings/search.bleve`
- Buckets: `findings`, `findings_meta`
- Interface: `FindingsStore` added to `interfaces.go`

### 3. Analyzers

#### Cyclomatic Complexity

- Added directly to `parser.go` during symbol extraction
- `Complexity int` field added to `Symbol` struct
- Per-language decision-node maps (if, for, while, case, &&, ||, etc.)
- Findings generated for functions exceeding threshold (default: 10)

#### Coupling Analysis

- Import extraction via tree-sitter queries in `parser.go`
- Graph construction + DFS cycle detection in `aide/pkg/findings/coupling.go`
- Findings: circular dependencies (critical), high fan-in/fan-out (warning)

#### Secret Detection

- Uses `github.com/praetorian-inc/titus` (Apache-2.0)
- Scans file content for 459 secret patterns
- Findings: each match = critical finding

#### Clone Detection

- Tree-sitter leaf-node tokenization with identifier normalization
- Rabin-Karp rolling hash (k=40 tokens)
- Hash collision → verify → extend → merge adjacent
- ~500 LOC in `aide/pkg/findings/clone/`
- Findings: each clone pair = warning

### 4. MCP Tools (read-only)

| Tool              | Description                                        |
| ----------------- | -------------------------------------------------- |
| `findings_search` | Full-text search findings by query                 |
| `findings_list`   | List findings filtered by analyzer, severity, file |
| `findings_stats`  | Aggregate counts by analyzer and severity          |

### 5. CLI Commands (write path)

```
aide findings run <analyzer> [paths...]   # Run analyzer(s)
aide findings run all                     # Run all analyzers
aide findings search <query>              # Search findings
aide findings list [--analyzer=X] [--severity=X] [--file=X]
aide findings stats                       # Show summary
aide findings clear [--analyzer=X]        # Clear findings
```

### 6. Skill Updates

- **review**: Add findings tools to MCP Tools section; add "Check findings" step
- **code-search**: Add findings_search to Available Tools
- **git**: Add Change Context section with findings awareness
- **patterns** (new): Skill for detecting architectural patterns using code_search + code_symbols

### 7. Bug Fix: Doc Comment Extraction

`extractWithQuery()` in `parser.go` doesn't call `extractPrecedingComment()`.
This silently drops doc comments for all 17 languages using the query path.
Fix: add `sym.DocComment = p.extractPrecedingComment(defNode, content)` after
symbol creation.

### 8. Skill Linter

Build-time TypeScript/Bun script at `scripts/validate-skills.ts`:

- Validates YAML frontmatter (required fields: name, description, triggers)
- Checks markdown structure (headings, MCP tool references)
- Validates MCP tool name format
- Wired into CI via package.json script

## Implementation Order

1. Plan documents (this file + preflight-exec concept)
2. Doc comment bug fix (2 lines)
3. Findings types + store interface + BoltDB/Bleve implementation
4. Wire store into MCPServer + gRPC
5. MCP tools (read-only) + CLI commands (write path)
6. Complexity analyzer (parser.go changes + Symbol struct)
7. Coupling analyzer (import queries + graph)
8. Secret detection (Titus)
9. Clone detection
10. Skill linter
11. Skill updates + new patterns skill
12. Build + test

## Decisions

- **Titus** for secrets (not gitleaks) — Apache-2.0, pure-Go, 459 rules
- **Separate DB** at `.aide/findings/` (not shared with memories)
- **MCP tools are read-only** — analyzers run via CLI only
- **No recent-changes analyzer** — LLM + git is sufficient
- **Pattern detection is a skill** — uses code_search/code_symbols, not automated
- **Preflight-exec is document-only** — not implemented in this phase
