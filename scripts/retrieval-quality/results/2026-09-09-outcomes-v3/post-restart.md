# Post-restart follow-up — September 9, 2026

The live CLI and MCP daemon both report `0.1.18-dev.29+51d42ec`, built
2026-09-09T09:12:29Z. MCP `instance_info` resolved the intended project root and
reported daemon authority. The new Code Retrieval welcome text appeared in this
resumed host context. Live definition search and symbols-only reference/source
requests reached their handlers; the previously fixed alternative-selector schema
is accepted by the installed MCP endpoint.

The pending reviewed results were committed in `b68b5c8` using the configured SSH
signer. The commit contains an SSH signature. Local signature verification is not
configured (`gpg.ssh.allowedSignersFile` is absent), so verification was not claimed.
The earlier validation and review files preserve their historical signing-blocked
snapshot; this follow-up records its resolution without rewriting that evidence.

A bounded live reference query for `buildWelcomeContext` returned 12 rows with a
limit of 12 and the text “Found 12 references”. It did not identify the limit in
the result. The follow-up fix qualifies returned counts and reports that more
indexed matches may exist when the effective limit is reached. It does not fetch
extra rows, count all matches, or claim a complete call graph. Empty results are
also qualified. This affects the common MCP output used by all hosts.

These are operational checks and a reporting correction, not additional v3 trials.
They do not change the frozen results, overhead cutoff, quality grades or savings
conclusions. New live-host performance trials on Claude Code/OpenCode remain
unperformed. The follow-up formatter requires a subsequent normal binary rebuild;
the checked running daemon was not replaced during this work.

## Follow-up validation

- Seven real-store/handler cases failed before the formatter change and passed
  afterward: limited, exact-fit, below-limit, default, negative-default, filtered
  empty, and a mixed batch.
- The complete `go test ./cmd/aide -count=1` suite passed with 335 passing test
  records (including subtests), no failures and no skips, using disposable clean
  temporary storage. Its initial run failed an anchor test because existing
  `/tmp/.git` and `/tmp/.aide` markers violated the test's unmarked-parent premise.
  Those markers were preserved; the passing run used `/var/tmp` and allowed local
  sockets needed by integration tests.
- `go build -o /tmp/aide-reference-coverage ./cmd/aide` passed. This temporary
  binary does not replace the running daemon.
- Independent review: `reference-coverage-review.md`; no blockers.

## Subsequent rebuilt-daemon check and type-read correction

The next rebuilt daemon reported `0.1.18-dev.31+cc713fd`. A live limited caller
query confirmed the new qualified count and limit notice. An exact-file,
exact-line read of the Go `Reference` struct still failed with competing `type`
and `class` candidates for the same declaration.

The parser correction prefers a class/interface capture over a generic type
capture only for the identical name and exact syntax-node byte range. Distinct
same-name declarations survive, including those on one line. Struct/interface
kind filters now use their specialized classification; named primitive types
remain `type`. Existing index rows need file reindexing to adopt this normalization;
current-source reads reparse and use it immediately in a newly built process.

Parser and source-handler regressions reproduce the failure before the fix and
pass afterward. The complete code-parser and CLI suites pass (442 passing records
including subtests; 35 optional-grammar checks skipped because their grammars are
unavailable). A temporary binary build and isolated real stdio MCP batch read also
pass; the successful batch returns both source bodies with 321 UTF-8 bytes of
text. This is returned text, not a token-savings measurement. The first smoke
setup rejected a controller-created VCS marker before any handler call; a fresh
fixture followed the bridge's no-VCS contract.

See `type-capture-validation.json`, `type-capture-result.txt` and
`type-capture-review.md`. This remains operational verification outside the frozen
v3 trials. The running daemon was not replaced with the temporary test binary.
