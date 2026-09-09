Updated only `src/core/context-pruning/replacement.ts`. Valid case-insensitive `aide_` names reuse the existing MCP replacement logic, with strict ASCII suffix validation and length limits. MCP and Bash behavior remains unchanged.

Verification: reproducer passed; independent checks passed for name boundaries, empty strings, metadata preservation, immutability, unsupported payloads, MCP, and Bash.
