Extend src/core/context-pruning/replacement.ts so OpenCode aide tool names use the same pure-text replacement behavior as existing MCP tools.

Accept names case-insensitively when they start with aide_ and have a suffix of 1 to 128 ASCII characters: the first suffix character must be a letter, and remaining characters must be letters, digits or underscores. Reject bare aide_, malformed suffixes and unrelated native tool names. Preserve all existing mcp__ behavior and legacy Bash behavior.

The new names must support plain strings (including empty strings) and objects with nonempty content arrays of pure text blocks. Preserve envelope and block metadata, reject mixed media or defined structuredContent, and do not mutate the original payload. These are the same replacement rules already implemented for MCP names; avoid duplicating that behavior.

Only edit src/core/context-pruning/replacement.ts. Run the visible reproducer with rtk proxy bun test ./reproduce.test.ts. You may add independent checks in new *.test.ts or *.spec.ts scratch files outside src, but do not modify the provided reproducer. Finish with your change and verification.
