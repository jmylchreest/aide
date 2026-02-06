# Codebase Indexing with Tree-sitter

## Overview

This document describes how to implement codebase indexing for AIDE using tree-sitter for parsing and a dedicated "code" memory store for symbol/structure data.

## Goals

1. **Understand project structure** - Directories, file types, entry points
2. **Index symbols** - Functions, classes, types, exports
3. **Track relationships** - Imports, inheritance, call graphs
4. **Enable semantic search** - "Find functions that handle authentication"

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     /aide:index command                      │
└─────────────────────────────┬───────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Indexer Pipeline                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ File Walker │─▶│ Tree-sitter │─▶│ Symbol Extractor    │  │
│  │ (glob)      │  │ Parser      │  │ (language-specific) │  │
│  └─────────────┘  └─────────────┘  └──────────┬──────────┘  │
│                                               │              │
│                                               ▼              │
│                              ┌─────────────────────────────┐ │
│                              │ Code Memory Store           │ │
│                              │ .aide/memory/code.db        │ │
│                              └─────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Data Model

### Symbol Types

```go
type Symbol struct {
    ID          string    `json:"id"`           // Unique ID
    Name        string    `json:"name"`         // Symbol name
    Kind        string    `json:"kind"`         // function, class, type, variable, etc.
    FilePath    string    `json:"file_path"`    // Relative path
    StartLine   int       `json:"start_line"`
    EndLine     int       `json:"end_line"`
    Signature   string    `json:"signature"`    // func(a: int) -> string
    DocComment  string    `json:"doc_comment"`  // JSDoc, docstring, etc.
    Exported    bool      `json:"exported"`     // Public API?
    ParentID    string    `json:"parent_id"`    // For nested symbols
    Language    string    `json:"language"`     // typescript, go, python, etc.
    UpdatedAt   time.Time `json:"updated_at"`
}

type Import struct {
    ID          string `json:"id"`
    FilePath    string `json:"file_path"`
    Source      string `json:"source"`      // Package/module imported
    Symbols     []string `json:"symbols"`   // Specific imports (or ["*"])
    IsRelative  bool   `json:"is_relative"` // ./local vs package
}

type FileInfo struct {
    Path        string    `json:"path"`
    Language    string    `json:"language"`
    Size        int64     `json:"size"`
    SymbolCount int       `json:"symbol_count"`
    UpdatedAt   time.Time `json:"updated_at"`
    Hash        string    `json:"hash"`       // For change detection
}
```

### Storage Schema

```
code.db (BBolt buckets):
├── files/          # FileInfo by path
├── symbols/        # Symbol by ID
├── symbols_by_file/  # path -> []symbol_id
├── symbols_by_name/  # name -> []symbol_id
├── imports/        # Import by ID
└── imports_by_file/  # path -> []import_id
```

### Search Index (Bleve)

Index fields for full-text search:
- `name` (boosted)
- `signature`
- `doc_comment`
- `file_path`
- `kind`

## Tree-sitter Integration

### Language Support

Use tree-sitter grammars for each language:

| Language | Grammar Package | Key Node Types |
|----------|-----------------|----------------|
| TypeScript | tree-sitter-typescript | function_declaration, class_declaration, interface_declaration, type_alias_declaration |
| JavaScript | tree-sitter-javascript | function_declaration, class_declaration, arrow_function |
| Go | tree-sitter-go | function_declaration, method_declaration, type_declaration |
| Python | tree-sitter-python | function_definition, class_definition |
| Rust | tree-sitter-rust | function_item, impl_item, struct_item |

### Extraction Queries

Example tree-sitter query for TypeScript functions:

```scheme
; Functions
(function_declaration
  name: (identifier) @name
  parameters: (formal_parameters) @params
  return_type: (type_annotation)? @return_type) @function

; Arrow functions assigned to const
(lexical_declaration
  (variable_declarator
    name: (identifier) @name
    value: (arrow_function
      parameters: (formal_parameters) @params))) @arrow_function

; Classes
(class_declaration
  name: (type_identifier) @name
  body: (class_body) @body) @class

; Interfaces
(interface_declaration
  name: (type_identifier) @name) @interface

; Type aliases
(type_alias_declaration
  name: (type_identifier) @name
  value: (_) @type) @type_alias

; Exports
(export_statement) @export
```

### Implementation Options

**Option A: Go with go-tree-sitter**

```go
import (
    sitter "github.com/smacker/go-tree-sitter"
    "github.com/smacker/go-tree-sitter/typescript"
)

func parseFile(path string, content []byte) ([]Symbol, error) {
    parser := sitter.NewParser()
    parser.SetLanguage(typescript.GetLanguage())

    tree, err := parser.ParseCtx(context.Background(), nil, content)
    if err != nil {
        return nil, err
    }

    return extractSymbols(tree.RootNode(), path)
}
```

**Option B: Node.js with tree-sitter npm packages**

```typescript
import Parser from 'tree-sitter';
import TypeScript from 'tree-sitter-typescript';

const parser = new Parser();
parser.setLanguage(TypeScript.typescript);

function parseFile(content: string): Symbol[] {
    const tree = parser.parse(content);
    return extractSymbols(tree.rootNode);
}
```

**Recommendation**: Go implementation in `aide` binary for:
- Single binary distribution
- Consistent with existing aide architecture
- Better performance for large codebases

## CLI Interface

```bash
# Index current project
aide code index

# Index specific paths
aide code index src/ lib/

# Search symbols
aide code search "authenticate"
aide code search --kind=function "handle*"
aide code search --file="auth.ts" "*"

# List symbols in file
aide code symbols src/auth.ts

# Show symbol details
aide code show "src/auth.ts:AuthService"

# Export as JSON (for context injection)
aide code export --format=json > symbols.json

# Clear index
aide code clear
```

## MCP Tools

```typescript
// New MCP tools for code memory

interface CodeSearchParams {
  query: string;
  kind?: 'function' | 'class' | 'type' | 'interface' | 'variable';
  file?: string;
  exported_only?: boolean;
  limit?: number;
}

interface CodeSymbolParams {
  file_path: string;
}

// Tools:
// - code_search: Search indexed symbols
// - code_symbols: List symbols in a file
// - code_index: Trigger re-indexing (skill only)
```

## Skill: /aide:index

```markdown
---
name: index
triggers:
  - /aide:index
  - index the codebase
  - analyze project structure
description: Index the codebase for symbol search
---

# Codebase Indexing

Run `aide code index` to parse and index the project.

## What Gets Indexed

- Functions, classes, interfaces, types
- Export relationships
- Import dependencies
- Documentation comments

## Usage

After indexing, use `code_search` MCP tool to find symbols:

- "Find all auth functions" → code_search(query="auth", kind="function")
- "Show UserService class" → code_search(query="UserService", kind="class")
```

## Context Injection

On session start, inject project structure summary:

```xml
<aide-project-structure>
## Project: aide

### Languages
- TypeScript (45 files, 3,420 symbols)
- Go (28 files, 890 symbols)

### Key Entry Points
- src/hooks/session-start.ts (SessionStart hook)
- aide/cmd/aide/main.go (CLI entry)

### Major Components
- src/hooks/ - Claude Code hooks (12 files)
- src/lib/ - Shared utilities (5 files)
- aide/pkg/store/ - Storage layer (8 files)

### Recently Modified
- src/hooks/session-start.ts (2 hours ago)
- aide/cmd/aide/cmd_memory.go (yesterday)

</aide-project-structure>
```

## Incremental Updates

Track file hashes to only re-index changed files:

```go
func shouldReindex(path string, info FileInfo) bool {
    currentHash := hashFile(path)
    return currentHash != info.Hash
}

func incrementalIndex(root string) {
    for _, file := range walkFiles(root) {
        info, exists := store.GetFileInfo(file.Path)
        if !exists || shouldReindex(file.Path, info) {
            symbols := parseFile(file.Path)
            store.UpdateFile(file.Path, symbols)
        }
    }
}
```

## File Watching (Future)

```go
// Watch for changes and auto-reindex
aide code watch

// Uses fsnotify to detect changes
// Debounces rapid changes
// Only re-indexes affected files
```

## Integration with Memory System

The code index is separate from the general memory store:

```
.aide/
├── memory/
│   └── store.db      # Memories, decisions, state, messages
└── code/
    ├── index.db      # Symbol storage (BBolt)
    └── search.bleve/ # Full-text search index
```

This separation allows:
- Independent indexing without affecting memories
- Different retention policies (code index can be rebuilt)
- Optimized storage for each use case

## Performance Considerations

1. **Parallel parsing** - Parse multiple files concurrently
2. **Lazy loading** - Don't load full AST into memory
3. **Incremental updates** - Only re-parse changed files
4. **Index size** - Estimate ~1KB per symbol, typical project <10MB
5. **Search speed** - Bleve provides <10ms search for most queries

## TODO

- [ ] Implement tree-sitter parsing in Go
- [ ] Add language detection (by extension + shebang)
- [ ] Create extraction queries for top 5 languages
- [ ] Build CLI commands (index, search, symbols)
- [ ] Add MCP tools (code_search, code_symbols)
- [ ] Implement incremental indexing
- [ ] Add project structure summary for context injection
- [ ] Consider LSP integration as alternative/complement
