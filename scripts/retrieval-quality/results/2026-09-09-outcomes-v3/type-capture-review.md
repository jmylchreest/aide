# Code Review: overlapping type captures

## Summary

No blocking defect found in the bounded change. The parser removes only a generic type capture whose name and complete definition byte range match a class/interface capture, retaining distinct declarations and existing traversal order.

## Decision Conformance

Scope checked: uncommitted changes in aide/pkg/code/parser.go, aide/pkg/code/parser_test.go, and aide/cmd/aide/cmd_mcp_retrieval_test.go.
Decisions loaded: 27, from live decision_list(origin="all").

Evidence P: rtk git diff --name-only confirms exactly these three Go files; rtk git diff and targeted reads show only extraction filtering and regression cases changed.
Evidence R: parser.go:267-278 calls extractWithQuery directly; code_references(symbol="extractWithQuery") returns that caller. findings_list(analyzer="deadcode", include_accepted=true) returned no findings for each of the three scoped files.

- ➖ plugin-runtime — n/a: P contains only Go backend/test code, explicitly excluded by this decision.
- ➖ survey-mcp-naming — n/a: P changes neither survey tools nor registrations.
- ➖ survey-run-identity — n/a: P changes no survey entries or run metadata.
- ➖ versioned-docs-generated — n/a: P includes no docs/versioned_docs paths.
- ✅ cross-platform-compatibility — conforms for the integration change: parser.go:282-385 uses Go slices/maps and tree-sitter byte ranges without platform-specific operations; cmd_mcp_retrieval_test.go:98-119 exercises the shared MCP handler with ordinary JSON inputs.
- ➖ hud-wrapper-root-resolution — n/a: P includes no HUD wrapper or installation/root-resolution code.
- ➖ pack-index-override — n/a: P includes no index.json or override merge code.
- ➖ project-identity-guard — n/a: P changes no project resolution or injection paths.
- ➖ store-routing — n/a: P changes no routing flags or store selection.
- ➖ survey-analyzers — n/a: P introduces no survey analyzers or schema changes.
- ➖ survey-code-graph — n/a: P changes neither BFS traversal nor graph persistence.
- ➖ survey-store-location — n/a: P changes no survey store paths.
- ➖ abandoned-approaches — n/a: P changes no memory category or recording instructions.
- ➖ max-depth-convention — n/a: P changes no traversal depth parameters.
- ➖ memory-scoring — n/a: P changes no memory scoring.
- ➖ non-language-detection — n/a: P changes neither marker labels nor grammar pack directories.
- ➖ pack-index-architecture — n/a: P changes no pack metadata or project markers; Go tag query remains unchanged at aide/pkg/grammar/packs/go/pack.json:15.
- ➖ plugin-version-pinning — n/a: P changes no installer or opencode.json handling.
- ➖ survey-git-approach — n/a: P introduces no Git operations.
- ➖ terminology-axes — n/a: P introduces no scope: tags, --scope flags, provenance tags, or write routing.
- ➖ multi-arch-binary-distribution — n/a: P changes no packaging or binary resolution.
- ➖ skills-architecture — n/a: P changes no skill discovery, management, or fetching.
- ✅ stored-id-determinism — changed filtering conforms: parser.go:329-340 keys by byte range/name and uses maps only for membership; parser.go:374-384 filters in original slice order. No map iteration or new ID/group assignment is introduced. Existing ULID/time assignment at parser.go:347-356 predates this diff and is not corrected here; this is not a claim of whole-parser ID determinism.
- ➖ survey-entry-scope — n/a: P changes code-index symbols, not survey entries.
- ➖ grammar-detection-dual-pass — n/a: P changes neither marker nor file-scan grammar detection.
- ➖ memory-isolation — n/a: P changes no cross-project memory/state movement.
- ✅ no-aspirational-code — conforms: R confirms the modified extraction function remains called by ParseContent; source snapshot parsing calls ParseContent at cmd_mcp_source.go:47, and the added Go tests call the real parser/handler.

## Findings

No critical or warning findings.

Optional coverage note addressed: inspected the added assertions at cmd_mcp_retrieval_test.go:118-130. The batch now requires the Reader interface heading, a direct kind=interface request requires the interface source, and a kind=type request for Record requires a current-source not-found error. These assertions pin the specialized classification contract. No outstanding review suggestions.

## Behavioral checks and limits

- Distinct candidates: filtering requires identical definition start byte, end byte, and name. Different kinds other than generic type are retained. Different names within a shared enclosing declaration are retained. Distinct same-name declarations on the same line have different byte ranges and survive; parser_test.go:188-197 includes this regression.
- Query order: specialized membership is completely collected before filtering; encountering type before or after class/interface changes neither the surviving set nor its traversal order. No in-place overwrite can affect an unvisited element because the write index never exceeds the read index.
- Kind filtering: class and interface remain canonical; standalone aliases/other generic types retain type. A request for kind=type for a struct/interface intentionally returns no current-source match. The handler's exact kind comparison at cmd_mcp_source.go:139 remains intact.
- Stale index: readOneSymbol deduplicates indexed candidate files (cmd_mcp_source.go:104-129) and reparses current bytes (39-51, 130-143), so historical type/class records do not recreate duplicate current definitions. Existing search results can still show both kinds until their file is reindexed. The unchanged 100-result index candidate cap applies before file deduplication, so sufficiently many stale duplicates can still require an explicit file. This patch neither migrates the index nor claims exhaustive discovery.
- Storage and security: no schema, store ID, query, IO boundary, or authorization change is introduced. IndexFileBatch replaces stored file symbols on reindex (aide/pkg/store/code.go:860-925).
- Review validation: all three files outlined through code_outline; targeted current-source reads, related code_search/code_references, live decisions, and accepted-inclusive deadcode findings inspected. No additional tests or broad analyzers were run by this reviewer; root owns test/build execution.
- Root-reported validation after the added assertions: full pkg/code and cmd/aide run passed with 442 passing test records, 35 skips for unavailable optional grammars, and zero failures; binary build and isolated real stdio MCP batch smoke also passed. This reviewer inspected the assertions without independently rerunning those checks.

## Verdict

Approve. The bounded normalization fixes false ambiguity without broad name/line deduplication or hidden candidate selection.
