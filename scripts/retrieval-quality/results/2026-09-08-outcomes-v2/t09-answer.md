Updated only `src/core/context-pruning/replacement.ts` to recognize valid case-insensitive `aide_` names and reuse existing MCP replacement behavior. Invalid suffixes, including trailing line terminators, are rejected; MCP and Bash behavior remains unchanged.

Verification: provided reproducer passed. Five additional scratch tests passed, covering boundaries, metadata, immutability, unsupported payloads, and legacy behavior. Scratch file removed.
