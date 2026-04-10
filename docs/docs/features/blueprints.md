---
sidebar_position: 3
---

# Blueprints

Blueprints are portable, language-specific bundles of best-practice decisions that bootstrap a project with proven conventions. Instead of manually recording dozens of decisions one by one, `aide init` seeds your project's decision store from curated blueprints in a single command.

## Quick Start

```bash
# Seed Go best practices
aide init go

# Seed Go + CI/CD best practices (auto-includes general + github-actions)
aide init go go-github-actions

# Auto-detect from project markers (go.mod, Cargo.toml, .github/workflows, etc.)
aide init --detect

# Preview what would be imported
aide init --show go
```

Once imported, blueprint decisions work exactly like any other decision — they are injected into every session context and enforced by all agents.

## How It Works

A blueprint is a JSON file containing a list of decisions with topic, rationale, and detailed guidance. When you run `aide init <name>`, AIDE:

1. Resolves the blueprint (local override, embedded, or remote registry)
2. Follows the `includes` chain (e.g., `go` includes `general`)
3. Imports each decision into your project's decision store
4. Skips topics that already have a decision (use `--force` to overwrite)
5. Sets `decided_by: "blueprint:<name>"` for provenance tracking

## Shipped Blueprints

These blueprints ship with every AIDE release:

| Blueprint | Decisions | Includes | Description |
|-----------|-----------|----------|-------------|
| `general` | 5 | — | Universal practices: commits, PRs, dependencies, secrets, documentation |
| `go` | 18 | general | Idiomatic Go: error handling, context, testing, slog, huma, golangci-lint v2 |
| `github-actions` | 7 | general | Workflow security: SHA pinning, permissions, OIDC, branch protection |
| `go-github-actions` | 6 | github-actions | Go CI/CD: race detector, GoReleaser, matrix builds, zig cross-compilation |

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

When you run `aide init <name>`, AIDE looks for the blueprint in this order:

1. **Local override** — `.aide/blueprints/<name>.json` in your project
2. **Embedded** — shipped with the aide binary
3. **Remote registries** — each configured registry URL, in order

First match wins. Direct file paths and URLs bypass the resolution chain:

```bash
aide init ./our-practices.json                        # local file
aide init https://example.com/blueprints/rust.json    # direct URL
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
aide init myorg-standards    # fetches from registry
aide init --list             # shows all available (embedded + registry)
```

### One-Off Registry

```bash
aide init --registry=https://raw.githubusercontent.com/myorg/aide-blueprints/main go
```

## Local Overrides

Place a `<name>.json` file in `.aide/blueprints/` to override the embedded blueprint of the same name. This lets teams customise shipped blueprints without forking AIDE.

For example, to override the Go blueprint with stricter complexity thresholds:

```bash
# Copy the embedded blueprint as a starting point
aide init --show go > .aide/blueprints/go.json

# Edit as needed, then import
aide init go    # uses your local override
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
| `name` | Yes | Identifier used in `aide init <name>` |
| `display_name` | Yes | Human-readable name for `--list` output |
| `description` | Yes | One-line description |
| `version` | Yes | Tracks which AIDE release last modified the blueprint |
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

- **Skip on conflict** (default): If a topic already exists in the decision store, the blueprint decision is skipped. A notice is printed.
- **Force overwrite** (`--force`): Overwrites existing decisions with blueprint values.
- **Provenance**: All imported decisions have `decided_by: "blueprint:<name>"` for traceability.
- **Includes**: Resolved recursively with cycle detection. Included blueprints are imported before the parent.

## CLI Reference

```bash
aide init [flags] [blueprints...]

# Import blueprints
aide init go                          # Import Go best practices
aide init go go-github-actions        # Import multiple
aide init --detect                    # Auto-detect from project markers
aide init ./custom.json               # Import from local file
aide init https://example.com/bp.json # Import from URL

# Inspect
aide init --list                      # List all available blueprints
aide init --show go                   # Preview decisions without importing

# Options
aide init --force                     # Overwrite existing decisions
aide init --dry-run                   # Show what would happen
aide init --registry=URL go           # Use a one-off registry
aide init --no-cache go               # Force fresh fetch from remote
```

## Contributing Blueprints

Blueprints live in `aide/pkg/blueprint/blueprints/` as JSON files. To contribute:

1. Create or edit a `<name>.json` file following the schema above
2. Ensure decisions are actionable, rationale explains "why", and details explain "how"
3. Remove version-specific references — frame guidance as "latest stable" or "modern Go"
4. Submit a PR — CI validates the schema automatically

Blueprint versions for shipped blueprints track the AIDE release version. The version is only bumped when the blueprint content actually changes between releases.
