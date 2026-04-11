---
sidebar_position: 3
---

# Blueprints

Blueprints are portable, language-specific bundles of best-practice decisions that bootstrap a project with proven conventions. Instead of manually recording dozens of decisions one by one, `aide blueprint import` seeds your project's decision store from curated blueprints in a single command.

## Quick Start

```bash
# Seed Go best practices
aide blueprint import go

# Seed Go + CI/CD best practices (auto-includes general + github-actions)
aide blueprint import go go-github-actions

# Auto-detect from project markers (go.mod, Cargo.toml, .github/workflows, etc.)
aide blueprint import --detect

# Preview what would be imported
aide blueprint show go
```

Once imported, blueprint decisions work exactly like any other decision — they are injected into every session context and enforced by all agents.

## How It Works

A blueprint is a JSON file containing a list of decisions with topic, rationale, and detailed guidance. When you run `aide blueprint import <name>`, AIDE:

1. Resolves the blueprint (local override, embedded, or remote registry)
2. Follows the `includes` chain (e.g., `go` includes `general`)
3. Imports each decision into your project's decision store
4. Skips user-set decisions; upgrades blueprint-set decisions if the version is newer and content changed
5. Sets `decided_by: "blueprint:<name>@<version>"` for provenance tracking

## Auto-Detection

`aide blueprint import --detect` scans your project and imports matching blueprints automatically. Detection is powered by the same [project marker index](./grammar.md#project-marker-index) used by the grammar/pack system.

For example, a project containing `go.mod` and `.github/workflows/` triggers markers for the `go` pack and `github-actions` label. AIDE then checks for matching blueprints:

1. **Direct match** — pack name `go` → `go` blueprint
2. **Label match** — label `github-actions` → `github-actions` blueprint
3. **Compound match** — pack `go` + label `github-actions` → `go-github-actions` blueprint

```bash
$ aide blueprint import --detect
Detected: go, github-actions, go-github-actions

  general            5 new
  go                 18 new
  github-actions     7 new
  go-github-actions  5 new

35 imported, 0 updated
```

Custom markers in `.aide/grammars/index.json` are included in detection, so org-specific tooling can automatically trigger custom blueprints.

## Shipped Blueprints

These blueprints ship with every AIDE release:

| Blueprint | Decisions | Includes | Description |
|-----------|-----------|----------|-------------|
| `general` | 5 | — | Universal practices: commits, PRs, dependencies, secrets, documentation |
| `go` | 18 | general | Idiomatic Go: error handling, context, testing, slog, huma, golangci-lint v2 |
| `rust` | 12 | general | Idiomatic Rust: error handling, testing, async, and tooling |
| `c` | 12 | general | Modern C: safety hardening, memory management, testing |
| `cpp` | 12 | general | Modern C++ (C++20/23): Core Guidelines, RAII, smart pointers |
| `zig` | 12 | general | Idiomatic Zig: allocator patterns, comptime, error handling |
| `github-actions` | 7 | general | Workflow security: SHA pinning, permissions, OIDC, branch protection |
| `go-github-actions` | 5 | github-actions | Go CI/CD: golangci-lint, matrix builds, cross-compilation, releases |
| `rust-github-actions` | 7 | github-actions | Rust CI/CD: clippy, cargo-nextest, cargo-deny, sanitizers, coverage |

### Go Blueprint Decisions

The `go` blueprint covers:

- **go-version-policy** — Always target latest stable Go; go.mod as single source of truth
- **go-module-management** — go mod tidy, go tool directives, no vendoring unless justified
- **go-project-structure** — cmd/internal/pkg layout; flat for small projects
- **go-error-handling** — Always wrap with `%w`, errors.Is/As, never nil/nil
- **go-context** — First param ctx, typed keys, defer cancel
- **go-naming** — Short packages, -er interfaces, Err prefix, MixedCaps
- **go-interfaces** — Consumer-side, 1-3 methods, accept interfaces return structs
- **go-concurrency** — errgroup over WaitGroup, bound goroutines, channels vs mutexes
- **go-goroutine-lifecycle** — Every goroutine must have a guaranteed exit path
- **go-cgo** — Prefer CGO_ENABLED=0; use zig for cross-compilation when CGO is required
- **go-logging** — log/slog only, structured key-value, no third-party loggers
- **go-testing** — Table-driven, t.Parallel, race detector, testing/synctest
- **go-stdlib-preference** — Prefer modern stdlib over third-party equivalents
- **go-rest-api** — huma v2 + chi for OpenAPI 3.1 spec-driven REST APIs
- **go-async-api** — AsyncAPI spec-first for event-driven microservices
- **go-third-party** — Justified library choices: chi, cobra, pgx, sqlc
- **go-tooling** — gofmt, golangci-lint v2, govulncheck
- **go-import-grouping** — Three groups: stdlib, third-party, internal

## Resolution Order

When you run `aide blueprint import <name>`, AIDE looks for the blueprint in this order:

1. **Local override** — `.aide/blueprints/<name>.json` in your project
2. **Embedded** — shipped with the aide binary
3. **Remote registries** — each configured registry URL, in order

First match wins. Direct file paths and URLs bypass the resolution chain:

```bash
aide blueprint import ./our-practices.json                        # local file
aide blueprint import https://example.com/blueprints/rust.json    # direct URL
```

## Remote Registries

A registry is just a base URL that serves `<name>.json` files. Any static file host works — GitHub, GitLab, S3, or a plain web server.

### Setting Up an Org Registry

1. Create a repository with blueprint JSON files:

```
myorg/aide-blueprints/
├── go.json
├── rust.json
└── myorg-standards.json
```

2. Configure the registry URL in your project:

```json
// .aide/config/aide.json
{
  "blueprints": {
    "registries": [
      "https://raw.githubusercontent.com/myorg/aide-blueprints/main"
    ]
  }
}
```

3. Import:

```bash
aide blueprint import myorg-standards    # fetches from registry
aide blueprint list                      # shows all available (embedded + registry)
```

### One-Off Registry

```bash
aide blueprint import --registry=https://raw.githubusercontent.com/myorg/aide-blueprints/main go
```

## Local Overrides

Place a `<name>.json` file in `.aide/blueprints/` to override the embedded blueprint of the same name. This lets teams customise shipped blueprints without forking AIDE.

For example, to override the Go blueprint with stricter complexity thresholds:

```bash
# Copy the embedded blueprint as a starting point
aide blueprint show go > .aide/blueprints/go.json

# Edit as needed, then import
aide blueprint import go    # uses your local override
```

## Blueprint Schema

```json
{
  "schema_version": 1,
  "name": "my-blueprint",
  "display_name": "My Blueprint",
  "description": "What this blueprint covers",
  "version": "0.0.0",
  "tags": ["language", "go"],
  "includes": ["general"],
  "decisions": [
    {
      "topic": "my-topic",
      "decision": "Short summary of the decision",
      "rationale": "Why this decision was made",
      "details": "Extended guidance on how to apply it",
      "references": ["https://example.com/docs"]
    }
  ]
}
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `schema_version` | Yes | Always `1` for now |
| `name` | Yes | Identifier used in `aide blueprint import <name>` |
| `display_name` | Yes | Human-readable name for `--list` output |
| `description` | Yes | One-line description |
| `version` | Yes | Semver version; used for version-aware upgrade during import |
| `tags` | No | Searchable tags |
| `includes` | No | Other blueprints to import first (resolved recursively) |
| `decisions` | Yes | Array of decision objects |

### Decision Fields

| Field | Required | Description |
|-------|----------|-------------|
| `topic` | Yes | Unique key in the decision store |
| `decision` | Yes | Short summary — the "what" |
| `rationale` | Yes | The "why" — reasoning behind the decision |
| `details` | No | Extended guidance — the "how" |
| `references` | No | URLs to relevant documentation |

## Import Semantics

- **Skip on conflict** (default): If a topic already exists and was set by the user (or a different blueprint), it is skipped.
- **Version-aware upgrade**: If a topic was previously imported from the same blueprint, and the blueprint version is newer with changed content, a new decision version is appended that supersedes the old one. The old version is preserved in history (`aide decision history <topic>`).
- **Force overwrite** (`--force`): Overwrites all existing decisions regardless of source or version.
- **Provenance**: Imported decisions have `decided_by: "blueprint:<name>@<version>"` for traceability (e.g., `blueprint:go@0.1.0`).
- **Includes**: Resolved recursively with cycle detection. Included blueprints are imported before the parent.

## CLI Reference

```bash
# List and inspect
aide blueprint list                               # List all available blueprints
aide blueprint show go                            # Preview decisions without importing

# Import blueprints
aide blueprint import go                          # Import Go best practices
aide blueprint import go go-github-actions        # Import multiple
aide blueprint import --detect                    # Auto-detect from project markers
aide blueprint import ./custom.json               # Import from local file
aide blueprint import https://example.com/bp.json # Import from URL

# Import options
aide blueprint import --force go                  # Overwrite existing decisions
aide blueprint import --dry-run go                # Show what would happen
aide blueprint import --registry=URL go           # Use a one-off registry
```

## Contributing Blueprints

Blueprints live in `aide/pkg/blueprint/blueprints/` as JSON files. To contribute:

1. Create or edit a `<name>.json` file following the schema above
2. Ensure decisions are actionable, rationale explains "why", and details explain "how"
3. Remove version-specific references — frame guidance as "latest stable" or "modern Go"
4. Submit a PR — CI validates the schema automatically

Blueprint versions for shipped blueprints are automatically bumped to match the release version by `make release` when the blueprint content has changed since the last tag.
