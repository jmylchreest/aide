Updated only `src/core/context-pruning/replacement.ts` to accept case-insensitive `aide_` names with the specified ASCII suffix rules and reuse existing MCP replacement behavior.

Verification passed: visible reproducer plus independent checks covering name boundaries, empty strings, metadata preservation, immutability, unsupported payloads, and existing MCP/Bash behavior.
