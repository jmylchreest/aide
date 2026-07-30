---
sidebar_position: 2
---

# Claude Code

## From Marketplace (Recommended)

```bash
claude plugin marketplace add jmylchreest/aide
claude plugin install aide@aide
```

## Register for Team Auto-Install

Add to `~/.claude/settings.json` (or `.claude/settings.json` for project-level) so team members are prompted to install:

```json
{
  "extraKnownMarketplaces": {
    "aide": {
      "source": {
        "source": "github",
        "repo": "jmylchreest/aide"
      }
    }
  }
}
```

## Permissions

Add to `~/.claude/settings.json`:

```json
{
  "permissions": {
    "allow": [
      "Bash(aide *)",
      "Bash(**/aide *)",
      "Bash(git worktree *)",
      "mcp__plugin_aide_aide__*"
    ]
  }
}
```

## Reinstall

```bash
claude plugin uninstall aide && claude plugin install aide@aide
```
