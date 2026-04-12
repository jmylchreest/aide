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

The project marker index (`packs/index.json`) contains 68+ detection rules for:

- Language project files (go.mod, Cargo.toml, package.json, pyproject.toml, etc.)
- Build systems (Make, CMake, Bazel, Just, Task, Rake)
- CI/CD (GitHub Actions, GitLab CI, Jenkins, CircleCI, Travis, Azure DevOps, Buildkite)
- Container/orchestration (Docker, Kubernetes, Helm)
- IaC (Terraform, Pulumi, Ansible, Crossplane)
- Monorepo tools (Nx, Lerna, Turborepo, pnpm workspaces)
- Documentation (MkDocs, Docusaurus, mdBook)

These markers are also used by the [survey](./survey.md) topology analyzer.

## Overriding Packs

Place custom `pack.json` files in `.aide/grammars/<language>/pack.json` to override the built-in pack for a language. Custom packs are merged with the built-in defaults (your file takes precedence).

Similarly, place a custom `index.json` in `.aide/grammars/index.json` to add or override project markers. Entries match by `(File, Kind)` composite key -- your entries override matching built-in entries while unmatched built-in entries are preserved.
