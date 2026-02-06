# Adapters (Examples)

This directory contains **example adapters** that demonstrate how aide's portable content (agents, skills) can be translated to other AI coding tools.

## Status: Examples Only

These adapters are provided as **starting points**, not production-ready integrations:

| Adapter | Status | Completeness |
|---------|--------|--------------|
| `continue/` | Example | Generates slash commands from agents/skills |
| `cursor/` | Example | Generates .cursorrules with condensed prompts |
| `aider/` | Example | Generates CONVENTIONS.md |

## What Works

- Reading aide agent/skill markdown files
- Generating basic tool-specific config formats
- Providing a template for community contributions

## What's Missing (vs Claude Code)

| Feature | Claude Code | Adapters |
|---------|:-----------:|:--------:|
| Dynamic skill injection | ✅ Hooks | ❌ Static |
| Keyword detection | ✅ Hooks | ❌ Manual |
| Model tier routing | ✅ Automatic | ❌ Manual |
| Read-only enforcement | ✅ PreToolUse | ❌ None |
| Persistence (ralph) | ✅ Stop hook | ❌ None |
| aide-memory integration | ✅ Direct | ⚠️ CLI only |

## Contributing

Community contributions to improve these adapters are welcome. Each tool has different capabilities, so adapters will vary in completeness.

## Writing a New Adapter

1. Create `adapters/<tool>/generate.ts`
2. Read agents from `src/agents/*.md`
3. Read skills from `src/skills/*.md`
4. Parse YAML frontmatter (see existing adapters)
5. Transform to tool's native format
6. Add README with integration instructions

## Full Integration (Claude Code)

For the complete aide experience with all features, use Claude Code:

```bash
# Install from GitHub
claude plugins install github:jmylchreest/aide

# Or for development
claude --plugin-dir /path/to/aide
```
