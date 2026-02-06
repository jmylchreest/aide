# Memory Injection System

## Overview

AIDE injects memories into session context using three tiers:
1. **Static** - Always injected (global preferences, project decisions)
2. **Dynamic** - Recent session memories (configurable count)
3. **Relevant** - Search-based injection from prompt keywords

## Memory Categories & Namespaces

### Categories

| Category | Injection | Lifecycle | Description |
|----------|-----------|-----------|-------------|
| `global` | Static (always) | Permanent | User preferences, conventions |
| `decision` | Static (always) | Permanent | Architectural decisions for project |
| `session` | Dynamic (recent N) | 30 days | Session summaries |
| `learning` | Relevant (search) | 90 days | Discoveries, insights |
| `pattern` | Relevant (search) | Permanent | Reusable approaches |
| `gotcha` | Relevant (search) | 90 days | Pitfalls to avoid |

### Namespaces (via tags)

Memories are scoped using tag prefixes:

```
project:<name>    - Project-specific (e.g., project:aide)
scope:global      - Cross-project (user preferences)
scope:session     - Current session only (ephemeral)
agent:<id>        - Created by specific agent
mode:<name>       - Related to a mode (autopilot, swarm)
```

### Storage Examples

```bash
# Global preference (always injected)
aide memory add --category=global --tags="scope:global" \
  "Prefers TypeScript over JavaScript"

# Project decision (always injected for this project)
aide memory add --category=decision --tags="project:aide,architecture" \
  "Use BBolt for local storage, PostgreSQL for distributed"

# Session summary (dynamic injection)
aide memory add --category=session --tags="project:aide,session:abc123" \
  "Implemented JWT auth with refresh tokens"

# Learning (search-based injection)
aide memory add --category=learning --tags="project:aide,hooks,stderr" \
  "Hooks must not write to stderr - causes Claude Code errors"
```

## Injection Strategy

### On Session Start

```
┌─────────────────────────────────────────────────────────────┐
│                    Memory Injection                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. STATIC (always inject)                                   │
│     ├─ category:global, scope:global                         │
│     └─ category:decision, project:<current>                  │
│                                                              │
│  2. DYNAMIC (recent N, configurable)                         │
│     └─ category:session, project:<current>                   │
│         ORDER BY created_at DESC LIMIT N                     │
│                                                              │
│  3. RELEVANT (search from prompt - on UserPromptSubmit)      │
│     └─ category:learning|pattern|gotcha                      │
│         SEARCH keywords extracted from prompt                │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Dynamic Count Configuration

Default: **3 recent sessions**

Adjustable via:
1. **Config file** (`.aide/config/aide.json`):
   ```json
   {
     "memory": {
       "dynamicCount": 3,
       "searchLimit": 5
     }
   }
   ```

2. **Keyword detection** (per-session override):
   ```
   "detailed context" → dynamicCount: 5
   "full history"     → dynamicCount: 10
   "quick task"       → dynamicCount: 1
   "eco mode"         → dynamicCount: 1
   ```

3. **Mode-based defaults**:
   | Mode | Dynamic Count | Search Limit |
   |------|---------------|--------------|
   | autopilot | 5 | 5 |
   | ralph | 5 | 5 |
   | swarm | 3 | 3 |
   | eco | 1 | 2 |
   | (default) | 3 | 3 |

### State Key for Dynamic Override

```bash
# Set via keyword detection hook
aide state set memoryDynamicCount 5

# Read in session-start
dynamicCount = aide state get memoryDynamicCount || config.default
```

## Search Term Extraction

On UserPromptSubmit, extract keywords for relevant memory search:

### Extraction Strategy

```typescript
function extractSearchTerms(prompt: string, cwd: string): string[] {
  const terms: string[] = [];

  // 1. Project name (always include)
  terms.push(getProjectName(cwd));

  // 2. File paths mentioned
  const filePaths = prompt.match(/[\w\/]+\.(ts|js|go|py|rs|md)/g) || [];
  terms.push(...filePaths.map(f => f.split('/').pop()?.replace(/\.\w+$/, '')));

  // 3. Technical terms (CamelCase, snake_case)
  const techTerms = prompt.match(/[A-Z][a-z]+(?:[A-Z][a-z]+)+|[a-z]+_[a-z_]+/g) || [];
  terms.push(...techTerms);

  // 4. Common keywords
  const keywords = ['auth', 'api', 'database', 'test', 'build', 'deploy',
                    'error', 'bug', 'fix', 'refactor', 'hook', 'config'];
  for (const kw of keywords) {
    if (prompt.toLowerCase().includes(kw)) {
      terms.push(kw);
    }
  }

  // 5. Quoted strings (explicit intent)
  const quoted = prompt.match(/"([^"]+)"|'([^']+)'/g) || [];
  terms.push(...quoted.map(q => q.replace(/['"]/g, '')));

  // Dedupe and limit
  return [...new Set(terms)].slice(0, 10);
}
```

### Search Query Construction

```bash
# Combine terms with OR for fuzzy matching
aide memory search --category=learning,pattern,gotcha \
  --limit=5 \
  "auth OR jwt OR token OR session"
```

## Injection Format

```xml
<aide-context>

## Preferences (Static)
- Prefers TypeScript over JavaScript
- Uses pnpm as package manager
- Avoids classes, prefers functional style

## Project Decisions
- **Storage**: BBolt for local, PostgreSQL for distributed
- **Testing**: Vitest for unit tests, Playwright for E2E

## Recent Sessions (Dynamic)
### 2 hours ago - Implemented JWT Auth
Added token-based auth with refresh tokens. Files: src/auth/*

### Yesterday - Fixed Hook Errors
Resolved stderr issue in hooks. Files: src/hooks/session-start.ts

## Relevant Context (Search)
### Learning: Hooks Must Not Write to stderr
Claude Code interprets stderr as error. Debug to files only.
[Relevance: 85%]

### Gotcha: ESM Requires .js Extensions
Import with .js even for .ts files when using ESM.
[Relevance: 72%]

</aide-context>
```

## Lifecycle Management

### Retention Policies

| Category | Default TTL | Auto-cleanup |
|----------|-------------|--------------|
| global | ∞ (permanent) | Never |
| decision | ∞ (permanent) | Never |
| session | 30 days | Daily cleanup |
| learning | 90 days | Weekly cleanup |
| pattern | ∞ (permanent) | Never |
| gotcha | 90 days | Weekly cleanup |

### Cleanup Command

```bash
# Run by session-start hook or cron
aide memory cleanup --dry-run
aide memory cleanup --apply

# Output:
# Deleted 5 session memories older than 30 days
# Deleted 2 learning memories older than 90 days
```

### Manual Promotion

```bash
# Promote a learning to permanent pattern
aide memory update <id> --category=pattern

# Promote session insight to global preference
aide memory update <id> --category=global --tags="scope:global"
```

## Implementation Plan

### Phase 1: Basic Injection (session-start.ts)

1. Fetch static memories:
   ```bash
   aide memory list --category=global
   aide memory list --category=decision --tags="project:${projectName}"
   ```

2. Fetch dynamic memories:
   ```bash
   aide memory list --category=session --tags="project:${projectName}" \
     --limit=${dynamicCount} --sort=created_at:desc
   ```

3. Format and inject into `additionalContext`

### Phase 2: Search-Based Injection (keyword-detector.ts)

1. Extract search terms from prompt
2. Search memories:
   ```bash
   aide memory search --category=learning,pattern,gotcha \
     --limit=${searchLimit} "${terms.join(' OR ')}"
   ```
3. Inject relevant results with relevance scores

### Phase 3: Keyword Overrides

1. Detect context keywords in prompt
2. Set session state: `aide state set memoryDynamicCount N`
3. Session-start reads this for next injection

### Phase 4: Lifecycle Cleanup

1. Add `--ttl` flag to memory add
2. Add cleanup subcommand
3. Run cleanup on session start (async, non-blocking)

## Configuration Schema

```json
// .aide/config/aide.json
{
  "memory": {
    "injection": {
      "static": {
        "enabled": true,
        "categories": ["global", "decision"]
      },
      "dynamic": {
        "enabled": true,
        "defaultCount": 3,
        "maxCount": 10,
        "category": "session"
      },
      "relevant": {
        "enabled": true,
        "defaultLimit": 3,
        "maxLimit": 10,
        "categories": ["learning", "pattern", "gotcha"],
        "minRelevance": 0.5
      }
    },
    "lifecycle": {
      "session": { "ttlDays": 30 },
      "learning": { "ttlDays": 90 },
      "gotcha": { "ttlDays": 90 }
    },
    "modeOverrides": {
      "autopilot": { "dynamicCount": 5, "searchLimit": 5 },
      "ralph": { "dynamicCount": 5, "searchLimit": 5 },
      "eco": { "dynamicCount": 1, "searchLimit": 2 }
    }
  }
}
```

## Open Questions

1. Should search-based injection happen on every prompt or just session start?
2. How to handle memory conflicts (same topic, different conclusions)?
3. Should we support memory "pinning" (always inject specific memory)?
4. How to surface memory relevance to user (show what was injected)?
