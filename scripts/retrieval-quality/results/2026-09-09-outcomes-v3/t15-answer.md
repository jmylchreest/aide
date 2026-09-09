Updated `src/core/context-pruning/replacement.ts` to recognize valid case-insensitive `aide_` names and reuse the existing MCP replacement logic. Existing MCP and Bash behavior remains intact.

Verification: required reproducer passed; independent boundary, metadata, immutability, and rejection checks passed (3 tests, 36 assertions total). Scratch tests removed.
