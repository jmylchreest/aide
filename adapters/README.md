# Integrations & Adapters

aide supports multiple AI coding tools at different integration levels.

## Full Integrations

These provide first-class aide support with hooks, memory, skills, and persistence:

| Platform        | Package                                       | Features                                                     |
| --------------- | --------------------------------------------- | ------------------------------------------------------------ |
| **Claude Code** | Built-in (plugin)                             | All features: hooks, skills, memory, HUD, persistence, swarm |
| **OpenCode**    | `@jmylchreest/aide-plugin`                    | Hooks, skills, memory, MCP tools. See [opencode/](opencode/) |
| **Codex CLI**   | Codex plugin + `@jmylchreest/aide-plugin --platform codex` | Hooks, skills, memory, MCP tools (no SubagentStart/Stop, no PreCompact) |

## Feature Comparison

| Feature                 |    Claude Code     |      OpenCode       |       Codex CLI       |
| ----------------------- | :----------------: | :-----------------: | :-------------------: |
| Dynamic skill injection |       Hooks        |       Plugin        |     UserPromptSubmit  |
| Memory injection        |    SessionStart    |  system.transform   |     SessionStart      |
| Tool tracking           |     PreToolUse     |   tool.execute.\*   |     PreToolUse        |
| Session summaries       |     Stop hook      |     State-based     |     Stop hook         |
| Read-only enforcement   |     PreToolUse     |   permission.ask    |     PreToolUse        |
| Persistence (autopilot) | Stop hook (block)  |    session.idle     |   Stop hook (block)   |
| Comment checker         |    PostToolUse     | tool.execute.after  |     PostToolUse       |
| Write guard             |     PreToolUse     | tool.execute.before |     PreToolUse        |
| Todo continuation       |     Stop hook      |    session.idle     |     Stop hook         |
| Subagent orchestration  | SubagentStart/Stop |   Multi-instance    |    Not available      |
| HUD status line         |    Terminal HUD    |   File-based only   |   File-based only     |
| Session end cleanup     |    SessionEnd      |  session.deleted    | Folded into Stop hook |
| Pre-compaction snapshot |    PreCompact      | session.compacting  |    Not available      |
| aide MCP tools (34)     |       Direct       |     MCP config      |     MCP config        |

## Full Integration Setup

### Claude Code

```bash
claude plugin marketplace add jmylchreest/aide
claude plugin install aide@aide
```

### OpenCode

```bash
bunx @jmylchreest/aide-plugin install
```

See [opencode/README.md](opencode/README.md) for detailed setup.

### Codex CLI

Recommended (Codex ≥ 0.144): install as a Codex plugin, then add hooks:

```bash
codex plugin marketplace add jmylchreest/aide
codex plugin add aide@aide
bunx @jmylchreest/aide-plugin install --platform codex   # hooks only
```

Codex consumes aide's Claude plugin manifest (`.claude-plugin/`) directly: the plugin provides the **MCP server** and **skills** (invoke with `$<name>` or the `/skills` picker — skills are never per-skill slash commands in Codex), and `codex plugin marketplace upgrade` keeps them current. The install step then only generates `~/.codex/hooks.json` and enables the `[features].hooks` flag — Codex removed support for plugin-shipped hooks (`plugin_hooks`), so lifecycle hooks still have to be registered directly. The installer detects a plugin-managed setup automatically and skips (and cleans up) the MCP entry and skill copies it would otherwise manage.

Standalone fallback (no plugin):

```bash
bunx @jmylchreest/aide-plugin install --platform codex
```

Without the plugin, this configures everything: MCP server in `~/.codex/config.toml`, lifecycle hooks in `~/.codex/hooks.json`, and skill copies in `~/.agents/skills/` (tracked in a manifest so re-installs update them and uninstall removes only aide's). Use `--project` for project-level config instead. Re-running the installer also repairs stale entries whose commands no longer resolve (e.g. after removing a global `aide-plugin` install).

**Codex limitations vs Claude Code:** No SubagentStart/Stop hooks (swarm mode limited), no PreCompact hook, no dedicated SessionEnd event (cleanup folded into Stop hook with autopilot mode guard).

## Writing a New Adapter

1. Create `adapters/<tool>/generate.ts`
2. Read agents from `src/agents/*.md`
3. Read skills from `src/skills/*.md`
4. Parse YAML frontmatter (see existing adapters)
5. Transform to tool's native format
6. Add README with integration instructions
