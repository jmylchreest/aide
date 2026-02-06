# Cursor Adapter

Generates Cursor rules from aide agents and skills.

## What is Cursor?

[Cursor](https://cursor.sh/) is an AI-first code editor built on VS Code. It uses `.cursorrules` files to customize AI behavior.

## Usage

```bash
# Run the generator
npx ts-node adapters/cursor/generate.ts
```

## Output

- `.cursorrules` - Cursor AI behavior rules

## Integration

1. Copy the generated `.cursorrules` to your project root
2. Cursor automatically picks up the rules

```bash
cp adapters/cursor/.cursorrules /path/to/your/project/
```

## What's Included

The generated rules contain:

- **Agent Personas** - Condensed versions of aide agents as behavioral guidance
- **Skill Triggers** - Keywords that activate specific behaviors
- **General Guidelines** - Model tiers, verification, minimal changes

## Limitations

- Cursor rules are static (no dynamic hooks)
- No explicit multi-agent orchestration
- No shared memory integration
- Single rules file vs aide's modular agents/skills
