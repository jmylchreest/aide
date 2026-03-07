---
sidebar_position: 3
---

# OpenCode

## From npm

```bash
bunx @jmylchreest/aide-plugin install
```

This modifies your `opencode.json` to register the aide plugin and MCP server. Skills become available as `/aide:*` slash commands.

## Check Status

```bash
bunx @jmylchreest/aide-plugin status
```

## Reinstall

```bash
bunx @jmylchreest/aide-plugin uninstall && bunx @jmylchreest/aide-plugin install
```

## How It Works

The OpenCode adapter integrates through:

- **System prompt transforms** for skill injection
- **Slash commands** for skill activation (`/aide:memorise`, `/aide:recall`, etc.)
- **Session-based tracking** for observational agent lifecycle
- **MCP server** for all 25 tools

See the [Platform Comparison](/docs/reference/platform-comparison) for detailed differences between Claude Code and OpenCode support.
