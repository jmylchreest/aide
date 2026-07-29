---
sidebar_position: 4
---

# Codex CLI

## Recommended: Codex Plugin

Requires Codex ≥ 0.144.

```bash
codex plugin marketplace add jmylchreest/aide
codex plugin add aide@aide
bunx @jmylchreest/aide-plugin install --platform codex   # hooks only
```

Codex consumes aide's Claude plugin manifest directly. The plugin provides:

- **MCP server** — all aide tools, registered from the plugin manifest
- **Skills** — discovered from the plugin, namespaced `aide:<name>`

The final step generates `~/.codex/hooks.json` and enables the `[features].hooks` flag. Codex does not support plugin-shipped hooks, so lifecycle hooks (context injection, skill matching, tool tracking, persistence) must be registered directly. The installer detects a plugin-managed setup and manages only hooks — it also cleans up any redundant MCP entry or skill copies left by a previous standalone install.

Update the plugin with:

```bash
codex plugin marketplace upgrade
```

## Invoking Skills

Codex never creates per-skill slash commands (`/aide:test` will not work). Instead:

- Type `$` in the composer to mention a skill explicitly (e.g. `$assess-findings`)
- Type `/skills` to open the skill picker
- Describe the task in plain language — Codex selects a matching skill implicitly, and aide's `skill-injector` hook independently injects matching skill content

## Standalone Install (no plugin)

For Codex versions without plugin support:

```bash
bunx @jmylchreest/aide-plugin install --platform codex
```

This configures everything directly: the MCP server in `~/.codex/config.toml`, lifecycle hooks in `~/.codex/hooks.json`, and skill copies in `~/.agents/skills/`. Skill copies are tracked in a manifest so re-installs update them and uninstall removes only aide's. Use `--project` for project-level config.

Re-running the installer also repairs stale entries whose commands no longer resolve (for example, after removing a global `aide-plugin` install).

## Check Status

```bash
bunx @jmylchreest/aide-plugin status --platform codex
```

## Sandboxed Shells and the aide Daemon

aide's daemon is the MCP server process itself: the first `aide mcp` in a project owns the stores and listens on `.aide/aide.sock`; every other aide process (CLI commands, hooks, later MCP servers) attaches to it over that socket.

Codex spawns MCP servers **outside** its sandbox, so the aide MCP tools always work. But shell commands and hooks run **inside** the sandbox, which denies socket `connect()` (`EPERM`) under the default `workspace-write` policy. The result: the MCP side of aide is fully live while any `aide` CLI invocation in the same session cannot reach it —

- `aide status` reports `Server: unreachable — socket present but this shell's sandbox denies connect()`
- CLI commands that need the daemon (and hooks such as tool-call observation) fail fast with an error explaining the sandbox denial, instead of stalling on the daemon's store locks until the hook budget expires

To let sandboxed commands reach the daemon, enable network access for the workspace-write sandbox in `~/.codex/config.toml`:

```toml
sandbox_mode = "workspace-write"

[sandbox_workspace_write]
network_access = true
```

Or as a one-off:

```bash
codex -c sandbox_mode="workspace-write" -c sandbox_workspace_write.network_access=true
```

Note this lifts the sandbox's *entire* network restriction for shell commands (outbound internet included, and `CODEX_SANDBOX_NETWORK_DISABLED` is no longer set) — Codex offers no narrower "unix sockets only" carve-out. If you prefer to keep the network sealed, aide degrades gracefully: MCP tools keep working, and only sandboxed CLI/hook invocations are skipped.

## Uninstall

```bash
bunx @jmylchreest/aide-plugin uninstall --platform codex
codex plugin remove aide@aide
```

## Limitations vs Claude Code

- No `SubagentStart`/`SubagentStop` hooks — swarm mode is limited
- No `PreCompact` hook
- No dedicated `SessionEnd` event — cleanup is folded into the `Stop` hook
- HUD is file-based only (no native status line)
- Sandboxed shell commands and hooks cannot reach the aide daemon unless `network_access = true` is set (see [Sandboxed Shells and the aide Daemon](#sandboxed-shells-and-the-aide-daemon))
