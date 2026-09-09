# Production review: retrieval guidance delivery

Scope: the new welcome guide in `src/core/session-init.ts`, OpenCode delivery changes in `src/opencode/hooks.ts`, and their tests in `src/test/session-init.test.ts` and `src/test/opencode-hints.test.ts`. Shared retrieval helpers, observation functions and host configuration were read as integration context. The experiment package and my own core/Go/skill implementation are not represented as independently reviewed here.

Verdict: approve this bounded production scope. No blocking defects or recorded-decision violations found. No production edits made during this review.

## Evidence

- E1: Read `git diff -- src/core/session-init.ts src/opencode/hooks.ts src/test/session-init.test.ts` plus the complete new `src/test/opencode-hints.test.ts`. Outlined all four scope files through code_outline. Changes are welcome strings, shared-helper delivery wiring, and focused contract tests; no resolver, survey, package, schema, storage or grammar implementation changes.
- E2: Read current `src/opencode/hooks.ts:888` (createToolAfterHandler, through line 1107), including identity setup, prior-read check before recordToolEvent, raw response recording before appended text, pruning-before-hint order, and appended-text accounting. Verified registration at hooks.ts:201; shared welcome invocations at hooks.ts:563 and :1205; Claude hook wrapper and call at `src/hooks/session-start.ts:343` and :569; Codex session-start registration at `src/cli/codex-config.ts:185`. Used code_references for buildWelcomeContext/createToolAfterHandler/toolFailureText and checked current source callpaths. Read `src/core/tool-observe.ts:145` failure classification.
- E3: Independently ran four suites: session-init, opencode-hints, opencode-retrieval, opencode-hooks — **33 tests passed**. Focused ESLint on the four reviewed files passed. Latest TypeScript build passed during implementation. Tests cover first read versus prior evidence, disabled code watching, bounded/small reads, absent matches/index failures, raw envelopes/errors, unchanged metadata, a single hint, and accounting of actual appended UTF-8 bytes.
- E4: Ran `rg -n 'buildWelcomeContext|createToolAfterHandler|session-start|flock|readlink|chmod|execSync|fetch\\('` against reviewed production files and host entrypoints; inspected the exact added diff to distinguish existing wrapper setup from changes. No new Unix-only commands, network requests, path construction or runtime dependency are introduced.
- E5: Called live findings_list separately for `src/core/session-init.ts` and `src/opencode/hooks.ts`, analyzer=deadcode, include_accepted=true, limit=100. Both returned no findings; actual reachability was additionally confirmed with E2.

## Findings and limits

The OpenCode correction is consequential: former before-hook code only logged suggestions. The after hook now appends model-visible guidance to supported rendered results. It checks prior-read evidence before observing the current read, preserves raw tool-response evidence, and records added text separately before measuring the complete adapter transformation. Failure envelopes and unsupported raw protocol results receive no retrieval hint.

The welcome guide is conditional, preserves text search/direct-read alternatives, and explicitly warns that empty or stale indexed results do not prove absence. It adds no mandatory code_stats call and makes no savings promise.

Search enrichment still performs existing synchronous local index queries (up to six bounded 3-second calls); moving delivery does not remove that latency. These tests verify the OpenCode adapter contract, not a live installed host/provider rendering. Installed package copies and Go tool descriptions require rebuild/reinstall or a fresh MCP process before evaluation. Session welcome text can be rebuilt at established session/compaction boundaries; the new section is emitted once per build, not on every tool call.

## Decision conformance

Live authoritative decision_list(origin=all) was loaded before implementation reads and refreshed for this review. Its full entries included the details required here; no decision_get fallback was necessary.

Decisions loaded: 27

- `survey-git-approach` — N/A — E1 changes no survey git operations or git implementation.
- `survey-run-identity` — N/A — E1 changes no survey entries, stamping, or freshness calculation.
- `pack-index-override` — N/A — E1 changes no pack loader or index.json merge behavior.
- `plugin-runtime` — PASS — E1 adds TypeScript hook logic and string guidance only; E4 finds no new shell/runtime mechanism. ESLint and TypeScript build pass.
- `plugin-version-pinning` — N/A — E1 changes no install/config version pin or package manifest.
- `project-identity-guard` — PASS — E1 leaves root resolution and memory injection boundaries unchanged; E2 uses existing state.cwd/establishContext and existing context identity.
- `store-routing` — PASS — E2 passes existing state.cwd into observation recording; E1 adds no store selection, parent bootstrap, or write routing.
- `survey-mcp-naming` — N/A — E1 adds no survey tool or naming change.
- `max-depth-convention` — N/A — E1 introduces no traversal or depth parameter.
- `memory-isolation` — PASS — E2 scopes added observations to state.cwd, sessionID and actor_id; E1 changes no memory fetching, cross-project transfer or inheritance.
- `memory-scoring` — N/A — E1 changes no memory selection/scoring; welcome text insertion leaves memory rendering intact.
- `no-aspirational-code` — PASS — E2 confirms real tool.execute.after registration and welcome callpaths; E5 deadcode findings include accepted entries and return none for both production files.
- `stored-id-determinism` — PASS — E2 adds no generated derived IDs/groupings; events reuse host sessionID/callID and stable literal labels.
- `survey-analyzers` — N/A — E1 changes no analyzer registration, stamping or grouping.
- `survey-entry-scope` — N/A — E1 adds no survey entries.
- `versioned-docs-generated` — N/A — E1 review scope contains no docs/versioned_docs files.
- `multi-arch-binary-distribution` — N/A — E1 changes no binary distribution, lookup or package metadata.
- `skills-architecture` — PASS — E1 adds plain string advice and no registry/network/discovery behavior; E4 finds no new fetch/execSync mechanism.
- `survey-code-graph` — N/A — E1 adds no graph storage or computation; guide labels impact results as candidates.
- `survey-store-location` — N/A — E1 changes no survey storage path or schema.
- `terminology-axes` — PASS — E1 introduces no scope:/origin: tags or --scope flags; E2 observation identity uses existing host/session/actor fields.
- `abandoned-approaches` — N/A — E1 adds no memory category or automated abandonment detection.
- `cross-platform-compatibility` — PASS — E2 routes shared helpers through OpenCode's actual rendered-result hook, with Claude/Codex welcome callpaths confirmed; E1 adds no platform-specific paths/commands and E3 exercises adapter contracts.
- `grammar-detection-dual-pass` — N/A — E1 changes no grammar detection or file scanning.
- `hud-wrapper-root-resolution` — N/A — E1 changes no HUD wrapper installation, pointer file or cache lookup.
- `non-language-detection` — N/A — E1 changes no pack labels/directories.
- `pack-index-architecture` — N/A — E1 changes no markers, pack.json or index.json.

