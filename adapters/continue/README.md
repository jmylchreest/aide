# Continue.dev Adapter

Generates Continue.dev configuration from aide agents and skills.

## What is Continue?

[Continue](https://continue.dev/) is an open-source AI code assistant that integrates with VS Code and JetBrains IDEs. It supports custom slash commands.

## Usage

```bash
# Run the generator
npx ts-node adapters/continue/generate.ts
```

## Output

- `continue-config-snippet.json` - Contains `customCommands` array

## Integration

1. Open the generated `continue-config-snippet.json`
2. Copy the `customCommands` array
3. Merge into your `~/.continue/config.json`

```json
{
  "customCommands": [
    // ... paste here
  ]
}
```

## Available Commands

After integration, you can use:

- `/architect` - Deep analysis and strategic guidance
- `/executor` - Code implementation
- `/explore` - Codebase search
- `/planner` - Planning and requirements
- `/autopilot` - Full autonomous execution
- `/eco` - Token-efficient mode
- etc.

## Limitations

- Continue doesn't support dynamic skill injection like Claude Code hooks
- Model tier routing must be configured manually in Continue
- No shared memory (aide-memory) integration
