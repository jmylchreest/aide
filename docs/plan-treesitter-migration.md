# Plan: Tree-Sitter Grammar Migration

## Status: Proposed

## Summary

Migrate from `smacker/go-tree-sitter` (community, monorepo with all grammars
compiled in) to `tree-sitter/go-tree-sitter` (official) with a hybrid grammar
loading strategy: core languages compiled in, all others downloaded on demand
via `purego` dynamic library loading.

## Motivation

- **Binary size**: Current aide binary is ~74MB because 27 language grammars
  are compiled in via CGO. Most users only need a handful.
- **Freshness**: `smacker/go-tree-sitter` is a community fork pinned to an
  Aug 2024 pseudoversion. The official `tree-sitter/go-tree-sitter` tracks
  tree-sitter core v0.25.x with the latest features (ABI versioning,
  language metadata, progress callbacks, lookahead iterators).
- **Extensibility**: Adding a new language currently requires a code change +
  rebuild + release. Dynamic loading makes it a download.
- **Cross-compilation**: Fewer compiled-in C grammars means faster builds and
  smaller artefacts.

## Current Usage

Three features use tree-sitter:

| Feature                | File                             | Languages | API Used                  |
| ---------------------- | -------------------------------- | --------- | ------------------------- |
| Code symbol extraction | `pkg/code/parser.go`             | 27        | Parse, Query, QueryCursor |
| Complexity analysis    | `pkg/findings/complexity.go`     | 6         | Parse, AST walk           |
| Clone tokenisation     | `pkg/findings/clone/tokenize.go` | 6         | Parse, leaf-node DFS      |

All 27 languages are in the code parser. Complexity and clone detection only
use: Go, TypeScript, JavaScript, Python, Rust, Java.

## Architecture

### Hybrid Grammar Loading

```
aide binary (~15-20MB)
  ├── tree-sitter core runtime (~150KB C, CGO)
  ├── 10 compiled-in grammars (Go, TS, JS, Python, Rust, Java, Zig, C, C++)
  │   (always available, no download needed)
  │
  └── Dynamic grammar loader (purego)
        └── .aide/grammars/
              ├── manifest.json          (installed grammars, versions, checksums)
              ├── libtree-sitter-ruby-v0.23.1-linux-amd64.so
              ├── libtree-sitter-php-v0.23.1-linux-amd64.so
              └── ...
```

### Grammar Distribution

Pre-built `.so`/`.dylib`/`.dll` files published as GitHub release assets on
the `jmylchreest/aide` repository, tagged to match the aide version.

Release asset naming convention:

```
aide-grammar-{lang}-{grammar-version}-{os}-{arch}.{ext}
```

Examples:

```
aide-grammar-ruby-v0.23.1-linux-amd64.so
aide-grammar-ruby-v0.23.1-darwin-arm64.dylib
aide-grammar-ruby-v0.23.1-windows-amd64.dll
```

The CI/CD pipeline builds all supported grammar shared libraries for each
release. This ensures:

- Grammars are built with the same tree-sitter ABI version as the binary
- Checksums are published for integrity verification
- Users download from the same GitHub release they got aide from

### Grammar Manifest

`.aide/grammars/manifest.json`:

```json
{
  "aide_version": "0.41.0",
  "abi_version": 15,
  "grammars": {
    "ruby": {
      "version": "0.23.1",
      "file": "libtree-sitter-ruby-v0.23.1-linux-amd64.so",
      "sha256": "abc123...",
      "installed_at": "2026-02-26T21:00:00Z"
    }
  }
}
```

### Version Detection & Compatibility

The official `go-tree-sitter` provides:

- `Language.AbiVersion()` — the tree-sitter ABI version the grammar was
  compiled against
- `Language.Metadata()` — semantic version (`major.minor.patch`) from the
  grammar's `tree-sitter.json`
- `LANGUAGE_VERSION` / `MIN_COMPATIBLE_LANGUAGE_VERSION` constants — the
  range of ABI versions the runtime supports

On load, aide verifies:

```
MIN_COMPATIBLE_LANGUAGE_VERSION <= grammar.AbiVersion() <= LANGUAGE_VERSION
```

If a grammar's ABI version is outside the compatible range (e.g., after an
aide upgrade bumps the tree-sitter core), the grammar is marked stale and
re-downloaded.

## CLI Interface

```
aide grammar list              # Show installed + available grammars
aide grammar install <lang>    # Download grammar for <lang>
aide grammar install all       # Download all available grammars
aide grammar update            # Update all installed grammars
aide grammar remove <lang>     # Remove a grammar
aide grammar clean             # Remove all downloaded grammars
```

### Automatic Grammar Download

When aide encounters a file it can't parse (extension mapped to a language
but grammar not installed), it:

1. Checks if the language is a compiled-in core grammar → use it
2. Checks `.aide/grammars/` for a downloaded grammar → load it
3. If neither exists, downloads the grammar from the matching GitHub release
4. On download failure (offline, etc.), silently skips the file

### Language Detection

A language scanner runs at project initialisation (or on `aide grammar detect`):

1. Walks the project tree (respecting `.aideignore`)
2. Maps file extensions to languages
3. Reports which languages are detected but missing grammars
4. Optionally auto-downloads missing grammars (configurable in `aide.json`)

`aide.json` configuration:

```json
{
  "grammars": {
    "auto_download": true,
    "languages": ["ruby", "php", "kotlin"]
  }
}
```

The `languages` field pins specific grammars; if omitted, auto-download
fetches whatever the scanner detects.

## Compiled-In Core Grammars

These are always available (no download needed):

| Language   | Used By                    | Rationale                                 |
| ---------- | -------------------------- | ----------------------------------------- |
| Go         | parser, complexity, clones | Primary development language              |
| TypeScript | parser, complexity, clones | Primary development language              |
| JavaScript | parser, complexity, clones | Ubiquitous                                |
| Python     | parser, complexity, clones | Ubiquitous                                |
| Rust       | parser, complexity, clones | Growing ecosystem                         |
| Java       | parser, complexity, clones | Enterprise staple                         |
| Zig        | parser                     | Growing ecosystem, systems language       |
| C          | parser                     | Systems language, tree-sitter itself is C |
| C++        | parser                     | Systems language                          |

These 9 languages are compiled in via the standard CGO bindings from the
official `tree-sitter-<lang>` repos. Estimated binary size: ~20-25MB
(vs 74MB currently with 27 languages).

## Dynamic Grammars (Downloaded on Demand)

All other languages supported by tree-sitter. Initially:

| Language | Package                                   |
| -------- | ----------------------------------------- |
| C#       | tree-sitter/tree-sitter-c-sharp           |
| Kotlin   | tree-sitter/tree-sitter-kotlin            |
| Scala    | tree-sitter-grammars/tree-sitter-scala    |
| Groovy   | tree-sitter-grammars/tree-sitter-groovy   |
| Ruby     | tree-sitter/tree-sitter-ruby              |
| PHP      | tree-sitter/tree-sitter-php               |
| Lua      | tree-sitter-grammars/tree-sitter-lua      |
| Elixir   | tree-sitter-grammars/tree-sitter-elixir   |
| Bash     | tree-sitter/tree-sitter-bash              |
| Swift    | tree-sitter-grammars/tree-sitter-swift    |
| OCaml    | tree-sitter/tree-sitter-ocaml             |
| Elm      | tree-sitter-grammars/tree-sitter-elm      |
| SQL      | tree-sitter-grammars/tree-sitter-sql      |
| YAML     | tree-sitter-grammars/tree-sitter-yaml     |
| TOML     | tree-sitter-grammars/tree-sitter-toml     |
| HCL      | tree-sitter-grammars/tree-sitter-hcl      |
| Protobuf | tree-sitter-grammars/tree-sitter-protobuf |
| HTML     | tree-sitter/tree-sitter-html              |
| CSS      | tree-sitter/tree-sitter-css               |

New languages can be added without any code change — just add grammar source
to the build pipeline and the `.so` appears in the next release.

## CI/CD Pipeline

### Grammar Build Matrix

Add a GitHub Actions job to the release workflow:

```yaml
grammar-build:
  strategy:
    matrix:
      lang: [ruby, php, csharp, kotlin, scala, ...]
      os: [ubuntu-latest, macos-latest, windows-latest]
      arch: [amd64, arm64]
  steps:
    - checkout tree-sitter-<lang> at pinned version
    - cc -shared -o libtree-sitter-<lang>.so -fPIC src/parser.c [src/scanner.c]
    - upload as release asset
```

Grammar versions are pinned in a `grammars.lock` file in the repo:

```
ruby=v0.23.1
php=v0.23.3
kotlin=v0.3.8
...
```

### Upgrade Flow

When aide is upgraded:

1. New binary has updated `LANGUAGE_VERSION` / `MIN_COMPATIBLE_LANGUAGE_VERSION`
2. On first run, checks each installed grammar's ABI version
3. If any are outside the compatible range, fetches updated grammars from the
   new release
4. Updates `manifest.json`

## API Migration

### Key API Changes (smacker → official)

| smacker                          | official                                  | Notes                                                   |
| -------------------------------- | ----------------------------------------- | ------------------------------------------------------- |
| `sitter.NewParser()`             | `tree_sitter.NewParser()`                 | Same                                                    |
| `parser.SetLanguage(lang)`       | `parser.SetLanguage(lang)`                | Same                                                    |
| `parser.ParseCtx(ctx, nil, src)` | `parser.ParseWithOptions(src, nil, opts)` | Progress callback replaces ctx                          |
| `sitter.NewQuery(pattern, lang)` | `tree_sitter.NewQuery(lang, pattern)`     | Arg order swapped                                       |
| `sitter.NewQueryCursor()`        | `tree_sitter.NewQueryCursor()`            | Same                                                    |
| `cursor.Exec(query, node)`       | `cursor.Matches(query, node, src)`        | Returns iterator                                        |
| `cursor.NextMatch()`             | Iterator `.Next()`                        | Different iteration pattern                             |
| `node.Content(src)`              | `node.Utf8Text(src)`                      | Renamed                                                 |
| `node.Child(i)`                  | `node.Child(i)`                           | Same                                                    |
| `node.ChildByFieldName(name)`    | `node.ChildByFieldName(name)`             | Same                                                    |
| GC-based cleanup                 | Explicit `.Close()` required              | Must defer .Close() on Parser, Tree, Query, QueryCursor |

### Grammar Loading Abstraction

Introduce a `grammar.Loader` interface that abstracts over compiled-in and
dynamic grammars:

```go
type Loader interface {
    // Load returns the Language for the given name, downloading if needed.
    Load(ctx context.Context, name string) (*tree_sitter.Language, error)

    // Available returns all grammar names that can be loaded.
    Available() []string

    // Installed returns grammars currently available locally.
    Installed() []GrammarInfo
}

type GrammarInfo struct {
    Name       string
    Version    string
    AbiVersion uint32
    BuiltIn    bool
    Path       string // empty for built-in
}
```

## Implementation Phases

### Phase 1: Core Migration (no behaviour change)

- Replace `smacker/go-tree-sitter` with `tree-sitter/go-tree-sitter`
- Keep all 27 languages compiled in (same as today)
- Update API calls (query arg order, iteration, explicit Close())
- Update tag/ref query strings if needed
- Verify all tests pass

### Phase 2: Grammar Loader Abstraction

- Implement `grammar.Loader` with compiled-in backend only
- Refactor `parser.go`, `complexity.go`, `tokenize.go` to use Loader
- Reduce compiled-in grammars to 9 core languages
- Remaining 18 languages return "not available" (graceful skip)

### Phase 3: Dynamic Loading

- Add purego-based dynamic loader to `grammar.Loader`
- Implement `.aide/grammars/` storage with manifest
- Implement download from GitHub releases
- Add `aide grammar` CLI subcommand
- Add ABI version checking on load

### Phase 4: Auto-Detection & Pipeline

- Language scanner for automatic grammar detection
- CI/CD grammar build matrix in release workflow
- `grammars.lock` file for version pinning
- Auto-update on aide upgrade
- `aide.json` grammar configuration

## Open Questions

1. **Zig grammar**: The official `tree-sitter-zig` grammar is at
   `tree-sitter-grammars/tree-sitter-zig`. Verify Go bindings exist or
   we'd need to create the CGO wrapper.

2. **purego + CGO coexistence**: The tree-sitter core runtime requires CGO.
   purego is used only for grammar loading. Confirm this mixed approach works
   (it should — purego just calls dlopen, orthogonal to CGO).

3. **Windows DLL loading**: purego's `Dlopen` is not available on Windows.
   Need to use `x/sys/windows.LoadLibrary` instead. Abstract behind the
   Loader interface.

4. **Grammar build reproducibility**: Should we vendor grammar sources in a
   submodule or fetch at build time? Submodule gives reproducibility;
   fetch-at-build is simpler.

5. **Extending complexity/clone analysers**: Currently only 6 languages.
   With dynamic loading, should these analysers also support dynamically
   loaded grammars? (Needs language-specific config like `branchTypes` for
   complexity.)
