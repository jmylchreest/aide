Updated only `src/core/context-pruning/replacement.ts` to accept case-insensitive `aide_` names with valid 1–128-character ASCII suffixes, reusing the existing MCP replacement logic.

Verification passed: the provided reproducer and independent checks for name boundaries, metadata preservation, immutability, invalid payloads, and existing MCP/Bash behavior.
