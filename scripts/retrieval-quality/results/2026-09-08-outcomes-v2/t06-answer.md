Updated only `src/core/context-pruning/replacement.ts` to accept valid case-insensitive `aide_` names through the existing MCP replacement logic, preserving MCP and Bash behavior.

Verification: the provided reproducer passed, plus 47 independent assertions covering name boundaries, payload validation, metadata preservation, immutability, and legacy behavior.
