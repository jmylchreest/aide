Updated only `src/core/context-pruning/replacement.ts` to accept case-insensitive `aide_` names with the specified ASCII suffix rules and reuse existing MCP replacement behavior.

Verification passed: visible reproducer and independent checks covering length boundaries, malformed names, metadata preservation, immutability, mixed media, structured content, MCP, and Bash behavior.
