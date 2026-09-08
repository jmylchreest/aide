---
title: Model usage accounting
---

# Model usage accounting

Aide reports captured model usage alongside text estimates and recorded aide work. These answer different questions: what the host reported consuming, what text aide prepared or observed, and what operations aide performed. Task correctness still requires its own verification evidence. None of these measurements alone proves savings.

## Where to see it

Run `aide token stats --session=<full-session-id> --details` or add `--json` to inspect `accounting.model_usage`. In aide-web, open **Telemetry → Tokens → Accounting**. Overview has a compact model-usage link when evidence exists. Date/session filters apply to the same report in every view. The existing activity chart continues to show estimated result text; it is not a chart of model usage.

The version 1 report contains `observations`, `conflicts`, `invalid`, and `by_source`. Each source group identifies its host, adapter source, optional model/provider, timing coverage and counters. Each available counter contains `tokens` and `observations`. For example, 1,234 input tokens over 1 of 2 recorded observations means the other observation has unknown input. It does not mean 50% of all model calls were captured. Absent fields mean unknown; reported zero remains zero. Some hosts substitute zero for missing provider data.

An older server without `model_usage` reports unavailable data. A supported report with no observations means no usage was recorded for the selection, not zero consumption. Model/provider remain unknown when the source does not establish them. There is no cross-source grand total or inferred price.

## Collection by host

| Host | Captured source | Included counters | Limits |
| --- | --- | --- | --- |
| Codex | `token_usage_record` rows from the explicit Stop-hook transcript path | Per-response input, cache read/write, output, reasoning and reported total, when supplied | Versioned adapter for an observed runtime record shape; unsupported runtimes remain unavailable. Cumulative `thread_token_usage` and `token_count` snapshots are never summed. |
| Claude Code | Assistant usage from the explicit Stop-hook transcript path | Uncached input and cache read/write; combined input only when all components are present | Assistant output can be a response-start placeholder, so output is omitted. Transcripts can lag the latest turn. |
| OpenCode | Observed `message.part.updated` events containing `step-finish` | Input/cache components, reasoning and separately labelled reported output | Message-level usage can omit earlier steps. Output/reasoning overlap varies by host version, so combined output stays unknown. |

Transcript collection reads only the explicitly supplied regular file, at most the latest 4 MiB and 1,000 usage records per Stop. Partial lines are not imported. Later Stops can retry delayed records or failed writes. There is no transcript-directory discovery or historical backfill. A long turn, unsupported source or unseen subagent can leave gaps. The collector extracts counter and identity metadata; it does not persist prompts, responses, tool arguments or reasoning text. Collection is bounded but adds local file parsing and one batch write when records exist; it does not call a model.

## Counter relationships

`input_tokens` includes cache when the source supports normalization. Claude/OpenCode input is assembled only when uncached input, cache read and cache write are all available. For Codex, input already includes cache. `output_tokens` includes reasoning only where that relationship is established. OpenCode's raw `reported_output_tokens` is separate because its relationship to reasoning depends on the host version. `total_tokens` is a supplied per-response total, not a sum fabricated from partial fields.

Cache rows overlap normalized input, and reasoning can overlap output. Do not add all rows. Counters are host-reported observations, not byte estimates, authoritative billing records or task-quality scores.

These boundaries follow [OpenAI cache counter semantics](https://developers.openai.com/api/docs/guides/prompt-caching), [Anthropic cache accounting](https://platform.claude.com/docs/en/build-with-claude/prompt-caching#tracking-cache-performance), and [Claude SDK per-step usage limits](https://code.claude.com/docs/en/agent-sdk/cost-tracking#track-per-step-usage). The SDK documentation informs the conservative Claude adapter; it does not guarantee an on-disk transcript schema. The [Claude hook contract](https://code.claude.com/docs/en/hooks#common-input-fields) describes asynchronous transcript writes. OpenCode's [step emission](https://github.com/anomalyco/opencode/blob/v1.2.27/packages/opencode/src/session/processor.ts) and [v1.2.27 normalization](https://github.com/anomalyco/opencode/blob/v1.2.27/packages/opencode/src/session/index.ts) differ from [current normalization](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/session/session.ts) in output/reasoning treatment.

## Repeated and conflicting evidence

The identity is host + session + response/message/step ID. Equal observations count once, including after restarts. Claude tool fanout can repeat one message ID. Different usage for the same identity remains conflicting evidence and is excluded rather than selecting the largest or latest report. Invalid supplied counters also prevent that identity from contributing. Deduplication and conflict checks use retained project evidence before time filtering; narrowing a date window cannot hide a known contradiction.

Time selection uses the earliest retained source-record timestamp when supplied, otherwise the earliest observation timestamp. This is not model execution duration. Retention limits the evidence available for deduplication and conflict detection. Usage records remain separate from tool event counts, text estimates and recorded MCP operations.

## Exercise it

After rebuilding/restarting aide and loading the updated plugin, complete a short normal turn in the host. Claude/Codex require a Stop hook carrying the supported transcript path; OpenCode requires an observed step-finish event. Inspect the full session ID with `token stats --details --session=<id>`. Repeat the inspection: reading the report must not increase usage. Subsequent turns add their own records while repeated source records remain deduplicated.

Use `bun run test --run src/test/model-usage.test.ts` for the synthetic adapter checks. The wider test suites verify store deduplication/conflicts, session/date selection, transport and rendering. These checks require no provider calls and establish accounting behavior, not an improvement in task quality or a causal token reduction.

If prepared context appears twice, inspect hook registration as well as accounting identities. User and project Codex configurations can both register the same aide hooks. That can prepare and inject context twice even though stable model-response IDs still deduplicate correctly. The [development toggle](../getting-started/codex.md#local-development-builds) assigns one dev hook owner and reports overlapping registrations. Correct the configuration; do not erase repeated context observations to manufacture savings. Fewer duplicate hook outputs do not by themselves quantify provider-token or billing changes.
