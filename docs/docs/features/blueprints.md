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
| `general` | 7 | documentation-core | Universal practices: commits, PRs, dependencies, secrets, text/time foundations, aide memory & decision persistence |
| `go` | 18 | general | Idiomatic Go: error handling, context, testing, slog, huma, golangci-lint v2 |
| `rust` | 12 | general | Idiomatic Rust: error handling, testing, async, and tooling |
| `c` | 12 | general | Modern C: safety hardening, memory management, testing |
| `cpp` | 12 | general | Modern C++ (C++20/23): Core Guidelines, RAII, smart pointers |
| `zig` | 12 | general | Idiomatic Zig: allocator patterns, comptime, error handling |
| `python` | 20 | general | Idiomatic modern Python: pyproject + uv, strict typing, Ruff, pytest, structured logging, asyncio |
| `existing-software-project` | 14 | — | Brownfield deference: the repository's own conventions, toolchain, lint config, and VCS style outrank general best practice; minimal blast radius; verification parity with CI. Opt-in: import when working in a codebase you did not create |
| `i18n` | 10 | — | Internationalisation & localisation: BCP 47 scope, ICU plurals, CLDR locale data, IANA time zones, Unicode handling, RTL layout, pseudo-localisation, translation workflow, HTTP API negotiation. Opt-in: include explicitly when serving more than one locale |
| `github-actions` | 7 | general | Workflow security: SHA pinning, permissions, OIDC, branch protection |
| `go-github-actions` | 5 | github-actions | Go CI/CD: golangci-lint, matrix builds, cross-compilation, releases |
| `rust-github-actions` | 7 | github-actions | Rust CI/CD: clippy, cargo-nextest, cargo-deny, sanitizers, coverage |
| `csharp` | 20 | general | Idiomatic modern C# / .NET: nullable reference types, records, async, DI, System.Text.Json, analysers, tooling |
| `csharp-github-actions` | 7 | github-actions | C# CI/CD: setup-dotnet, locked restore, warnings-as-errors, NuGet audit, coverage, releases |
| `csharp-unity` | 15 | general | Unity (C#): thin MonoBehaviours, ScriptableObject architecture, zero-alloc hot paths, Input System, Job System, Addressables, IL2CPP |
| `python-github-actions` | 7 | github-actions | Python CI/CD: uv, frozen installs, interpreter matrix, Ruff, strict typing, pip-audit, PyPI Trusted Publishing |
| `python-api` | 11 | python | Python API services: framework selection, ASGI, validated boundaries, SQLAlchemy + Alembic, auth, queues, OpenTelemetry. Opt-in |
| `python-django` | 16 | python | Django: app layout, env-driven settings, ORM query discipline, DB constraints, reversible migrations, DRF/Ninja, security defaults, deployment |

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

### C# Blueprint Decisions

The `csharp` blueprint covers:

- **csharp-language-and-runtime-version** — Current LTS TFM as source of truth; LangVersion tracks it; pin the SDK via global.json
- **csharp-project-layout** — Lean SDK-style csproj; centralise in Directory.Build.props / Directory.Packages.props
- **csharp-nullable-reference-types** — Enable nullable reference types solution-wide; model nullability in the type system
- **csharp-type-design** — Seal types not designed for extension; records for data; small immutable structs; flat types
- **csharp-immutability-and-records** — Records with init-only members; read-only collection surfaces; with-expressions
- **csharp-pattern-matching** — Switch expressions and recursive patterns; compiler-enforced exhaustiveness
- **csharp-async-await** — Async all the way; flow CancellationToken; ConfigureAwait(false) in libraries
- **csharp-error-handling** — Precise exceptions + throw-helpers; expected failures via Try-patterns/nullable returns
- **csharp-collections-and-linq** — LINQ for transforms; read-only abstractions; Span/pooling on measured hot paths
- **csharp-naming-conventions** — PascalCase/camelCase/_camelCase, I- and T- prefixes, Async suffix
- **csharp-comments-and-docs** — XML doc comments on the public API; inline // reserved for the why
- **csharp-dependency-injection** — Constructor injection via Microsoft.Extensions.DependencyInjection; matched lifetimes
- **csharp-configuration-and-options** — Options pattern bound to sealed POCOs; validate eagerly at startup
- **csharp-logging** — Microsoft.Extensions.Logging; structured templates; [LoggerMessage] for hot paths
- **csharp-json-serialisation** — System.Text.Json; source-generated context for AOT/perf; cached options
- **csharp-testing** — xUnit or NUnit, Arrange-Act-Assert, table-driven cases, NSubstitute/Moq, TimeProvider
- **csharp-resource-management** — IDisposable/IAsyncDisposable in sealed owners; using / await using
- **csharp-web-api** — ASP.NET Core Minimal APIs/controllers; TypedResults; OpenAPI; ProblemDetails
- **csharp-analysers-and-format** — .NET analysers + .editorconfig + dotnet format; warnings as errors in CI
- **csharp-dependency-management** — Central Package Management (multi-project); lock files; NuGet audit

The `csharp-github-actions` blueprint covers:

- **csharp-ci-workflow** — setup-dotnet via global.json; locked restore; build/test; format check
- **csharp-ci-caching** — NuGet cache keyed on packages.lock.json; locked mode
- **csharp-ci-quality** — Warnings-as-errors in CI, analysers/code-style in build, dotnet format gate
- **csharp-ci-security** — NuGet Audit promoted to errors; CodeQL; Dependabot/dependency-review
- **csharp-ci-matrix** — ubuntu/windows/macos matrix (fail-fast: false); TFM axis for multi-targeting
- **csharp-ci-test-coverage** — TRX + Cobertura, published, with a threshold gate
- **csharp-release** — Tag-driven; MinVer/Nerdbank.GitVersioning; NuGet OIDC Trusted Publishing; self-contained app assets

### Unity Blueprint Decisions

The `csharp-unity` blueprint is **self-contained** (includes only `general`, not `csharp`) because Unity idioms supersede the mainstream ones — `== null` over `is null`, coroutines/Awaitable over Task-everywhere, the Unity Test Framework over xUnit, `.asmdef` over SDK-style csproj, and zero-allocation hot paths. It covers:

- **unity-runtime-and-versioning** — Pin the Unity LTS via ProjectVersion.txt; .NET Standard 2.1; IL2CPP for release
- **unity-assembly-definitions** — Partition with .asmdef (runtime/editor/test); one-way dependencies
- **unity-monobehaviour-design** — Thin sealed adapters delegating to plain testable classes; lifecycle by cadence
- **unity-object-lifetime-and-equality** — The `== null` overload; TryGetComponent; Destroy vs DestroyImmediate
- **unity-serialisation-and-scriptableobjects** — [SerializeField] private fields; ScriptableObject config
- **unity-update-loop-performance** — Zero-allocation hot paths; cache references; non-alloc APIs
- **unity-memory-and-gc** — Pool with UnityEngine.Pool; reuse buffers; keep incremental GC on
- **unity-async-coroutines-awaitable** — Coroutines / Awaitable / UniTask on the main loop; destroyCancellationToken
- **unity-physics-and-timing** — Rigidbody motion in FixedUpdate; input in Update; interpolation
- **unity-input-system** — The Input System package; Input Action Assets; polling + phase callbacks
- **unity-architecture-and-di** — Composition; ScriptableObject event channels or a DI container (VContainer)
- **unity-jobs-and-burst** — The Job System + Burst over NativeArray for measured CPU hot work
- **unity-testing** — Unity Test Framework: EditMode (NUnit) + PlayMode ([UnityTest])
- **unity-assets-and-addressables** — Addressables over Resources; async load + release handles
- **unity-build-il2cpp-aot** — IL2CPP/AOT-first; preserve stripped types with [Preserve]/link.xml

### Python Blueprint Decisions

The `python` blueprint covers:

- **python-version-policy** — `requires-python` as the supported range; test every version you claim
- **python-packaging-and-metadata** — All metadata in `pyproject.toml`; `src/` layout; no `setup.py`
- **python-environments-and-dependencies** — uv-managed interpreter and env; committed lock; frozen installs
- **python-type-annotations** — Annotate everything; built-in generics and `X | None`; `Protocol` over base classes
- **python-static-type-checking** — Strict mypy or Pyright as a required CI gate; narrow, justified ignores only
- **python-linting-and-formatting** — Ruff as the single linter *and* formatter; broad rule selection
- **python-project-structure** — Domain packages over technical layers; acyclic imports; explicit public surface
- **python-error-handling** — One exception hierarchy; catch narrowly; always `raise ... from`
- **python-logging** — stdlib `logging` with `__name__` loggers; no f-strings in log calls; configure only at the entry point
- **python-data-modelling** — Frozen dataclasses internally, Pydantic at boundaries, enums for closed sets
- **python-async** — Async only for I/O concurrency; never block the loop; `TaskGroup` + timeouts
- **python-concurrency-and-parallelism** — Threads for I/O, processes for CPU; bound every pool
- **python-testing** — pytest, plain asserts, fixtures, parametrize, one behaviour per test
- **python-test-doubles-and-isolation** — Fakes over mocks; patch where used; no network in unit tests
- **python-configuration-and-secrets** — One validated settings object built at startup, never `os.environ` inline
- **python-cli** — Typer/Click behind `[project.scripts]`; the command is a thin shell over a library
- **python-performance** — Profile first; better algorithm or library before micro-optimisation
- **python-security** — No `pickle`/`eval`/`shell=True` on untrusted input; `pip-audit` in CI
- **python-docstrings-and-comments** — Docstrings carry the contract; inline `#` carries the *why*
- **python-imports-and-module-execution** — Absolute, top-level, side-effect-free; guard with `__main__`

`python-api` (opt-in, includes `python`) covers HTTP API services: framework selection (FastAPI as the
default for new APIs, Flask where already established, Django via its own blueprint), ASGI serving,
Pydantic-validated request and response contracts, code-generated OpenAPI with RFC 9457 error bodies,
lifespan-scoped dependencies, SQLAlchemy 2.0 + Alembic, auth, durable background work, OpenTelemetry,
in-process testing, and container deployment.

`python-django` (includes `python`) opens with **django-supersession**, which states precisely which
core Python decisions Django overrides — configuration, persistence, and boundary validation — and which
still apply unchanged.

### Working in an Existing Codebase

`existing-software-project` is a **modifier**, not a language blueprint. It includes nothing and is
imported alongside whichever language blueprints apply:

```bash
aide blueprint import existing-software-project python
```

Its first decision, **existing-codebase-precedence**, states the ordering every other decision defers to:

1. Explicit instruction in the current task
2. Recorded `aide` decisions for this project
3. Conventions observable in the surrounding code and committed config
4. Language and framework blueprints
5. General best practice

All 14 decisions carry `precedence: 100` (via the blueprint's `default_precedence`), so they are injected
in a `## Overriding Decisions` block ahead of every ordinary decision — see [Precedence](#precedence).

**existing-vcs-conventions** is a worked example of that override — it explicitly supersedes the `general`
blueprint's Conventional Commits default when the repository's own history does not use it. The remaining
decisions cover convention discovery, toolchain fidelity, linter config as binding, blast radius,
dependency restraint, test conventions, error and logging idiom, naming and comment idiom, public contract
stability, generated and vendored code, verification parity with CI, and modernisation as explicit work.

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
      "references": ["https://example.com/docs"],
      "precedence": 0
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
| `default_precedence` | No | Injection weight applied to every decision that does not set its own. Defaults to `0`. See [Precedence](#precedence) |
| `decisions` | Yes | Array of decision objects |

### Decision Fields

| Field | Required | Description |
|-------|----------|-------------|
| `topic` | Yes | Unique key in the decision store |
| `decision` | Yes | Short summary — the "what" |
| `rationale` | Yes | The "why" — reasoning behind the decision |
| `details` | No | Extended guidance — the "how" |
| `references` | No | URLs to relevant documentation |
| `precedence` | No | Overrides the blueprint's `default_precedence` for this decision |

## Precedence

Decisions are injected into every session as a flat list. Precedence controls **the order they
appear in and whether they claim authority over each other**.

Precedence is a plain integer sort key — higher is injected earlier — but one value carries meaning:

| Precedence | Where it renders | What it claims |
|-----------|------------------|----------------|
| `>= 100` | `## Overriding Decisions`, ahead of everything else | "Where this conflicts with anything below, this wins" |
| `1`–`99` | `## Project Decisions`, above the `0`s | Nothing. Ordering only |
| `0` (default) | `## Project Decisions` | Nothing |
| negative | `## Project Decisions`, last | Nothing |

So a decision at `200` renders in the overriding block above one at `100`, while a decision at `80`
stays in the ordinary block — sorted above the defaults, but making no claim over them. Only the
`100` threshold changes the *claim*; every other value changes only the *order*.

Both halves matter. Sorting alone does not create precedence — putting a decision first does not tell
an agent that it wins — so the overriding block carries an explicit header stating the relationship,
and a guardrail decision should still say in its own text what it overrides.

`existing-software-project` is the only shipped blueprint above the threshold
(`default_precedence: 100`). Language blueprints stay at `0`; if everything were an override, the
distinction would carry no information.

### Setting precedence on a decision

```bash
aide decision set house-style "Follow the repo's existing conventions" --precedence=100
```

**Omitting `--precedence` on an existing topic carries the current value forward.** This matters
because writes are append-only and only the newest revision is injected: without carry-forward,
rewording a guardrail — from the CLI, from the web UI, or by any partial update — would silently
demote it out of the overriding block. Pass `--precedence=0` to demote deliberately.

## Import Semantics

- **Skip on conflict** (default): If a topic already exists and was set by the user (or a different blueprint), it is skipped.
- **Version-aware upgrade**: If a topic was previously imported from the same blueprint, and the blueprint version is newer with changed content, a new decision version is appended that supersedes the old one. The old version is preserved in history (`aide decision history <topic>`). A change to `precedence` alone counts as changed content, so re-weighting a decision is a real upgrade rather than a silent no-op.
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
