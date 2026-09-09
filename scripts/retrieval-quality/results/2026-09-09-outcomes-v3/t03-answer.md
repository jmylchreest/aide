Updated `src/core/context-pruning/replacement.ts` to recognize case-insensitive `aide_` names with valid 1–128-character ASCII suffixes and reuse existing MCP replacement behavior. MCP and Bash handling remain unchanged.

Verification passed:
- Supplied reproducer: 1 test, 3 assertions.
- Independent checks for name boundaries, metadata, immutability, mixed media, structured content, MCP, and Bash.
