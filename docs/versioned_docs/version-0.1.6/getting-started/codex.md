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
