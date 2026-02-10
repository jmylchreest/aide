# Integrations & Adapters

aide supports multiple AI coding tools at different integration levels.

## Full Integrations

These provide first-class aide support with hooks, memory, skills, and persistence:

| Platform        | Package                    | Features                                                     |
| --------------- | -------------------------- | ------------------------------------------------------------ |
| **Claude Code** | Built-in (plugin)          | All features: hooks, skills, memory, HUD, persistence, swarm |
| **OpenCode**    | `@jmylchreest/aide-plugin` | Hooks, skills, memory, MCP tools. See [opencode/](opencode/) |

## Feature Comparison

| Feature                 |    Claude Code     |      OpenCode       |
| ----------------------- | :----------------: | :-----------------: |
| Dynamic skill injection |       Hooks        |       Plugin        |
| Memory injection        |    SessionStart    |  system.transform   |
| Tool tracking           |     PreToolUse     |   tool.execute.\*   |
| Session summaries       |     Stop hook      |     State-based     |
| Read-only enforcement   |     PreToolUse     |   permission.ask    |
| Persistence (ralph)     | Stop hook (block)  |    session.idle     |
| Comment checker         |    PostToolUse     | tool.execute.after  |
| Write guard             |     PreToolUse     | tool.execute.before |
| Todo continuation       |     Stop hook      |    session.idle     |
| Subagent orchestration  | SubagentStart/Stop |   Multi-instance    |
| HUD status line         |    Terminal HUD    |   File-based only   |
| aide-memory MCP         |       Direct       |     MCP config      |

## Writing a New Adapter

1. Create `adapters/<tool>/generate.ts`
2. Read agents from `src/agents/*.md`
3. Read skills from `src/skills/*.md`
4. Parse YAML frontmatter (see existing adapters)
5. Transform to tool's native format
6. Add README with integration instructions

## Full Integration Setup

### Claude Code

```bash
# Install from GitHub
claude plugins install github:jmylchreest/aide

# Or for development
claude --plugin-dir /path/to/aide
```

### OpenCode

See [opencode/README.md](opencode/README.md) for setup instructions.
