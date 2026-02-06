# Aider Adapter

Generates Aider conventions from aide agents and skills.

## What is Aider?

[Aider](https://aider.chat/) is a command-line AI coding assistant. It uses conventions files to provide context to the AI.

## Usage

```bash
# Run the generator
npx ts-node adapters/aider/generate.ts
```

## Output

- `CONVENTIONS.md` - Full agent and skill documentation
- `.aider.conf.yml` - Aider configuration to read conventions

## Integration

1. Copy both files to your project root:

```bash
cp adapters/aider/CONVENTIONS.md /path/to/your/project/
cp adapters/aider/.aider.conf.yml /path/to/your/project/
```

2. Run aider normally - it will read CONVENTIONS.md automatically

```bash
aider
```

Or explicitly:

```bash
aider --read CONVENTIONS.md
```

## What's Included

**CONVENTIONS.md** contains:
- Full agent prompts (architect, executor, explore, etc.)
- Skill workflows (autopilot, eco, ralph, etc.)
- Best practices and guidelines

**.aider.conf.yml** contains:
- `read: CONVENTIONS.md` - Auto-loads conventions
- Suggested model and context settings

## Limitations

- Aider doesn't have hooks or dynamic skill injection
- No multi-agent orchestration (single conversation)
- aide-memory can be used separately via CLI
- Model routing is manual
