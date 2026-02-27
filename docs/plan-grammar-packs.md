# Grammar Packs Design

## Status: DESIGN (not yet implemented)

## Problem

Per-language knowledge is scattered across 7+ locations in the codebase:

| Category                                        | Location                              | Languages      |
| ----------------------------------------------- | ------------------------------------- | -------------- |
| File extensions / filenames / shebangs          | `code/types.go`                       | 28             |
| Tag queries (symbol extraction)                 | `code/parser.go` `TagQueries`         | 18             |
| Ref queries (call-site references)              | `code/parser.go` `RefQueries`         | 10             |
| Complexity configs (funcNodeTypes, branchTypes) | `findings/complexity_*.go` (16 files) | 17             |
| Clone tokenisation node-type maps               | `clone/tokenize.go` (3 maps)          | cross-language |
| Import extraction regexes                       | `findings/coupling.go`                | 5              |
| Grammar binaries (compiled)                     | `grammar/builtin.go`                  | 9              |
| Grammar definitions (downloadable)              | `grammar/dynamic.go`                  | 19             |

Adding a new language requires touching 4-7 files. The Go code contains hundreds of
lines of per-language data that could live in data files.

## Goal

Replace all per-language hardcoded data with **grammar packs** — self-contained,
versioned archives that bundle a compiled grammar binary with all the analysis metadata
aide needs for that language. The Go code becomes a single generic engine that reads
pack data.

## Pack Format

Each language produces one archive per platform:

```
aide-grammar-{name}-{version}-{os}-{arch}.tar.gz
  └── {name}/
      ├── grammar.so|.dylib|.dll    # Compiled tree-sitter grammar
      └── pack.json                 # All metadata for this language
```

Extracted to `.aide/grammars/{name}/`, this mirrors the embedded layout exactly.

### `pack.json` Schema

```json
{
  "name": "go",
  "version": "0.25.0",
  "c_symbol": "tree_sitter_go",

  "meta": {
    "extensions": [".go"],
    "filenames": [],
    "shebangs": [],
    "aliases": ["golang"]
  },

  "queries": {
    "tags": "(function_declaration name: (identifier) @name) @definition.function\n...",
    "refs": "(call_expression function: (identifier) @name) @reference.call\n..."
  },

  "complexity": {
    "func_node_types": [
      "function_declaration",
      "method_declaration",
      "func_literal"
    ],
    "branch_types": [
      "if_statement",
      "for_statement",
      "switch_case",
      "go_statement",
      "defer_statement",
      "expression_case",
      "type_case",
      "default_case",
      "communication_case"
    ],
    "name_field": "name"
  },

  "imports": {
    "patterns": [
      {
        "regex": "^\\s*import\\s+\"([^\"]+)\"",
        "group": 1,
        "context": "single"
      },
      { "regex": "^\\s*\"([^\"]+)\"", "group": 1, "context": "block" }
    ],
    "block_start": "import (",
    "block_end": ")"
  },

  "tokenisation": {
    "identifier_types": [
      "identifier",
      "type_identifier",
      "field_identifier",
      "package_identifier"
    ],
    "literal_types": [
      "interpreted_string_literal",
      "raw_string_literal",
      "int_literal",
      "float_literal"
    ],
    "keyword_types": [
      "func",
      "return",
      "var",
      "const",
      "type",
      "struct",
      "interface",
      "map",
      "chan",
      "go",
      "defer",
      "select",
      "range"
    ]
  }
}
```

Fields are optional — a minimal pack for a markup language like HTML might only have
`meta` and `queries.tags` (no complexity, no imports, no tokenisation).

## Architecture

### Two tiers: embedded + downloadable

```
                ┌─────────────────────────────────────────────┐
                │             aide binary (Go)                │
                │                                             │
                │  ┌─────────────────────────────────────┐    │
                │  │  Embedded packs (9 core languages)  │    │
                │  │  go:embed packs/*.json              │    │
                │  │  CGO-linked grammar binaries        │    │
                │  └─────────────────────────────────────┘    │
                │                                             │
                │  ┌─────────────────────────────────────┐    │
                │  │  Generic pack loader                │    │
                │  │  - Reads pack.json from any source  │    │
                │  │  - Provides: queries, complexity,   │    │
                │  │    imports, tokenisation configs     │    │
                │  │  - Falls back to generic defaults   │    │
                │  └─────────────────────────────────────┘    │
                └────────────────────┬────────────────────────┘
                                     │ auto-download
                ┌────────────────────▼────────────────────────┐
                │  .aide/grammars/{name}/                     │
                │    grammar.so                               │
                │    pack.json                                │
                │  (19+ dynamic languages)                    │
                └─────────────────────────────────────────────┘
```

### Embedded packs (9 core languages)

The 9 core languages (Go, TypeScript, JavaScript, Python, Rust, Java, C, C++, Zig)
get their **grammar binaries** compiled in via CGO (unchanged from today). Their
**pack.json metadata** is embedded via `go:embed`.

Directory layout in source (mirrors the runtime layout for downloaded packs):

```
aide/pkg/grammar/packs/
  go/
    pack.json
  typescript/
    pack.json
  javascript/
    pack.json
  python/
    pack.json
  rust/
    pack.json
  java/
    pack.json
  c/
    pack.json
  cpp/
    pack.json
  zig/
    pack.json
```

```go
//go:embed packs/*/pack.json
var embeddedPacks embed.FS
```

At startup, the loader walks `packs/*/pack.json` via the embedded FS and populates
the pack registry. This is the same `{name}/pack.json` path structure used by
downloaded packs in `.aide/grammars/{name}/pack.json`, so the loader reads from
both sources identically.
The grammar binary comes from the existing CGO linkage (`builtin.go`). Only the
metadata files are embedded — no grammar binaries in `go:embed` (they're
platform-specific and already handled by CGO).

### Downloaded packs (19+ dynamic languages)

Dynamic languages download their full pack archive (grammar binary + pack.json).
The archive extracts to `.aide/grammars/{name}/` — the same `{name}/pack.json`
layout as the embedded packs — so the loader reads from both sources identically.

The download URL and versioning use the existing mechanism:

- URL template: `https://github.com/jmylchreest/aide/releases/download/{version}/{asset}`
- Asset name: `aide-grammar-{name}-{version}-{os}-{arch}.tar.gz`
- Version: release tag for release builds, `snapshot` for dev builds

### Pack registry

```go
// PackRegistry holds loaded pack metadata for all known languages.
type PackRegistry struct {
    mu    sync.RWMutex
    packs map[string]*Pack  // keyed by language name
}

// Pack is the in-memory representation of a pack.json.
type Pack struct {
    Name    string        `json:"name"`
    Version string        `json:"version"`
    CSymbol string        `json:"c_symbol"`
    Meta    PackMeta      `json:"meta"`
    Queries PackQueries   `json:"queries"`
    Complexity *PackComplexity `json:"complexity,omitempty"`
    Imports    *PackImports    `json:"imports,omitempty"`
    Tokenisation *PackTokenisation `json:"tokenisation,omitempty"`
}

type PackMeta struct {
    Extensions []string `json:"extensions"`
    Filenames  []string `json:"filenames,omitempty"`
    Shebangs   []string `json:"shebangs,omitempty"`
    Aliases    []string `json:"aliases,omitempty"`
}

type PackQueries struct {
    Tags string `json:"tags,omitempty"`
    Refs string `json:"refs,omitempty"`
}

type PackComplexity struct {
    FuncNodeTypes []string `json:"func_node_types"`
    BranchTypes   []string `json:"branch_types"`
    NameField     string   `json:"name_field"`
}

type PackImports struct {
    Patterns   []ImportPattern `json:"patterns"`
    BlockStart string          `json:"block_start,omitempty"`
    BlockEnd   string          `json:"block_end,omitempty"`
}

type ImportPattern struct {
    Regex   string `json:"regex"`
    Group   int    `json:"group"`
    Context string `json:"context,omitempty"` // "single", "block", etc.
}

type PackTokenisation struct {
    IdentifierTypes []string `json:"identifier_types,omitempty"`
    LiteralTypes    []string `json:"literal_types,omitempty"`
    KeywordTypes    []string `json:"keyword_types,omitempty"`
}
```

### Loader changes

The `CompositeLoader` gains a `PackRegistry` and the loading flow becomes:

1. **Embedded packs**: On init, load all `packs/*.json` via `go:embed` into the registry.
2. **Dynamic packs**: When a language is requested and not in the registry, check
   `.aide/grammars/{name}/pack.json`. If present, load it.
3. **Auto-download**: If not present and `autoDownload=true`, download the archive,
   extract it, load both grammar binary and pack.json.
4. **Fallback**: If no pack.json exists (e.g., user manually placed a grammar .so),
   use generic defaults for complexity/tokenisation (as today).

### Consumer changes

Each consumer currently reading from hardcoded maps switches to the pack registry:

| Consumer                            | Currently reads           | Switches to                            |
| ----------------------------------- | ------------------------- | -------------------------------------- |
| `code/parser.go` `TagQueries`       | Hardcoded map             | `pack.Queries.Tags`                    |
| `code/parser.go` `RefQueries`       | Hardcoded map             | `pack.Queries.Refs`                    |
| `code/types.go` `LangExtensions`    | Hardcoded map             | `pack.Meta.Extensions`                 |
| `code/types.go` `LangFilenames`     | Hardcoded map             | `pack.Meta.Filenames`                  |
| `code/types.go` `ShebangLangs`      | Hardcoded map             | `pack.Meta.Shebangs`                   |
| `findings/complexity.go`            | `complexityLanguages` map | `pack.Complexity`                      |
| `findings/complexity_*.go`          | 16 per-language files     | Deleted (data in pack.json)            |
| `findings/coupling.go`              | Regex switch              | `pack.Imports.Patterns`                |
| `clone/tokenize.go`                 | 3 cross-language maps     | `pack.Tokenisation` + generic fallback |
| `grammar/scan.go` `NormaliseLang()` | Alias map                 | `pack.Meta.Aliases`                    |

### What gets deleted

Once packs are fully adopted:

- `findings/complexity_go.go` through `complexity_zig.go` (16 files)
- `complexityLanguages` map and `registerComplexityLang()` in `complexity.go`
- `genericComplexityLang` → becomes a Go constant/default, not per-language data
- `TagQueries` and `RefQueries` maps in `code/parser.go` (~170 lines)
- `LangExtensions`, `LangFilenames`, `ShebangLangs` maps in `code/types.go` (~100 lines)
- Import regex patterns and per-language extraction functions in `coupling.go` (~80 lines)
- `identifierTypes`, `literalTypes`, `keywordTypes` maps in `clone/tokenize.go` (~115 lines)
- `DynamicGrammars` map in `grammar/dynamic.go` (moves to pack.json `c_symbol` field)
- `NormaliseLang()` alias table in `grammar/scan.go`

## Build Pipeline

### CI changes (release.yml)

The `build-grammars` job changes from producing bare `.so` files to producing
`.tar.gz` archives containing `grammar.so` + `pack.json`:

```bash
# For each language and platform:
mkdir -p "pack-${name}/${name}"
# ... compile grammar.so as today ...
cp "grammar${EXT}" "pack-${name}/${name}/grammar${EXT}"
cp "aide/pkg/grammar/packs/${name}/pack.json" "pack-${name}/${name}/pack.json"
tar -czf "aide-grammar-${name}-${VERSION}-${OS}-${ARCH}.tar.gz" -C "pack-${name}" "${name}"
```

The pack.json source files live at `aide/pkg/grammar/packs/{name}/pack.json` in the
repository — the same directory structure used by `go:embed` for core languages.
CI copies from the same source regardless of whether a language is core or dynamic.

### Local dev builds

The `aide-dev-toggle.sh` script and `make build` workflow remain unchanged for the
Go binary. For grammars:

- **Core 9**: Metadata is `go:embed`'d — always available in dev builds, no extra steps.
- **Dynamic 19**: Downloaded at runtime from the `snapshot` release, as today. The
  only change is the archive format (`.tar.gz` containing grammar + pack.json instead
  of a bare `.so`).

To test pack changes locally without pushing to GitHub:

1. Edit `aide/pkg/grammar/packs/{name}/pack.json`
2. Run `make build` (embeds the updated JSON for core languages)
3. For dynamic languages: manually place your test pack.json at `.aide/grammars/{name}/pack.json`

### Adding a new language

With packs, adding a new language is:

1. Create `packs/{name}.json` with all metadata
2. Add the grammar build to CI's `build-grammars` matrix
3. Done — no Go code changes required

If the language needs a compiled-in grammar (promotion to core), additionally:

1. Add the Go module dependency to `go.mod`
2. Add the `Register()` call in `builtin.go`
3. Add the JSON file to the `packs/` embed directory

## Tokenisation Strategy

Clone detection tokenisation currently uses three cross-language maps. With packs, each
language can declare its own token type classifications. However, many languages share
common node type names (e.g., `identifier`, `string`, `number`).

Strategy:

1. **Pack-specific tokenisation**: If `pack.Tokenisation` is present, use those lists
   for the language.
2. **Generic fallback**: If absent, use a built-in generic map (the current cross-language
   maps, effectively). This handles unknown languages and languages where nobody has
   specified tokenisation rules.
3. **Merge**: The generic fallback types are always included as a base; pack-specific
   types extend (not replace) them.

## Import Extraction Strategy

The current regex-based import extraction (`coupling.go`) is fundamentally different
from the tree-sitter query approach — it works on raw text, not parse trees. Packs
encode these regexes in `imports.patterns`.

The generic engine in Go:

```go
func extractImportsFromPack(content []byte, pack *Pack) []string {
    if pack.Imports == nil {
        return nil
    }
    var imports []string
    // For each line, try each pattern
    scanner := bufio.NewScanner(bytes.NewReader(content))
    inBlock := false
    for scanner.Scan() {
        line := scanner.Text()
        if pack.Imports.BlockStart != "" && strings.Contains(line, pack.Imports.BlockStart) {
            inBlock = true
            continue
        }
        if inBlock && pack.Imports.BlockEnd != "" && strings.TrimSpace(line) == pack.Imports.BlockEnd {
            inBlock = false
            continue
        }
        for _, p := range pack.Imports.Patterns {
            if p.Context == "block" && !inBlock {
                continue
            }
            if p.Context == "single" && inBlock {
                continue
            }
            re := regexp.MustCompile(p.Regex)
            if m := re.FindStringSubmatch(line); m != nil && len(m) > p.Group {
                imports = append(imports, m[p.Group])
            }
        }
    }
    return imports
}
```

Future improvement: Replace regex import extraction with tree-sitter queries
(`imports.scm`) for languages where we have a grammar loaded. But regex is fine as
a first step and works without parsing.

## Migration Plan

### Phase 1: Create pack.json files (no behaviour change)

1. Create `aide/pkg/grammar/packs/{name}/pack.json` for all 28 languages
2. Populate from existing hardcoded data (mechanical extraction)
3. Add `go:embed` for the pack JSON files
4. Create `PackRegistry` and `Pack` types
5. Load embedded packs at startup
6. **No consumers changed yet** — existing hardcoded maps remain

### Phase 2: Switch consumers to pack registry

One consumer at a time, with tests at each step:

1. `code/types.go` — `DetectLanguage()` reads from pack registry instead of hardcoded maps
2. `code/parser.go` — `TagQueries`/`RefQueries` read from pack registry
3. `findings/complexity.go` — reads `pack.Complexity` instead of `complexityLanguages` map
4. `findings/coupling.go` — reads `pack.Imports` instead of per-language regex functions
5. `clone/tokenize.go` — reads `pack.Tokenisation` with generic fallback

### Phase 3: Update CI to produce pack archives

1. Change `build-grammars` job to produce `.tar.gz` archives
2. Update `DynamicLoader` to extract archives instead of expecting bare `.so` files
3. Load `pack.json` from extracted archive directory
4. Update manifest to track pack version alongside grammar version

### Phase 4: Clean up

1. Delete `complexity_*.go` (16 files)
2. Delete hardcoded `TagQueries`, `RefQueries`, `LangExtensions`, etc.
3. Delete per-language import extraction functions
4. Delete `identifierTypes`/`literalTypes`/`keywordTypes` cross-language maps
5. Update documentation

## Open Questions

1. **Schema versioning**: Should pack.json have a `schema_version` field so the loader
   can handle future format changes gracefully?

2. **Pack validation**: Should `aide grammar scan` validate pack.json files (e.g., check
   that declared node types exist in the grammar's `node-types.json`)?

3. **User-contributed packs**: Should users be able to drop a custom pack.json into
   `.aide/grammars/{name}/` to add or override language support? (Probably yes — this
   falls out naturally from the architecture.)

4. **Language constants**: `code/types.go` currently defines `LangGo`, `LangPython`, etc.
   as string constants used throughout the codebase. With packs, these could be derived
   from pack names. But some code paths (e.g., coupling's block detection) may still
   need language-specific branching. Decision: keep constants for now, derive from packs
   later.

5. **Tokenisation merge vs. replace**: Should pack tokenisation types extend the generic
   fallback or replace it entirely? Extending is safer (catches common types) but may
   include irrelevant types. Replacing is cleaner but requires each pack to be exhaustive.
