# Integrations & Adapters

aide supports multiple AI coding tools at different integration levels.

## Full Integrations

These provide first-class aide support with hooks, memory, skills, and persistence:

| Platform        | Package                    | Features                                                     |
| --------------- | -------------------------- | ------------------------------------------------------------ |
| **Claude Code** | Built-in (plugin)          | All features: hooks, skills, memory, HUD, persistence, swarm |
| **OpenCode**    | `@jmylchreest/aide-plugin` | Hooks, skills, memory, MCP tools. See [opencode/](opencode/) |

## Example Adapters

These are **starting points** that translate aide's portable content to other tools:

| Adapter     | Status  | Completeness                                  |
| ----------- | ------- | --------------------------------------------- |
| `continue/` | Example | Generates slash commands from agents/skills   |
| `cursor/`   | Example | Generates .cursorrules with condensed prompts |
| `aider/`    | Example | Generates CONVENTIONS.md                      |

### What Works (Adapters)

- Reading aide agent/skill markdown files
- Generating basic tool-specific config formats
- Providing a template for community contributions

### What's Missing (Adapters vs Full Integrations)

| Feature                 |      Claude Code      |      OpenCode       |  Adapters   |
| ----------------------- | :-------------------: | :-----------------: | :---------: |
| Dynamic skill injection |       ✅ Hooks        |      ✅ Plugin      |  ❌ Static  |
| Memory injection        |    ✅ SessionStart    | ✅ system.transform |   ❌ None   |
| Tool tracking           |     ✅ PreToolUse     | ✅ tool.execute.\*  |   ❌ None   |
| Session summaries       |     ✅ Stop hook      |   ⚠️ State-based    |   ❌ None   |
| Read-only enforcement   |     ✅ PreToolUse     |  ⚠️ permission.ask  |   ❌ None   |
| Persistence (ralph)     | ✅ Stop hook (block)  | ❌ No stop blocking |   ❌ None   |
| Subagent orchestration  | ✅ SubagentStart/Stop |  ⚠️ Multi-instance  |   ❌ None   |
| HUD status line         |    ✅ Terminal HUD    | ❌ File-based only  |   ❌ None   |
| Usage tracking          |     ✅ OAuth API      |  ❌ Not available   |   ❌ None   |
| aide-memory MCP         |       ✅ Direct       |    ✅ MCP config    | ⚠️ CLI only |

## Contributing

Community contributions to improve these adapters are welcome. Each tool has different capabilities, so adapters will vary in completeness.

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
