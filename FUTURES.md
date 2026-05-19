# aide FUTURES

Forward-looking ideas that aren't on the active roadmap yet. Items here are
exploratory — they should be promoted to `TODO.md` (or a design doc) before
implementation starts.

---

## Architectural Health Signal

A project-level continuous health score, computed from the existing code index,
that gives agents a gradient to optimise against during autonomous work
(autopilot, swarm). Today aide emits findings as a flat list; agents can
trivially "fix" one issue while making the project worse along another axis.
A single ungameable aggregate, paired with pointer-quality diagnostics,
closes that loop.

### Core idea

Compute a small set of normalised `[0,1]` graph-shape metrics over the code
index and aggregate them with a **geometric mean** (Nash Social Welfare).
Geometric mean is the anti-gaming property: improving one dimension while
tanking another cannot lift the aggregate, so an agent cannot "win" by
optimising one number.

### Candidate dimensions

All five are computable from data the index already has (symbols, imports,
call edges, file/symbol size):

1. **Modularity** — Newman's Q on the import graph. High Q ⇒ clean module
   boundaries.
2. **Acyclicity** — Tarjan SCC count → sigmoid. Cycles are penalised hard.
3. **Depth** — longest DAG path → sigmoid. Discourages deep dependency chains.
4. **Equality** — `1 − Gini(cyclomatic complexity per symbol)`. Penalises
   "god functions" that concentrate complexity.
5. **Redundancy** — `1 − (dead + duplicate fraction)`. Already partially
   covered by `findings/deadcode` and `findings/clone`.

### Outputs

- A `health` MCP tool returning
  `{ score, bottleneck_dimension, dimensions{...}, diagnostics{god_files,
  hotspots, deep_chains, complexity_outliers, dead_groups, duplicate_groups} }`.
- Findings of class `architecture/{cycle, god_file, deep_chain,
  complexity_outlier}` with file/symbol pointers, fed through the existing
  findings/triage flow.
- A `health.toml` (sibling to blueprints) for hard CI gates: `max_cycles=0`,
  `min_modularity=0.4`, layer/boundary deny rules. Violations become
  high-severity findings.

### Autopilot/swarm integration

The actual feedback loop — and the reason this is worth building:

- `health_snapshot` before a task batch / story.
- `health_diff` after.
- Block stage completion (or require an explicit `decision` approving the
  regression) if the aggregate score dropped. This makes the agent
  responsible for not silently degrading architecture while chasing a
  green test suite.

### Effort estimate

~1–2 weeks on top of the existing code index *if* import resolution
(below) is good enough that fan-in / cycles / modularity numbers aren't
dominated by same-name collisions. Most of the data is there; the cost
is metric math, normalisation, finding emitters, and the session-baseline
plumbing. Without import resolution, modularity (Newman Q on the import
graph) and acyclicity (SCCs) are the dimensions that degrade first.

### Out of scope

- GUI / treemap visualisation. Run-on-demand + autopilot hook is enough.
- Real-time file-watcher mode. Aide's existing `aide:patterns` cadence is
  sufficient.

---

## Test-Gap Detection (near-term — likely belongs in `findings/`)

A natural extension of the current findings analyzers (`complexity`,
`coupling`, `deadcode`, `clones`, `security`, `secrets`, `todos`).

**Heuristic**: surface symbols with **high fan-in** (many callers) that have
**no covering test** — i.e. the index has incoming call edges but no test-file
caller in the reverse-call set.

Aide already has the inputs:

- Call edges from the code index (`pkg/code`).
- Test-file detection (the topology / classification pass already
  distinguishes test files per language).

Proposed shape:

- New analyzer `pkg/findings/testgap/` registered alongside the others
  (mirrors `deadcode.go` / `coupling.go`).
- New constant `AnalyzerTestGap = "testgap"` in `pkg/findings/types.go`.
- Severity scaled by fan-in: high fan-in untested symbol → `warning`;
  exported + high fan-in untested → `critical`.
- Finding metadata carries `fanIn`, `callers`, and the symbol's qualified
  name so triage tooling can sort by impact.

This is small enough to ship independently of the broader health-signal
work above and gives agents an immediate "what should I write a test for
next?" signal.

**Accuracy depends on import resolution** (see next section). Without it,
fan-in counts and "is any caller a test file?" both leak across same-named
symbols. Acceptable for a first version (Aider's repo map ships with strictly
*no* resolution and is still useful); document the caveat and sharpen as
resolvers land per language.

---

## Cross-File Symbol Resolution

Both items above (and several existing features — `survey_graph`,
`code_references`, dead-code accuracy) are bottlenecked on the same problem:
**tree-sitter gives us the import statement's text, not what it resolves to**.
This is well-trodden ground in the OSS code-intelligence world; the consensus
is that there is no language-agnostic shortcut, and every serious tool either
ships per-language resolvers or accepts acknowledged fuzziness.

### What the field does

Two camps, with one experimental third:

1. **Real toolchain per language (precise).** SCIP indexers (`scip-go` shells
   to `go list`; `scip-typescript` uses the TS Compiler API; `scip-python`
   embeds Pyright; `scip-java` uses SemanticDB). CodeQL, Kythe, Glean wrap
   real compilers. `gopls`/`go-callvis` use `go/packages`. `dependency-cruiser`
   and `madge` use the real Node + tsconfig resolver. Same-name collisions
   are a non-issue because the compiler has already disambiguated.
   Cost: ship a build environment per language.

2. **Tree-sitter + heuristics (fuzzy, acknowledged).** Aider's repo map does
   *no* resolution at all — global name match + PageRank + LLM tolerance. Pure
   AST tools (`pyan`, `snakefood`, `findimports`) are best-effort. SLang-based
   Sonar analyzers and Semgrep OSS sit here too. This is where aide is today.

3. **`tree-sitter-stack-graphs` (declarative scope rules per language).**
   GitHub's middle path. Production-deployed for "precise code nav" on Python
   and TS/JS, Java in beta. Solves same-name collisions via scope-stack
   lookup, but only as faithfully as the per-language binding rules are
   written.

**There is no reusable cross-language import resolver.** Even hub
architectures (Kythe, Glean) only standardise the fact schema, not the
resolution logic.

### Realistic options for aide, ranked by cost

1. **Path-aware import edges (cheapest, biggest win).** Parse build files —
   `go.mod`, `Cargo.toml`, `tsconfig.json` paths, `package.json`,
   `pyproject.toml` — to map import-string → repo subtree. Resolve a call
   reference by intersecting the caller's import set (via the resolver) with
   the candidate target's package directory. ~1 week per language, no
   runtime toolchain dep, kills the cross-package collision case for
   fan-in/modularity/cycles. Aide's `pkg/survey` already detects most of
   these manifests during the topology pass.

2. **Adopt `tree-sitter-stack-graphs` for Python and TS/JS.** Inherit
   GitHub's binding rules. Heaviest payoff in the languages where build-file
   parsing is hardest (Python's `__init__.py` re-exports, TS's `paths` aliases
   layered on Node resolution). Adds a Rust runtime dependency (the engine is
   Rust); we'd FFI or shell out.

3. **Opportunistic toolchain shell-out.** When `go list` / `tsc` / `pyright`
   is on PATH and the project uses it, ingest its output (SCIP if available)
   for precise edges; fall back to options 1+2 otherwise. This is the model
   GitHub's precise code nav uses.

This work is also TODO.md §5's actual core — "implement import resolution
per language" — restated with field context. Item §5 step 1 (`@qualifier`
captures in tree-sitter ref queries) is still useful: it eliminates the
remaining ambiguity in the *one* case path-aware resolution can't solve
(a single file imports two same-named-export packages), and it captures
import aliases.

### Acceptable interim

For both the Architectural Health Signal and Test-Gap Detection, ship on
option-1 resolution for the languages it covers (start with Go + Rust),
fall back to current name-matching elsewhere with a documented caveat,
and progressively replace fallbacks as resolvers land. Don't gate either
feature on full multi-language resolution.
