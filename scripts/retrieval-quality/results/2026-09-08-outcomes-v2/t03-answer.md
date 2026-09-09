Updated only `src/core/context-pruning/replacement.ts`. Valid case-insensitive `aide_` names now reuse the existing MCP replacement logic, enforcing the ASCII suffix rules and preserving legacy Bash behavior.

Verification passed: the visible reproducer and independent checks for name boundaries, payload rejection, metadata preservation, immutability, MCP names, and Bash.
