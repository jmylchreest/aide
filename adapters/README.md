# Integrations & Adapters

aide supports multiple AI coding tools at different integration levels.

## Full Integrations

These provide first-class aide support with hooks, memory, skills, and persistence:

| Platform        | Package                                       | Features                                                     |
| --------------- | --------------------------------------------- | ------------------------------------------------------------ |
| **Claude Code** | Built-in (plugin)                             | All features: hooks, skills, memory, HUD, persistence, swarm |
| **OpenCode**    | `@jmylchreest/aide-plugin`                    | Hooks, skills, memory, MCP tools. See [opencode/](opencode/) |
| **Codex CLI**   | `@jmylchreest/aide-plugin --platform codex`   | Hooks, skills, memory, MCP tools (no SubagentStart/Stop, no PreCompact) |

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

```bash
bunx @jmylchreest/aide-plugin install --platform codex
```

This generates `~/.codex/config.toml` (MCP server) and `~/.codex/hooks.json` (lifecycle hooks), and enables the `codex_hooks` feature flag. Use `--project` for project-level config instead.

**Codex limitations vs Claude Code:** No SubagentStart/Stop hooks (swarm mode limited), no PreCompact hook, no dedicated SessionEnd event (cleanup folded into Stop hook with autopilot mode guard).

## Writing a New Adapter

1. Create `adapters/<tool>/generate.ts`
2. Read agents from `src/agents/*.md`
3. Read skills from `src/skills/*.md`
4. Parse YAML frontmatter (see existing adapters)
5. Transform to tool's native format
6. Add README with integration instructions
