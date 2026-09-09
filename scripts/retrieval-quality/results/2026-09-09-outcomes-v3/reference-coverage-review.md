# Code review: code_references coverage reporting

Verdict: Approve. No blockers or new correctness/security findings in the scoped change. This reviewer inspected tests without running them. Root reports seven focused cases verified red-to-green, the full cmd/aide suite passing in clean temporary storage (335 pass records including subtests, zero skipped/failed), and a successful binary build.

Scope: uncommitted changes in `aide/cmd/aide/cmd_mcp_code.go`, `aide/cmd/aide/cmd_mcp_code_helpers.go`, and new `aide/cmd/aide/cmd_mcp_code_references_test.go`.

## Findings and verification

- `cmd_mcp_code_helpers.go:83-98` reports returned indexed candidates and explicitly limits semantic coverage. At the cap, “more indexed matches may exist” is correct both when the store truncates and when the total exactly fits. Below-cap output makes no exhaustive semantic claim; empty output explicitly says absence is not proved.
- `cmd_mcp_code.go:406-445` passes the same positive, normalized limit into each search and its formatter, including batch results. Omitted/negative limits resolve to 50 (`constants.go:108`), avoiding the store's independent default of 100. No extra query or counting pass was introduced. `aide/pkg/store/code.go:599-643` confirms the store stops at its cap and returns no truncation flag.
- `cmd_mcp_code_references_test.go:13-67` uses the real store/handler and checks seven cases: truncated, exact-fit, below-limit, default, negative-default, filtered empty, and batch with an empty member. Assertions cover returned row count, qualified count, conditional cap uncertainty, qualified empty results, and semantic coverage text.
- The scoped diff introduces no provider or token-savings claim. Existing file-group map iteration in `cmd_mcp_code_helpers.go:105` is unchanged and is not expanded by this change.
- This review's live `code_references(symbol="formatCodeReferences", limit=10)` call reported the old “Found 2 references” header, while current source confirmed both new formatter call sites. That call did not validate patched output. The user rebuilt before this formatter fix; root's live confirmation covered the earlier committed build `51d42ec` and schema fix only. The new formatter was built to `/tmp` and tested, but the daemon was not restarted with it. This reviewer did not inspect experiment logs.

## Decision conformance

Loaded authoritative live `decision_list({})` before reading code. Decisions loaded: 27.

Evidence key: P = scoped `git diff --unified=4` plus complete new test read; only formatter text/signature, its two handler calls, and the new test change. O = `code_outline` on all three files. R = current-source reads of handler lines 385-449, formatter lines 83-120, and test lines 13-67. G = `rg -n 'exec\.|execSync|execFile|/bin/|scope:|--scope|origin:|[Ss]aving|provider|token'` across the three files. C = `rg -n 'formatCodeReferences|DefaultCodeRefsLimit|handleCodeReferences|func retrievalFixture' aide/cmd/aide --glob '*.go'`. F = `findings_list(analyzer="deadcode", file="aide/cmd/aide/cmd_mcp_code", include_accepted=true, limit=100)` returned no findings. Indexed `code_search` returned no SearchReferences definition, so store behavior was verified by `rg` and source read instead.

- ➖ `stored-id-determinism` — P/R: no stored IDs, assignments, or groupings change; pre-existing display map iteration is unchanged.
- ➖ `survey-code-graph` — P/O: no survey graph computation or persistence change.
- ➖ `survey-store-location` — P/O: no survey store/path change.
- ➖ `abandoned-approaches` — P: no memory category or skill-instruction change.
- ➖ `pack-index-override` — P: no pack loading or index override change.
- ➖ `project-identity-guard` — P/R: no root resolution or injection change; test reuses existing isolated fixture.
- ➖ `skills-architecture` — P/O: no skill discovery, installation, registry, or network change.
- ➖ `survey-entry-scope` — P: no survey entry change.
- ➖ `survey-run-identity` — P: no analyzer stamping, freshness, or metadata change.
- ✅ `cross-platform-compatibility` — R/G: added runtime code only formats strings using Go's fmt/strings; no shell, OS-specific path, executable, or assistant-specific protocol behavior. Test fixture uses t.TempDir/filepath.Join (`cmd_mcp_retrieval_test.go:20-35`).
- ➖ `grammar-detection-dual-pass` — P/O: no grammar detection change.
- ➖ `hud-wrapper-root-resolution` — P: no HUD wrapper or plugin-root resolution change.
- ➖ `max-depth-convention` — P/R: no max_depth input or traversal change; reference result limit is a separate existing option.
- ➖ `multi-arch-binary-distribution` — P: no packaging, download, or binary resolution change.
- ➖ `non-language-detection` — P: no marker labels or pack directory change.
- ➖ `store-routing` — P/R: runtime path is the existing read-only reference search; no write routing or store bootstrap change.
- ➖ `survey-analyzers` — P/O: no survey analyzer or schema change.
- ➖ `memory-isolation` — P/R: no memory, project-state sharing, or cross-project boundary change.
- ➖ `pack-index-architecture` — P: no packs/index.json or language pack change.
- ➖ `plugin-version-pinning` — P: no opencode configuration or version pin change.
- ➖ `survey-git-approach` — P/R/G: no git runtime operation added or changed.
- ➖ `survey-mcp-naming` — P/O: no survey MCP tool addition or rename.
- ➖ `terminology-axes` — P/G: no governed scope:/origin: tag, --scope flag, or anchor-chain terminology change.
- ➖ `versioned-docs-generated` — P: no docs/versioned_docs path changes in this scoped patch.
- ➖ `memory-scoring` — P/O: no memory score, ordering, or scoring environment variable change.
- ✅ `no-aspirational-code` — C/R/F: registered code_references handler (`cmd_mcp_code.go:154`) calls the updated formatter at lines 425 and 445; new test calls that handler at line 39. Deadcode lookup including accepted findings returned none; direct current-source call paths confirm reachability.
- ➖ `plugin-runtime` — P/O/G: this change is in the Go backend, explicitly excluded by the decision; no TypeScript or shell runtime change.

No recorded decision is violated or has a pre-existing violation extended by the scoped patch.
