# aide - AI Development Environment

## Overview

aide is a multi-agent orchestration framework for AI-assisted development. It provides intelligent delegation, persistent memory, parallel execution with swarm mode, and seamless integration with Claude Code and other AI tools.

**Core Philosophy:**
- Dynamic over static (skills, models, agents loaded at runtime)
- Single unified memory (notepad + swarm + learnings merged)
- Provider-agnostic (works with Claude, OpenRouter, OpenAI, Ollama)
- Git-native (worktrees for parallel work, `.aide/` committed to repo)

---

## Architecture Overview

```
aide/
├── src/                          # TypeScript - hooks and orchestration
│   ├── hooks/                    # Claude Code hook scripts
│   ├── agents/                   # Agent prompt definitions (markdown)
│   ├── skills/                   # Built-in skills (markdown)
│   └── lib/                      # Core TypeScript libraries
├── aide-memory/                  # Go - unified memory system
│   ├── cmd/                      # CLI entry point
│   ├── pkg/                      # Core packages
│   └── ffi/                      # TypeScript/Python bindings
├── .aide/                        # Project-local state (committed)
│   ├── skills/                   # Project-specific skills
│   ├── config/                   # Project config overrides
│   └── exports/                  # Human-readable memory exports
└── bridge/                       # Integration utilities
    └── skills-sh/                # skills.sh marketplace integration
```

---

## Component Details

### 1. Dynamic Skills System

**Location:** `.aide/skills/` (project) + `~/.aide/skills/` (global)

**Features:**
- Recursive subdirectory discovery
- Hot-reload on file changes
- Markdown with YAML frontmatter
- skills.sh marketplace integration

**Skill Format:**
```markdown
---
name: Deploy Production
triggers:
  - "deploy"
  - "ship it"
  - "release"
priority: 10
---

## Instructions

When deploying this project:
1. Run `npm run build`
2. Run `npm run test`
3. Execute `./scripts/deploy.sh`

## Verification

Confirm deployment succeeded by checking health endpoint.
```

**Discovery Flow:**
1. On `UserPromptSubmit` hook, scan skill directories
2. Match triggers against user prompt (case-insensitive)
3. Inject matching skill content into context
4. Cache with file watcher for hot-reload

**skills.sh Integration:**
```json
// .aide/skills/registry.json
{
  "installed": [
    { "name": "anthropics/claude-code", "version": "1.2.0" },
    { "name": "cursor-rules/react", "version": "2.0.0" }
  ],
  "autoUpdate": true,
  "syncInterval": "24h"
}
```

---

### 2. Model Aliases and Auto-Discovery

**Location:** `.aide/config/models.json`

**Design Goals:**
- No hardcoded model names
- Automatic support for new models (e.g., Opus 5)
- Provider-agnostic tier system

**Configuration:**
```json
{
  "tiers": {
    "fast": "Cheapest/fastest model",
    "balanced": "Good cost/capability balance",
    "smart": "Most capable model",
    "code": "Optimized for code generation"
  },

  "providers": {
    "anthropic": {
      "fast": "claude-haiku-*",
      "balanced": "claude-sonnet-*",
      "smart": "claude-opus-*"
    },
    "openrouter": {
      "fast": "anthropic/claude-3-haiku",
      "balanced": "anthropic/claude-3.5-sonnet",
      "smart": "anthropic/claude-3-opus"
    },
    "openai": {
      "fast": "gpt-4o-mini",
      "balanced": "gpt-4o",
      "smart": "o1-preview"
    },
    "ollama": {
      "fast": "llama3.2:3b",
      "balanced": "llama3.2:70b",
      "smart": "llama3.2:70b"
    }
  },

  "aliases": {
    "opus": "smart",
    "sonnet": "balanced",
    "haiku": "fast",
    "cheap": "fast",
    "quick": "fast",
    "thorough": "smart",
    "best": "smart"
  },

  "resolution": {
    "strategy": "pattern-match",
    "apiQueryFallback": true,
    "cacheFor": "24h"
  }
}
```

**Keyword Detection:**
```
model:smart           → explicit tier
model:opus            → alias → smart tier
using opus            → natural language
with haiku            → natural language
cheap fix this        → alias in context
```

**Auto-Discovery Algorithm:**
```typescript
function classifyModel(modelId: string): Tier {
  // Pattern matching for known providers
  if (/opus/i.test(modelId)) return 'smart';
  if (/sonnet/i.test(modelId)) return 'balanced';
  if (/haiku/i.test(modelId)) return 'fast';
  if (/o1|o3/i.test(modelId)) return 'smart';
  if (/gpt-4o(?!-mini)/i.test(modelId)) return 'balanced';
  if (/mini|turbo/i.test(modelId)) return 'fast';

  // Version heuristic: higher = better
  const version = modelId.match(/(\d+)/)?.[1];
  if (version && parseInt(version) >= 5) return 'smart';

  return 'balanced'; // safe default
}
```

---

### 3. Agent Definitions

**Location:** `src/agents/*.md`

**12 Core Agents (no tiers - use model override instead):**

| Agent | Default Model | Purpose | Tools |
|-------|---------------|---------|-------|
| `architect` | balanced | Analysis, debugging, architecture review | Read, Glob, Grep, LSP (read-only) |
| `executor` | balanced | Implementation, code changes | Read, Glob, Grep, Edit, Write, Bash |
| `explore` | fast | Fast codebase search | Read, Glob, Grep, AST search |
| `planner` | smart | Strategic planning, task decomposition | Read, Glob, Grep |
| `researcher` | balanced | External docs, APIs, web search | Read, WebSearch, WebFetch, Kagi, Exa |
| `designer` | balanced | UI/UX, frontend implementation | Read, Glob, Grep, Edit, Write, Bash |
| `writer` | fast | Documentation, comments | Read, Glob, Edit, Write |
| `product-owner` | balanced | Requirements, user stories, acceptance criteria | Read, Glob, Grep |
| `scientist` | balanced | Data analysis, statistics, ML | Read, Glob, Bash, python_repl |
| `data-architect` | balanced | Schema design, data modeling, ETL | Read, Glob, Grep, Edit |
| `reviewer` | smart | Code review + security audit (combined) | Read, Glob, Grep, AST, LSP |
| `qa-tester` | balanced | Testing, verification | Read, Glob, Bash |

**Agent Prompt Format:**
```markdown
---
name: architect
description: Strategic architecture and debugging advisor
defaultModel: balanced
readOnly: true
tools:
  - Read
  - Glob
  - Grep
  - lsp_diagnostics
  - lsp_hover
  - ast_grep_search
---

# Architect Agent

You are a strategic advisor for software architecture and debugging.

## Core Rules

1. You are READ-ONLY. You analyze but NEVER modify code.
2. When you identify changes needed, specify exact file:line locations.
3. Delegate implementation to `executor` agent.

## Capabilities

- Architecture review and recommendations
- Root cause analysis for bugs
- Performance analysis
- Security assessment
- Code quality evaluation

## Output Format

Always provide:
1. Analysis summary
2. Specific findings with file:line references
3. Recommended actions (for executor to implement)
```

**Model Override:**
```
"architect model:smart debug this race condition"
"explore using fast find all API endpoints"
```

---

### 4. Built-in Skills

**Location:** `src/skills/*.md`

| Skill | Triggers | Purpose |
|-------|----------|---------|
| `autopilot` | "autopilot", "build me", "I want a" | Full autonomous execution |
| `plan` | "plan this", "plan the" | Planning interview workflow |
| `ralph` | "ralph", "don't stop", "until done" | Persistence until completion |
| `build-fix` | "fix build", "fix errors", "fix lint", "fix types" | Build + lint + type errors |
| `analyze` | "analyze", "debug", "investigate" | Deep investigation |
| `deepsearch` | "search for", "find all" | Thorough codebase search |
| `review` | "review code", "security review", "audit" | Code + security review |
| `eco` | "eco", "$", "budget", "cheap mode" | Token-efficient execution |
| `git` | "commit", "branch", "worktree", "git" | Git operations + worktree management |
| `cancel` | "stop", "cancel", "abort" | Stop active modes |

---

### 5. HUD (Status Line)

**Display Format:**
```
[aide] eco | balanced | 2 agents | 5/12 | 45k ctx | 82% cache | $0.12 | 3m24s
       ^     ^          ^          ^      ^         ^           ^       ^
       mode  model      active     tasks  context   cache hit   cost    time
```

**Configuration:** `.aide/config/hud.json`
```json
{
  "enabled": true,
  "elements": ["mode", "model", "agents", "tasks", "context", "cache", "cost", "time"],
  "format": "[aide] {mode} | {model} | {agents} | {tasks} | {context} | {cache} | {cost} | {time}",
  "refresh": "on-response",
  "position": "statusline"
}
```

**Data Sources:**
| Element | Source |
|---------|--------|
| mode | `.aide/state/mode.json` |
| model | Current session / override |
| agents | Count of active Task subagents |
| tasks | TodoList API |
| context | Response header `x-token-count` |
| cache | Response header `x-cache-hit-ratio` |
| cost | Calculated: tokens × model pricing |
| time | Session start timestamp |

---

### 6. Swarm Mode with Shared Memory

**Activation Keywords:**
```
swarm 3                → 3 agents
swarm 5 executors      → 5 executor agents
swarm mixed            → auto-select agent types
```

**Architecture:**
```
┌─────────────────────────────────────────────────────────┐
│                   SWARM ORCHESTRATOR                     │
│  ┌─────────────────────────────────────────────────┐    │
│  │           SHARED MEMORY (aide-memory)            │    │
│  │  • Tasks (pending/claimed/done)                  │    │
│  │  • Discoveries (findings agents share)           │    │
│  │  • Decisions (architectural choices)             │    │
│  │  • Messages (inter-agent communication)          │    │
│  └─────────────────────────────────────────────────┘    │
│                          ▲                               │
│            ┌─────────────┼─────────────┐                │
│            │             │             │                │
│     ┌──────┴──────┐ ┌────┴────┐ ┌──────┴──────┐        │
│     │  Agent 1    │ │ Agent 2 │ │  Agent 3    │        │
│     │  worktree-1 │ │  wt-2   │ │  worktree-3 │        │
│     └─────────────┘ └─────────┘ └─────────────┘        │
└─────────────────────────────────────────────────────────┘
```

**Swarm Tools (injected into agents):**
```typescript
interface SwarmTools {
  claim_task(taskId: string): { success: boolean; task?: Task };
  complete_task(taskId: string, result: string): void;
  share_discovery(category: string, content: string): void;
  get_discoveries(category?: string): Discovery[];
  make_decision(topic: string, decision: string, rationale: string): boolean;
  get_decision(topic: string): Decision | null;
  send_message(content: string, toAgent?: string): void;
  get_messages(): Message[];
  report_blocker(description: string): void;
}
```

---

### 7. Git Worktree Management

**Purpose:** Isolate parallel agents to prevent file conflicts.

**Worktree Structure:**
```
project/                      # Main working directory
aide-worktrees/
├── swarm-task-1-auth/        # Agent 1's isolated worktree
├── swarm-task-2-api/         # Agent 2's isolated worktree
└── swarm-task-3-tests/       # Agent 3's isolated worktree
```

**Lifecycle:**
```typescript
// On swarm start
async function createWorktrees(tasks: Task[]): Promise<Worktree[]> {
  return Promise.all(tasks.map(async (task) => {
    const name = `swarm-${task.id}-${slugify(task.title)}`;
    const path = `../aide-worktrees/${name}`;
    const branch = `aide/${name}`;

    await exec(`git worktree add ${path} -b ${branch}`);
    return { name, path, branch, taskId: task.id };
  }));
}

// On swarm complete
async function mergeWorktrees(worktrees: Worktree[]): Promise<void> {
  for (const wt of worktrees) {
    try {
      await exec(`git merge --no-ff ${wt.branch} -m "Merge ${wt.name}"`);
    } catch (e) {
      // Flag conflict for user resolution
      console.error(`Conflict in ${wt.name}, manual merge required`);
    }
    await exec(`git worktree remove ${wt.path}`);
    await exec(`git branch -d ${wt.branch}`);
  }
}
```

**State Tracking:** `.aide/state/worktrees.json`
```json
{
  "active": [
    {
      "name": "swarm-task-1-auth",
      "path": "../aide-worktrees/swarm-task-1-auth",
      "branch": "aide/swarm-task-1-auth",
      "taskId": "task-1",
      "agent": "executor",
      "createdAt": "2026-02-03T10:00:00Z"
    }
  ]
}
```

---

### 8. MCP and Tool Guardrails

**Philosophy:** Use Claude's native MCP discovery. aide adds guardrails via hooks.

**Read-Only Enforcement (PreToolUse hook):**
```typescript
const WRITE_TOOLS = ['Edit', 'Write', 'Bash', 'NotebookEdit'];
const READ_ONLY_AGENTS = ['architect', 'explore', 'researcher', 'analyst', 'product-owner', 'reviewer'];

function preToolUse({ toolName, agentName }): HookResult {
  if (READ_ONLY_AGENTS.includes(agentName) && WRITE_TOOLS.includes(toolName)) {
    return {
      continue: false,
      message: `Agent "${agentName}" is read-only. Delegate to executor for modifications.`
    };
  }
  return { continue: true };
}
```

**MCP Tools Available:**
| Tool | Type | Available To |
|------|------|--------------|
| Kagi Search | Read-only | All agents |
| Exa Search | Read-only | All agents |
| WebFetch | Read-only | All agents |
| LSP diagnostics | Read-only | architect, executor, reviewer |
| AST search | Read-only | explore, architect, reviewer |
| AST replace | Write | executor only |
| python_repl | Sandboxed | scientist only |

---

### 9. Hooks System

**Location:** `src/hooks/`

| Hook | Event | Purpose |
|------|-------|---------|
| `keyword-detector.ts` | UserPromptSubmit | Detect magic keywords, model overrides |
| `skill-injector.ts` | UserPromptSubmit | Load and inject matching skills |
| `pre-tool-enforcer.ts` | PreToolUse | Enforce read-only, inject reminders |
| `persistence.ts` | Stop | Prevent stopping with incomplete work |
| `hud-updater.ts` | PostToolUse | Update status line |
| `session-start.ts` | SessionStart | Initialize state, load config |

**Hook Contract:**
```typescript
// Input (stdin JSON)
interface HookInput {
  event: string;
  sessionId: string;
  cwd: string;
  prompt?: string;        // UserPromptSubmit
  toolName?: string;      // PreToolUse/PostToolUse
  agentName?: string;     // If in subagent context
}

// Output (stdout JSON)
interface HookOutput {
  continue: boolean;      // false = block action
  message?: string;       // Shown to user if blocked
  hookSpecificOutput?: {
    additionalContext?: string;  // Injected into context
  };
}
```

---

## aide-memory (Go)

### Overview

Unified memory system combining:
- Notepad wisdom (learnings, decisions, issues)
- Swarm coordination (tasks, messages)
- Semantic search (optional)

### Architecture

```
aide-memory/
├── cmd/
│   └── aide-memory/
│       └── main.go              # CLI entry point
├── pkg/
│   ├── store/
│   │   ├── bbolt.go             # KV storage (bbolt)
│   │   └── vector.go            # Vector storage (chromem-go)
│   ├── memory/
│   │   ├── types.go             # Core types
│   │   ├── api.go               # High-level API
│   │   ├── decay.go             # Priority decay
│   │   └── export.go            # Markdown/JSON export
│   ├── swarm/
│   │   ├── tasks.go             # Atomic task claiming
│   │   └── messages.go          # Inter-agent messaging
│   └── server/
│       ├── socket.go            # Unix socket server
│       └── http.go              # Optional HTTP API
├── ffi/
│   ├── bindings.go              # CGO exports
│   └── typescript/              # TS bindings via FFI
└── internal/
    └── testutil/                # Test utilities
```

### Data Model

```go
package memory

type Category string

const (
    CategoryLearning  Category = "learning"
    CategoryDecision  Category = "decision"
    CategoryIssue     Category = "issue"
    CategoryDiscovery Category = "discovery"
    CategoryBlocker   Category = "blocker"
)

type Memory struct {
    ID        string    `json:"id"`
    Category  Category  `json:"category"`
    Content   string    `json:"content"`
    Tags      []string  `json:"tags"`
    Priority  float32   `json:"priority"`   // 0.0-1.0, decays over time
    Plan      string    `json:"plan"`       // Optional plan scope
    Agent     string    `json:"agent"`      // Which agent created it
    CreatedAt time.Time `json:"createdAt"`
    UpdatedAt time.Time `json:"updatedAt"`
}

type Task struct {
    ID          string    `json:"id"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    Status      string    `json:"status"`    // pending, claimed, done, blocked
    ClaimedBy   string    `json:"claimedBy"` // Agent ID
    ClaimedAt   time.Time `json:"claimedAt"`
    Worktree    string    `json:"worktree"`  // Git worktree path
    Result      string    `json:"result"`    // Outcome when completed
}

type Message struct {
    ID        uint64    `json:"id"`
    From      string    `json:"from"`
    To        string    `json:"to"`         // Empty = broadcast
    Content   string    `json:"content"`
    ReadBy    []string  `json:"readBy"`
    CreatedAt time.Time `json:"createdAt"`
}

type Decision struct {
    Topic     string    `json:"topic"`      // Unique key
    Decision  string    `json:"decision"`
    Rationale string    `json:"rationale"`
    DecidedBy string    `json:"decidedBy"`
    CreatedAt time.Time `json:"createdAt"`
}
```

### Storage Layer

**Primary: bbolt (pure Go, ACID)**
```go
// Buckets
const (
    BucketMemories  = "memories"
    BucketTasks     = "tasks"
    BucketMessages  = "messages"
    BucketDecisions = "decisions"
    BucketMeta      = "meta"
)

func (s *Store) AddMemory(m *Memory) error {
    return s.db.Update(func(tx *bbolt.Tx) error {
        b := tx.Bucket([]byte(BucketMemories))
        data, _ := json.Marshal(m)
        return b.Put([]byte(m.ID), data)
    })
}

func (s *Store) ClaimTask(taskID, agentID string) (*Task, error) {
    var task *Task
    err := s.db.Update(func(tx *bbolt.Tx) error {
        b := tx.Bucket([]byte(BucketTasks))
        data := b.Get([]byte(taskID))
        if data == nil {
            return ErrNotFound
        }

        json.Unmarshal(data, &task)
        if task.Status != "pending" {
            return ErrAlreadyClaimed
        }

        task.Status = "claimed"
        task.ClaimedBy = agentID
        task.ClaimedAt = time.Now()

        data, _ = json.Marshal(task)
        return b.Put([]byte(taskID), data)
    })
    return task, err
}
```

**Secondary: chromem-go (semantic search)**
```go
func (s *Store) SemanticSearch(query string, limit int) ([]Memory, error) {
    results, err := s.vectors.Query(context.Background(), query, limit, nil, nil)
    if err != nil {
        return nil, err
    }

    memories := make([]Memory, len(results))
    for i, r := range results {
        s.db.View(func(tx *bbolt.Tx) error {
            b := tx.Bucket([]byte(BucketMemories))
            data := b.Get([]byte(r.ID))
            json.Unmarshal(data, &memories[i])
            return nil
        })
    }
    return memories, nil
}
```

### CLI Interface

```bash
# Memory operations
aide-memory add --category=learning "Found auth middleware at src/auth.ts"
aide-memory add --category=decision --topic=auth-strategy "JWT with refresh tokens" --tags=auth,security
aide-memory search "authentication"                    # Keyword search
aide-memory semantic "how do we handle auth?"          # Semantic search
aide-memory list --category=decision --plan=auth-refactor

# Swarm operations
aide-memory task create "Implement user model" --description="..."
aide-memory task claim task-1 --agent=executor-1
aide-memory task complete task-1 --result="User model at src/models/user.ts"
aide-memory task list --status=pending

aide-memory message send "User model ready" --from=executor-1
aide-memory message send "Need user model" --from=executor-2 --to=executor-1
aide-memory message list --agent=executor-1

aide-memory decision set auth-strategy "JWT" --rationale="..."
aide-memory decision get auth-strategy

# Export
aide-memory export --format=markdown --output=.aide/exports/
aide-memory export --format=json --output=.aide/exports/memory.json

# Server mode
aide-memory serve --socket=/tmp/aide-memory.sock
aide-memory serve --http=:8080
```

### Unix Socket Protocol

```json
// Request
{"cmd": "add", "category": "learning", "content": "Found middleware", "tags": ["auth"]}
{"cmd": "search", "query": "authentication", "limit": 10}
{"cmd": "claim", "taskId": "task-1", "agentId": "executor-1"}
{"cmd": "decision.get", "topic": "auth-strategy"}

// Response
{"ok": true, "data": {...}}
{"ok": false, "error": "task already claimed"}
```

### FFI Bindings (for TypeScript hooks)

```go
// ffi/bindings.go
package main

import "C"
import (
    "encoding/json"
    "unsafe"
)

//export AideMemoryAdd
func AideMemoryAdd(dbPath, category, content, tags *C.char) *C.char {
    // ... implementation
    return C.CString(`{"ok": true}`)
}

//export AideMemoryClaimTask
func AideMemoryClaimTask(dbPath, taskId, agentId *C.char) *C.char {
    // ... implementation
}
```

**TypeScript usage:**
```typescript
import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const aideMemory = require('./aide-memory.node');

const result = aideMemory.add({
  category: 'learning',
  content: 'Found auth middleware',
  tags: ['auth']
});
```

### File Format

```
.aide/
└── memory/                   # All memory storage
    ├── store.db              # bbolt database (binary, ACID)
    ├── vectors/              # chromem-go persistence (optional)
    │   └── memories.gob
    └── exports/              # Human-readable (auto-generated)
        ├── learnings.md
        ├── decisions.md
        ├── issues.md
        └── current-swarm.md
```

**Git Considerations:**
- `store.db` is binary but deterministic (same content = same bytes)
- Export markdown on each write for human review
- `.gitattributes`: `*.db binary`

---

## Implementation Phases

### Phase 1: Foundation
- [ ] Project structure setup
- [ ] aide-memory Go module with bbolt storage
- [ ] Basic CLI (add, search, list, export)
- [ ] Unix socket server

### Phase 2: Core Hooks
- [ ] keyword-detector (magic keywords, model override)
- [ ] skill-injector (dynamic skill loading)
- [ ] persistence (stop prevention)
- [ ] pre-tool-enforcer (read-only guardrails)

### Phase 3: Agent System
- [ ] 12 agent prompt definitions
- [ ] Model alias resolution
- [ ] Agent spawning with tool filtering

### Phase 4: Swarm Mode
- [ ] Task claiming in aide-memory
- [ ] Inter-agent messaging
- [ ] Decision coordination
- [ ] Git worktree integration

### Phase 5: Semantic Search
- [ ] chromem-go integration
- [ ] Embedding generation (local or API)
- [ ] Cross-plan semantic queries

### Phase 6: HUD and Polish
- [ ] Status line implementation
- [ ] Cost/context/cache tracking
- [ ] skills.sh integration
- [ ] Documentation

---

## Configuration Files

### `.aide/config/settings.json`
```json
{
  "defaultMode": "balanced",
  "ecoModeByDefault": false,
  "autoSkillDiscovery": true,
  "skillsShSync": true,
  "memoryDecayRate": 0.01,
  "maxSwarmAgents": 5,
  "worktreeBasePath": "../aide-worktrees"
}
```

### `.aide/config/models.json`
(See Model Aliases section above)

### `.aide/config/hud.json`
(See HUD section above)

---

## Testing Strategy

### Unit Tests
- aide-memory: Go standard testing
- Hooks: Jest with mock Claude Code events
- Skills: Snapshot testing for prompt injection

### Integration Tests
- Swarm coordination with multiple processes
- Worktree lifecycle (create, work, merge, cleanup)
- Memory persistence across sessions

### E2E Tests
- Full autopilot workflow
- Swarm with 3 agents completing task
- Skill hot-reload during session

---

## Security Considerations

1. **Read-only enforcement:** Hard block via PreToolUse hook, not just prompts
2. **Path sanitization:** Prevent traversal in skill/memory paths
3. **Worktree isolation:** Each agent's worktree is sandboxed
4. **No secrets in .aide/:** Memory DB should not store credentials
5. **FFI boundary:** Validate all inputs from TypeScript

---

## Future Considerations

1. **Remote memory sync:** Optional sync across machines
2. **Team collaboration:** Shared memory for team projects
3. **Plugin system:** Third-party skill/agent packages
4. **IDE integration:** VS Code extension for HUD
5. **Metrics dashboard:** Token usage, cost trends, agent performance
