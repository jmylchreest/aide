Updated `src/core/context-pruning/replacement.ts` to support Bash adapter output with validated metadata and safe integer exit statuses. Replacements preserve the envelope without mutation and reject conflicting representations or images.

Verification: the visible reproducer passed, plus 94 edge-case and regression assertions covering adapter validation, legacy Bash selection, and MCP behavior.
