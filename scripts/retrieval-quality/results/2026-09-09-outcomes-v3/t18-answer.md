Updated only `src/core/context-pruning/replacement.ts` to accept valid case-insensitive `aide_` names through the existing MCP replacement logic. ASCII suffix limits are enforced, including rejecting trailing newlines. MCP and Bash behavior remain unchanged.

Verification: provided reproducer passed; additional boundary and compatibility checks passed—4 tests, 67 assertions. Temporary checks were removed.
