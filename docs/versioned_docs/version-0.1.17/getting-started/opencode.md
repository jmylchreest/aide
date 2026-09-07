---
sidebar_position: 3
---

# OpenCode

## From npm

```bash
bunx @jmylchreest/aide-plugin@latest install
```

This modifies your `opencode.json` to register the aide plugin and MCP server. Skills become available as `/aide:*` slash commands.

The install writes an **exact version pin** into both entries:

```json
{
  "plugin": ["@jmylchreest/aide-plugin@0.1.13"],
  "mcp": {
    "aide": {
      "type": "local",
      "command": ["bunx", "-y", "@jmylchreest/aide-plugin@0.1.13", "mcp"],
      "enabled": true
    }
  }
}
```

The pin is deliberate, and it is what makes upgrades work. OpenCode caches a
plugin install in a directory keyed by the spec string and reuses it if it
exists, so an unpinned entry is installed once and then never updated again —
however many releases go by. `bunx <name>` behaves the same way for the MCP
server. Changing the pin changes the cache key, which is what triggers a real
reinstall; it also keeps the plugin, the MCP server and the bundled Go binary
(shipped as a per-platform npm package) on one matching version.

## Upgrade

```bash
bunx @jmylchreest/aide-plugin@latest install
```

Re-running install with `@latest` fetches the newest package and re-pins both
entries to it. OpenCode installs the new version on its next start.

## Check Status

```bash
bunx @jmylchreest/aide-plugin@latest status
```

Reports the pinned version and flags a config that is unpinned or behind.

## Reinstall

```bash
bunx @jmylchreest/aide-plugin@latest uninstall && bunx @jmylchreest/aide-plugin@latest install
```

## How It Works

The OpenCode adapter integrates through:

- **System prompt transforms** for skill injection
- **Slash commands** for skill activation (`/aide:memorise`, `/aide:recall`, etc.)
- **Session-based tracking** for observational agent lifecycle
- **MCP server** for all 32 tools

See the [Platform Comparison](/docs/reference/platform-comparison) for detailed differences between Claude Code and OpenCode support.
