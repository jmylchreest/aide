# Plan: Tree-Sitter Grammar Migration

## Status: In Progress

## Summary

Migrate from `smacker/go-tree-sitter` (community, monorepo with all 27
grammars compiled in) to `tree-sitter/go-tree-sitter` (official) with a
hybrid grammar loading strategy: 9 core languages compiled in via CGO,
all others downloaded on demand and loaded via `purego` dynamic library
loading.

This is a single-pass migration — the core API rewrite, grammar loader
abstraction, purego dynamic loading, and CI build pipeline are all
implemented together. No intermediate "all 27 compiled in" step.

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
aide binary (~20-25MB)
  ├── tree-sitter core runtime (~150KB C, CGO)
  ├── 9 compiled-in grammars (Go, TS, JS, Python, Rust, Java, Zig, C, C++)
  │   (always available, no download needed)
  │
  └── Dynamic grammar loader (purego)
        └── .aide/grammars/
              ├── manifest.json          (installed grammars, versions, checksums)
              ├── libtree-sitter-ruby-v0.23.1-linux-amd64.so
              ├── libtree-sitter-php-v0.23.1-linux-amd64.so
              └── ...
```

### Grammar Loader Interface

```go
// pkg/grammar/loader.go

type Loader interface {
    // Load returns the Language for the given name.
    // For compiled-in grammars, returns immediately.
    // For dynamic grammars, checks local cache then downloads from GitHub.
    Load(ctx context.Context, name string) (*tree_sitter.Language, error)

    // Available returns all grammar names that can be loaded (compiled-in + downloadable).
    Available() []string

    // Installed returns grammars currently available locally (compiled-in + cached).
    Installed() []GrammarInfo

    // Install downloads a grammar to the local cache.
    Install(ctx context.Context, name string) error

    // Remove deletes a grammar from the local cache.
    Remove(name string) error
}

type GrammarInfo struct {
    Name       string
    Version    string
    AbiVersion uint32
    BuiltIn    bool     // true for 9 core grammars
    Path       string   // empty for built-in
}
```

Loading priority:

1. Check compiled-in grammars → return immediately
2. Check `.aide/grammars/` for a cached `.so`/`.dylib`/`.dll` → load via purego
3. Download from GitHub release assets → cache locally → load via purego
4. On download failure (offline, etc.) → return error, caller skips silently

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
- `Language.Metadata()` — semantic version from `tree-sitter.json`
- `LANGUAGE_VERSION` / `MIN_COMPATIBLE_LANGUAGE_VERSION` constants — the
  range of ABI versions the runtime supports

On load, aide verifies:

```
MIN_COMPATIBLE_LANGUAGE_VERSION <= grammar.AbiVersion() <= LANGUAGE_VERSION
```

If a grammar's ABI version is outside the compatible range (e.g., after an
aide upgrade), it is marked stale and re-downloaded.

## Compiled-In Core Grammars (9)

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

Import paths for compiled-in grammars:

| Language   | Go Import Path                                                | Function               |
| ---------- | ------------------------------------------------------------- | ---------------------- |
| Go         | `github.com/tree-sitter/tree-sitter-go/bindings/go`           | `Language()`           |
| TypeScript | `github.com/tree-sitter/tree-sitter-typescript/bindings/go`   | `LanguageTypescript()` |
| JavaScript | `github.com/tree-sitter/tree-sitter-javascript/bindings/go`   | `Language()`           |
| Python     | `github.com/tree-sitter/tree-sitter-python/bindings/go`       | `Language()`           |
| Rust       | `github.com/tree-sitter/tree-sitter-rust/bindings/go`         | `Language()`           |
| Java       | `github.com/tree-sitter/tree-sitter-java/bindings/go`         | `Language()`           |
| Zig        | `github.com/tree-sitter-grammars/tree-sitter-zig/bindings/go` | `Language()`           |
| C          | `github.com/tree-sitter/tree-sitter-c/bindings/go`            | `Language()`           |
| C++        | `github.com/tree-sitter/tree-sitter-cpp/bindings/go`          | `Language()`           |

## Dynamic Grammars (Downloaded on Demand)

| Language | Source Repository                       | C Symbol Name         |
| -------- | --------------------------------------- | --------------------- |
| C#       | tree-sitter/tree-sitter-c-sharp         | `tree_sitter_c_sharp` |
| Kotlin   | tree-sitter-grammars/tree-sitter-kotlin | `tree_sitter_kotlin`  |
| Scala    | tree-sitter/tree-sitter-scala           | `tree_sitter_scala`   |
| Groovy   | amaanq/tree-sitter-groovy               | `tree_sitter_groovy`  |
| Ruby     | tree-sitter/tree-sitter-ruby            | `tree_sitter_ruby`    |
| PHP      | tree-sitter/tree-sitter-php             | `tree_sitter_php`     |
| Lua      | tree-sitter-grammars/tree-sitter-lua    | `tree_sitter_lua`     |
| Elixir   | tree-sitter/tree-sitter-elixir          | `tree_sitter_elixir`  |
| Bash     | tree-sitter/tree-sitter-bash            | `tree_sitter_bash`    |
| Swift    | alex-pinkus/tree-sitter-swift           | `tree_sitter_swift`   |
| OCaml    | tree-sitter/tree-sitter-ocaml           | `tree_sitter_ocaml`   |
| Elm      | elm-tooling/tree-sitter-elm             | `tree_sitter_elm`     |
| SQL      | DerekStride/tree-sitter-sql             | `tree_sitter_sql`     |
| YAML     | tree-sitter-grammars/tree-sitter-yaml   | `tree_sitter_yaml`    |
| TOML     | tree-sitter-grammars/tree-sitter-toml   | `tree_sitter_toml`    |
| HCL      | tree-sitter-grammars/tree-sitter-hcl    | `tree_sitter_hcl`     |
| Protobuf | coder3101/tree-sitter-proto             | `tree_sitter_proto`   |
| HTML     | tree-sitter/tree-sitter-html            | `tree_sitter_html`    |
| CSS      | tree-sitter/tree-sitter-css             | `tree_sitter_css`     |

For dynamic loading, the shared library exports `tree_sitter_{lang}()` which
returns a `const TSLanguage *`. The purego loader calls this via `Dlopen` +
`Dlsym`.

## API Migration

### Key API Changes (smacker → official)

| smacker                          | official                              | Notes                                                   |
| -------------------------------- | ------------------------------------- | ------------------------------------------------------- |
| `sitter.NewParser()`             | `tree_sitter.NewParser()`             | Same                                                    |
| `parser.SetLanguage(lang)`       | `parser.SetLanguage(lang) error`      | Returns error now                                       |
| `parser.ParseCtx(ctx, nil, src)` | `parser.Parse(src, nil)`              | No context; old tree is second arg                      |
| `sitter.NewQuery(pattern, lang)` | `tree_sitter.NewQuery(lang, pattern)` | Arg order swapped; pattern is string not []byte         |
| `sitter.NewQueryCursor()`        | `tree_sitter.NewQueryCursor()`        | Same                                                    |
| `cursor.Exec(query, node)`       | `cursor.Matches(query, node, src)`    | Returns iterator; source required                       |
| `cursor.NextMatch()`             | `matches.Next()`                      | Iterator pattern: returns `*QueryMatch` or nil          |
| `node.Type()`                    | `node.Kind()`                         | Renamed                                                 |
| `node.Content(src)`              | `node.Utf8Text(src)`                  | Renamed                                                 |
| `node.StartPoint().Row`          | `node.StartPosition().Row`            | Renamed                                                 |
| `node.EndPoint().Row`            | `node.EndPosition().Row`              | Renamed                                                 |
| `node.Child(int)`                | `node.Child(uint)`                    | Index type changed                                      |
| `node.ChildCount() uint32`       | `node.ChildCount() uint`              | Return type changed                                     |
| `query.CaptureNameForId(i)`      | `query.CaptureNames()[i]`             | Array access instead of method                          |
| `query.CaptureCount()`           | `len(query.CaptureNames())`           | Derived from slice                                      |
| GC-based cleanup                 | Explicit `.Close()` required          | Must defer .Close() on Parser, Tree, Query, QueryCursor |

### Grammar Provider Changes

smacker (current):

```go
import "github.com/smacker/go-tree-sitter/golang"
lang := golang.GetLanguage()  // returns *sitter.Language
```

official compiled-in:

```go
import (
    tree_sitter "github.com/tree-sitter/go-tree-sitter"
    tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)
lang := tree_sitter.NewLanguage(tree_sitter_go.Language())  // unsafe.Pointer → *Language
```

official dynamic (purego):

```go
lib, _ := purego.Dlopen("libtree-sitter-ruby.so", purego.RTLD_LAZY)
var treeSitterRuby func() uintptr
purego.RegisterLibFunc(&treeSitterRuby, lib, "tree_sitter_ruby")
lang := tree_sitter.NewLanguage(unsafe.Pointer(treeSitterRuby()))
```

## CLI Interface

```
aide grammar list              # Show installed + available grammars
aide grammar install <lang>    # Download grammar for <lang>
aide grammar install --all     # Download all available grammars
aide grammar update            # Update all installed grammars
aide grammar remove <lang>     # Remove a grammar
aide grammar clean             # Remove all downloaded grammars
aide grammar detect            # Scan project and report missing grammars
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

## CI/CD Pipeline

### Grammar Build Workflow

A dedicated GitHub Actions workflow (or job in release.yml) that builds
grammar shared libraries for all dynamic languages:

```yaml
grammar-build:
  strategy:
    matrix:
      lang:
        [
          ruby,
          php,
          csharp,
          kotlin,
          scala,
          groovy,
          lua,
          elixir,
          bash,
          swift,
          ocaml,
          elm,
          sql,
          yaml,
          toml,
          hcl,
          protobuf,
          html,
          css,
        ]
      include:
        - os: ubuntu-latest
          goos: linux
          ext: so
        - os: macos-latest
          goos: darwin
          ext: dylib
        - os: ubuntu-latest # cross-compile for windows
          goos: windows
          ext: dll
      arch: [amd64, arm64]
  steps:
    - checkout tree-sitter-<lang> at pinned version from grammars.lock
    - cc -shared -o aide-grammar-<lang>-<version>-<os>-<arch>.<ext> \
      -fPIC src/parser.c [src/scanner.c]
    - upload as release asset
```

### grammars.lock

Pins grammar versions for reproducibility:

```
ruby=v0.23.1@tree-sitter/tree-sitter-ruby
php=v0.23.3@tree-sitter/tree-sitter-php
kotlin=v0.3.8@tree-sitter-grammars/tree-sitter-kotlin
...
```

### Upgrade Flow

When aide is upgraded:

1. New binary has updated `LANGUAGE_VERSION` / `MIN_COMPATIBLE_LANGUAGE_VERSION`
2. On first run, checks each installed grammar's ABI version
3. If any are outside the compatible range, fetches updated grammars from the
   new release
4. Updates `manifest.json`

## Implementation Steps

This is a single-pass implementation. Order of work:

### Step 1: Grammar Loader Package

Create `pkg/grammar/` with:

- `loader.go` — `Loader` interface and composite loader
- `builtin.go` — compiled-in grammar registry (9 languages)
- `dynamic.go` — purego-based loader from `.aide/grammars/`
- `download.go` — GitHub release asset downloader
- `manifest.go` — manifest.json read/write
- `platform.go` — OS/arch detection, library file naming

### Step 2: Core API Migration

Rewrite all 3 consumer files to use the official `go-tree-sitter` API:

- `pkg/code/parser.go` — use `grammar.Loader`, update all API calls
- `pkg/findings/complexity.go` — use `grammar.Loader`, update all API calls
- `pkg/findings/clone/tokenize.go` — use `grammar.Loader`, update all API calls

### Step 3: 9 Core Grammars

Add the 9 core grammar dependencies to `go.mod` and wire them into
`pkg/grammar/builtin.go`.

### Step 4: go.mod Cleanup

Remove `smacker/go-tree-sitter` from `go.mod`. Run `go mod tidy`.

### Step 5: Tests & Verification

- All existing tests pass
- Build compiles for all target platforms
- Binary size is in expected range (~20-25MB)

### Step 6: CLI & CI

- `aide grammar` subcommand
- Grammar build workflow in CI
- `grammars.lock` file

### Step 7: Auto-Detection

- Language scanner
- Auto-download on first use
- `aide.json` grammar configuration

## Open Questions

1. **Windows DLL loading**: purego's `Dlopen` is not available on Windows.
   Need to use `x/sys/windows.LoadLibrary` instead. Abstract behind the
   Loader interface with build tags.

2. **Grammar build reproducibility**: Should we vendor grammar sources in a
   submodule or fetch at build time? Submodule gives reproducibility;
   fetch-at-build is simpler. Recommend fetch-at-build with version pinning
   via `grammars.lock`.

3. **Extending complexity/clone analysers**: Currently only 6 languages.
   With dynamic loading, should these analysers also support dynamically
   loaded grammars? (Needs language-specific config like `branchTypes` for
   complexity — this is a code change, not just a grammar.)
