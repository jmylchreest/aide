# Follow-up change review

Scope: the two guidance strings in `src/core/session-init.ts`, their existing regression test in `src/test/session-init.test.ts`, and this analysis directory. No frozen results or runtime accounting changes. Independent review found one ambiguity: source verification could imply a duplicate direct read. Resolved by explicitly allowing current symbol-body output as evidence. No remaining actionable findings.

## Verification

- `bun run test --run src/test/session-init.test.ts src/test/opencode-hooks.test.ts src/test/codex-integration.test.ts`: 28 tests pass.
- `python3 analyze.py --check --verify-raw`: all frozen result hashes, response/call associations and derived counters match.
- `python3 scripts/retrieval-quality/outcomes-v5/run.py verify`: frozen v5 protocol and inherited v4 corpus/helpers match.
- Independent reviewer reproduced accounting and checked overlap, first-edit boundaries, quality conclusions and document links.
- Runtime changes are plain text; the analysis reads trusted local artifacts and never executes captured tool inputs. Raw logs remain local; only selected counters and identifiers appear in the derived report.

## Decision conformance

Decisions loaded: 27, from live `decision_list(origin="all")` before code investigation.

Evidence references: **D** is the full diff and new-file inventory above; **G** is `src/core/session-init.ts:581–591` (guidance strings only); **H** is `src/hooks/session-start.ts:343–348`, `src/opencode/hooks.ts:563`, `src/cli/codex-config.ts:185`, `src/cli/hook.ts:20`, and `.claude-plugin/plugin.json:35` (shared welcome and hook wiring); **A** is `analyze.py` in this directory, executed both to generate and check its output. Paths and command examples were checked from the repository root, with RTK prefixes when executed.

- `skills-architecture` — not applicable: D changes no skill discovery, registry or fetching logic.
- `stored-id-determinism` — not applicable to stored grouping IDs: D adds no clustering or ID allocation; A deterministically preserves existing response/call IDs.
- `survey-analyzers` — not applicable: D contains no analyzer definitions or schema changes.
- `survey-code-graph` — not applicable: D contains no graph computation or storage.
- `grammar-detection-dual-pass` — not applicable: D contains no grammar scanning or marker logic.
- `memory-scoring` — not applicable: G changes retrieval prose only; D changes no scores or decay rules.
- `multi-arch-binary-distribution` — not applicable: D changes no packaging or binary resolution.
- `pack-index-architecture` — not applicable: D contains no pack files or index implementation.
- `pack-index-override` — not applicable: D contains no pack override or merge logic.
- `project-identity-guard` — not applicable: G adds no resolution or injection routing; A resolves its own analysis artifact directory, not a project anchor.
- `survey-git-approach` — not applicable: D introduces no survey or Git operations.
- `survey-mcp-naming` — not applicable: G names only existing code tools; D introduces no survey tools.
- `max-depth-convention` — not applicable: D introduces no depth arguments or tree traversal.
- `memory-isolation` — not applicable: D changes no memory reads, writes or sharing; A reads only the named experiment's evidence.
- `plugin-version-pinning` — not applicable: D changes no installation config, version or dependency pins.
- `store-routing` — not applicable: D contains no store selection, creation or write routing.
- `survey-run-identity` — not applicable: D contains no survey run metadata or freshness computation.
- `survey-store-location` — not applicable: D contains no survey storage changes.
- `terminology-axes` — not applicable: D adds no `scope:`/`origin:` tags or routing flags.
- `hud-wrapper-root-resolution` — not applicable: D changes no HUD wrapper or root resolution.
- `plugin-runtime` — conforms: G is TypeScript string content; A is development-only analysis, with no new plugin runtime entry or dependency.
- `survey-entry-scope` — not applicable: D introduces no survey entries.
- `versioned-docs-generated` — conforms: D adds analysis under `scripts/retrieval-quality/analysis/`; no `docs/versioned_docs/` files changed.
- `abandoned-approaches` — not applicable: D changes no memory categories or detection.
- `cross-platform-compatibility` — conforms: G changes shared text, introduces no OS-specific execution, and H confirms the shared host paths; 28 platform-independent hook/context tests pass. This does not claim live trials on other operating systems or hosts.
- `no-aspirational-code` — conforms: H proves the existing welcome builder is called; `findings_list(analyzer="deadcode", file="src/core/session-init.ts", include_accepted=true)` returned no findings. A was executed successfully. Future experiments are described only in Markdown.
- `non-language-detection` — not applicable: D contains no detection packs, labels or index changes.
