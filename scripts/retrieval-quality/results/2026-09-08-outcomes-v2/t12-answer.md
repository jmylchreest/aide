Updated only `src/core/context-pruning/replacement.ts`. Valid case-insensitive `aide_` names now reuse MCP replacement handling, with strict ASCII suffix validation and 1–128 character bounds. Existing MCP and Bash behavior is preserved.

Verification: provided reproducer passed; three independent scratch tests passed with 40 assertions covering naming boundaries, payload rules, metadata preservation, immutability, and legacy behavior. Scratch tests were removed.
