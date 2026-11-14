---
sidebar_position: 6
---

# Grammar Management

AIDE uses [tree-sitter](https://tree-sitter.github.io/) grammars for code indexing, static analysis, and survey. The grammar system manages grammar installation, language detection, and language packs.

## Built-in Grammars

10 grammars are compiled into the binary (no download needed):

Go, TypeScript, TSX, JavaScript, Python, Rust, Java, C, C++, Zig

## Dynamic Grammars

Additional grammars can be downloaded from GitHub releases as platform-specific shared libraries:

```bash
aide grammar install ruby               # Install a specific grammar
aide grammar install --all              # Install all available grammars
aide grammar remove ruby                # Remove a downloaded grammar
aide grammar list                       # List all grammars (built-in + available + installed)
aide grammar list --installed           # Only show installed grammars
```

Dynamic grammars are stored in `.aide/grammars/<name>/` with SHA256 checksum verification and version tracking.

### Auto-Download

During normal operation (e.g., when the file watcher encounters a new language), grammars are downloaded automatically with exponential backoff on failure. Stale grammars (version mismatch with the running aide binary) are re-downloaded automatically.

### Lock Files

Grammar installations produce a `.aide-grammars.lock` file for reproducible team installs. Commit this to version control, and others can restore the exact set:

```bash
aide grammar install                    # Install from lock file (no args)
aide grammar install --no-lock          # Install without updating lock file
```

## Language Packs

Each grammar has a **language pack** (`pack.json`) that defines:

- **Meta**: File extensions, filenames, shebangs, aliases
- **Queries**: Tree-sitter tag and reference queries for symbol extraction
- **Complexity**: Function and branch node types for cyclomatic complexity analysis
- **Imports**: Regex patterns for dependency extraction
- **Tokenisation**: Node types for clone detection
- **Entrypoints**: Symbol and file patterns for survey entry point detection
- **Security**: Language-specific security rules (SQL injection, command injection, etc.)

32 language packs ship with AIDE, including 8 metadata-only packs (JSON, YAML, HTML, CSS, SQL, TOML, Dockerfile, Protobuf) that provide file detection without a tree-sitter parser.

## Project Scanning

Scan a project to detect which languages are used:

```bash
aide grammar scan                       # Scan current directory
aide grammar scan /path/to/project      # Scan specific path
aide grammar scan --json                # JSON output
```

Scanning uses a dual-pass approach:

1. **Project markers**: Detects languages from build files (e.g., `go.mod` implies Go, `Cargo.toml` implies Rust)
2. **File scan**: Catches languages that appear as scattered files without project markers (e.g., bash scripts)

## Project Marker Index

The project marker index lives in `packs/index.json` (a minimal stub) plus
per-topic partials under `packs/index.d/`:

```
packs/
  index.json                       # canonical (intentionally minimal)
  index.d/
    languages.json                 # go.mod, Cargo.toml, pyproject.toml, ...
    build-systems.json             # Make, CMake, Bazel, Just, Task, Rake, Nix
    containers.json                # Docker, docker-compose, Helm, Kustomize
    ci-cd.json                     # GitHub Actions, GitLab, Jenkins, ...
    monorepo.json                  # Nx, Lerna, Turbo, pnpm workspaces
    iac.json                       # Terraform, Pulumi, Ansible, ...
    dev-tooling.json               # pre-commit, renovate, dependabot, ...
    docs.json                      # MkDocs, Docusaurus, mdBook, ...
    consumer-formats.json          # .astro/.svelte/.vue/.mdx (see below)
```

All `*.json` files in `index.d/` are loaded in lexical order and merged into
the registry. This split keeps each topic file under ~50 lines and lets
contributors add a new tech stack by creating one focused partial instead of
editing a 600-line monolith. These markers feed the [survey](./survey.md)
topology analyzer.

### Consumer Formats

Some file formats consume code without containing parseable symbols
themselves — for example, an `.astro` file imports a React component and
references it by element name (`<App />`). These formats have no grammar
pack and produce no symbols, but reference verifiers (e.g. the deadcode
analyzer) need to scan them as plain text to catch use sites the index
cannot capture.

`packs/index.d/consumer-formats.json` declares them:

```json
{
  "consumer_formats": [
    { "extensions": [".astro"], "label": "astro" },
    { "extensions": [".svelte"], "label": "svelte" },
    { "extensions": [".vue"], "label": "vue" },
    { "extensions": [".mdx"], "label": "mdx" }
  ]
}
```

Consumer formats merge by `Label`; users can add or override via
`.aide/grammars/index.d/*.json`.

## Overriding Packs

Place custom `pack.json` files in `.aide/grammars/<language>/pack.json` to
override the built-in pack for a language. Custom packs are merged with the
built-in defaults (your file takes precedence).

For project markers and consumer formats, drop overrides into either
`.aide/grammars/index.json` (canonical) or `.aide/grammars/index.d/*.json`
(per-topic partials). All disk files load after the embedded defaults.
Project markers merge by `(File, Kind)`; consumer formats merge by
`Label`. Your entries override matching built-in entries while unmatched
built-in entries are preserved.
