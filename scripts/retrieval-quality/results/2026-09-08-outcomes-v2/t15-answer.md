Updated only `src/core/context-pruning/replacement.ts` to accept case-insensitive `aide_` names with valid 1–128-character ASCII suffixes, reusing existing MCP replacement behavior.

Verification passed: provided reproducer and independent checks for boundaries, invalid names, metadata preservation, immutability, mixed media, structured content, and legacy MCP/Bash behavior.
