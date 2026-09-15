# Grading interpretation frozen before v5

Use the unchanged v4 prompts, check IDs, criteria, reference answers and implementation tests. The rubric instruction “Do not require facts not requested in the prompt” constrains its required-fact lists.

- Trace: omission of the writer catch-path debug message does not independently fail `writer_and_cache` when failure return, acknowledgment-dependent caching and retry behavior are correct. Explicitly false logging claims still count against correctness.
- Trace: describing JSONL input without explicitly restating the final newline does not independently fail the writer criterion; the prompt asks what is spawned, rather than byte-level wire-preservation checks. Incorrect payload claims still count.
- Impact: the prompt explicitly requests preserving JSONL payload, logging and proposing wire checks; those remain required.
- Impact: enumerating the text skill handler separately is not required when the requested affected-consumer inventory, nonconsumer categories and shared-type exclusion principle are correct. Claiming that handler is affected without evidence still fails.

These interpretations were recorded during the v4 blinded adjudication and are now declared before v5 answers exist. All other rubric facts and mandatory runtime/source-review criteria remain unchanged. Grade without treatment, usage, timing or retrieval-trace information. Preserve misses, false positives, unsupported claims and unknown evidence; do not adjust requirements after outcomes.
