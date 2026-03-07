---
sidebar_position: 5
---

# Configuration

AIDE is configured through environment variables. All variables are optional.

## Environment Variables

| Variable                    | Description                                    |
| --------------------------- | ---------------------------------------------- |
| `AIDE_DEBUG=1`              | Enable debug logging (logs to `.aide/_logs/`)  |
| `AIDE_FORCE_INIT=1`         | Force initialization in non-git directories    |
| `AIDE_CODE_WATCH=1`         | Enable file watching for auto-reindex          |
| `AIDE_CODE_WATCH_DELAY=30s` | Delay before re-indexing after file changes    |
| `AIDE_MEMORY_INJECT=0`      | Disable memory injection                       |
| `AIDE_SHARE_AUTO_IMPORT=1`  | Auto-import shared decisions/memories on start |

## Project Configuration

Project-level settings can be stored in `.aide/config/aide.json`:

```json
{
  "findings": {
    "complexity": {
      "threshold": 10
    },
    "coupling": {
      "fanOut": 15,
      "fanIn": 20
    },
    "clones": {
      "windowSize": 50,
      "minLines": 6
    }
  }
}
```

| Setting                         | Default | Description                                  |
| ------------------------------- | ------- | -------------------------------------------- |
| `findings.complexity.threshold` | 10      | Cyclomatic complexity threshold per function |
| `findings.coupling.fanOut`      | 15      | Maximum outgoing imports before flagging     |
| `findings.coupling.fanIn`       | 20      | Maximum incoming imports before flagging     |
| `findings.clones.windowSize`    | 50      | Sliding window size in tokens for detection  |
| `findings.clones.minLines`      | 6       | Minimum clone size in lines to report        |

Values in `aide.json` serve as project-level defaults. CLI flags override config file values. If neither is set, the built-in defaults apply.

## File Exclusions

Create a `.aideignore` file in your project root to exclude files from indexing and analysis. Uses gitignore syntax:

```gitignore
# Exclude generated files
*.generated.ts
*.min.js

# Exclude vendor
vendor/

# But include important vendor files
!vendor/important.go
```

Built-in defaults already exclude common generated files, lock files, build artifacts, and directories like `node_modules/`, `.git/`, `vendor/`, etc.

## Troubleshooting

```bash
aide version                              # Check binary
aide status                               # Full system dashboard
AIDE_DEBUG=1 claude                       # Debug logging (or AIDE_DEBUG=1 opencode)
```
