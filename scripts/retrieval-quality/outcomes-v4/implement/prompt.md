Implement the following API evolution in this frozen aide source workspace.

The usage batch writer currently exposes a positional boolean API. Replace it
with an explicit request and discriminated result, and migrate its consumers.
Export these types from the existing shared module:

```ts
interface UsageWriteRequest {
  binary: string;
  cwd: string;
  events: ObserveBatchEvent[];
}
type UsageWriteResult =
  | { status: "acknowledged"; recorded: number }
  | { status: "unacknowledged"; reason: "invalid-ack" | "write-error" };
```

- `recordModelUsage(request: UsageWriteRequest): UsageWriteResult` takes one
  object. An empty batch returns `{status:"acknowledged",recorded:0}` without
  invoking the binary. An exact full-batch acknowledgment returns the submitted
  count. Missing/malformed output, a mismatched count or any positive skipped
  count returns `{status:"unacknowledged",reason:"invalid-ack"}`. A thrown write
  or serialization error returns `{status:"unacknowledged",reason:"write-error"}`.
  Preserve the existing case-insensitive acknowledgment syntax, surrounding
  whitespace tolerance, explicit `skipped 0`, JSONL payload, 10-second timeout,
  cwd and stdio behavior. Keep the write best-effort and do not add retries here.
- `createOpenCodeUsageRecorder` continues to accept an optional injected writer,
  now with the request/result contract above. Its returned recorder takes one
  object `{binary:string,cwd:string,part:unknown}` and returns void. Forward the
  normalized single-event batch through the new writer API. Cache a fingerprint
  only after `status === "acknowledged"`; an unacknowledged result object must
  remain retryable. Preserve invalid-part filtering, changed-counter forwarding,
  cwd scoping, independent recorder instances and oldest-first 1,024-entry cap.
- Migrate the OpenCode event adapter to the recorder's new request shape.
  Migrate the Stop/session-summary adapter to the writer's new request shape and
  append `; write=${result.status}` to its existing `Usage scan:` debug message.
  A failed usage write must still allow summary capture and `{continue:true}`.
  Preserve recursion suppression, unknown/missing-binary handling and the
  behavior of other hook event types.
- Preserve all normalization and collection semantics, source timestamps,
  partial-coverage meaning and Go durable deduplication/accounting behavior.
  This exercise does not add host usage sources or change data collection.
- Migrate the affected original Vitest test consumers and mocks to the new
  request/result API. Keep their existing behavioral coverage. These files
  remain available for source review even though the offline Bun runner does
  not execute Vitest. Report that validation limit accurately.

You may edit TypeScript under `src/`, including original tests; do not change
the experiment harness, visible tests, package/config,
README or Go source. The original Vitest tests are navigation evidence and are
not the offline test runner. Run `bun test tests` for visible regression checks.
You may add your own scratch tests under `tests/`, but the existing visible test
files must remain unchanged. Added tests do not count toward the grader's
mandatory visible-check total.
Additional hidden tests check the requested contract and adapter behavior.
The Bun runner does not type-check TypeScript; distinguish runtime checks from
type or Vitest validation in your report. No dependency installation or network
access is needed. Finish with a concise
description of the changes and checks performed.
