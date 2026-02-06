# Automatic Memory Capture

## Overview

This document describes how to implement automatic session memory capture in AIDE, inspired by Supermemory's approach but adapted for local-first operation.

## How Supermemory Captures Memory

Supermemory reads Claude's transcript file (`.jsonl`) on the `Stop` hook and:

1. **Parses transcript** - Reads each JSON line from the transcript
2. **Tracks progress** - Stores last captured UUID to avoid duplicates
3. **Filters content** - Only captures `user` and `assistant` messages
4. **Formats structured text**:
   ```
   [turn:start timestamp="2026-02-05T10:30:00Z"]

   [role:user]
   Can you help me fix the auth bug?
   [user:end]

   [role:assistant]
   I'll look at the authentication code...
   [assistant:end]

   [tool:Read]
   file_path: src/auth.ts
   [tool:end]

   [tool_result:Read status="success"]
   export function authenticate(token: string)...
   [tool_result:end]

   [turn:end]
   ```
5. **Cleans content** - Removes system reminders, thinking blocks
6. **Truncates** - Tool results capped at 500 chars

## Proposed AIDE Implementation

### Activation

Add a new mode/keyword: `autocapture` or integrate with existing modes.

```
# Enable for session
autocapture on

# Or as part of another mode
autopilot build the feature  # autopilot could enable autocapture
```

### Hook: session-end.ts (Stop event)

```typescript
// On Stop hook:
// 1. Read transcript from transcript_path
// 2. Parse entries since last capture
// 3. Format as structured memory
// 4. Store via aide memory add --category=session
```

### Memory Schema Extension

```typescript
interface SessionMemory {
  id: string;
  category: 'session';
  content: string;           // Formatted turn text
  metadata: {
    session_id: string;
    project: string;         // Git remote or path
    timestamp: string;
    turn_count: number;
    tools_used: string[];    // ['Edit', 'Bash', 'Read']
    files_modified: string[]; // Extracted from Edit/Write tools
    summary?: string;        // Optional AI-generated summary
  };
  tags: string[];            // ['auth', 'bugfix', etc.]
}
```

### Capture Format

```
[session:start id="abc123" project="aide" timestamp="2026-02-05T10:30:00Z"]

## User Request
Can you help me fix the auth bug in the login flow?

## Work Performed
- Read src/auth.ts to understand current implementation
- Identified missing token validation
- Added validateToken() function
- Updated login handler to use validation

## Files Modified
- src/auth.ts (added validateToken function)
- src/handlers/login.ts (integrated validation)

## Tools Used
Read, Edit, Bash (npm test)

## Outcome
Fixed authentication bug. Tests passing.

[session:end]
```

### Filtering Rules

**Skip capturing:**
- System reminders (`<system-reminder>`)
- Injected context (`<aide-session-start>`, `<supermemory-context>`)
- Thinking blocks
- Large file reads (just note "Read file X")
- Glob/Grep results (just note "Searched for X")

**Always capture:**
- User messages (the actual request)
- Assistant explanations
- Edit/Write operations (what changed)
- Bash commands and outcomes
- Task tool usage (subagent spawning)

**Truncation limits:**
- Tool inputs: 200 chars
- Tool results: 500 chars
- Total turn: 10,000 chars (summarize if larger)

### Storage

```bash
# Store as session memory
aide memory add \
  --category=session \
  --tags="project:aide,session:abc123" \
  "$(cat formatted_turn.txt)"
```

### Retrieval on Session Start

Modify `session-start.ts` to fetch recent session memories:

```typescript
// Fetch last 3 session summaries for this project
const sessions = await runAide(cwd, [
  'memory', 'search',
  '--category=session',
  '--limit=3',
  `project:${projectName}`
]);

// Inject as context
<aide-recent-sessions>
## Recent Work (last 3 sessions)

### Session abc123 (2 hours ago)
Fixed authentication bug in login flow.
Files: src/auth.ts, src/handlers/login.ts

### Session def456 (yesterday)
Implemented user profile page.
Files: src/pages/profile.tsx, src/api/user.ts

</aide-recent-sessions>
```

### Configuration

```json
// .aide/config/aide.json
{
  "autocapture": {
    "enabled": false,           // Opt-in by default
    "onModes": ["autopilot", "ralph"],  // Auto-enable for these modes
    "skipTools": ["Read", "Glob", "Grep", "LS"],
    "maxTurnSize": 10000,
    "summarize": true,          // Use AI to summarize large turns
    "injectOnStart": true,      // Show recent sessions on start
    "maxRecentSessions": 3
  }
}
```

## Implementation Plan

1. **Phase 1: Basic Capture**
   - Add `session-end.ts` hook (Stop event)
   - Parse transcript, format turns
   - Store via `aide memory add --category=session`

2. **Phase 2: Smart Filtering**
   - Skip noisy tools (Read results, Grep results)
   - Extract file modification list from Edit/Write
   - Track tools used per turn

3. **Phase 3: Summarization**
   - For large turns, generate summary
   - Could use local model or Claude itself (meta-prompt)

4. **Phase 4: Context Injection**
   - Modify session-start to fetch recent sessions
   - Smart relevance filtering based on current project

## Comparison with Supermemory

| Aspect | Supermemory | AIDE (Proposed) |
|--------|-------------|-----------------|
| Trigger | Stop hook | Stop hook |
| Storage | Cloud API | Local BBolt |
| Format | Structured tags | Markdown with metadata |
| Filtering | Skip Read results | Configurable skip list |
| Retrieval | Semantic search (AI) | Full-text + category filter |
| Summarization | Cloud AI | Optional (local/meta-prompt) |
| Privacy | Data sent to cloud | All local |

## Open Questions

1. Should autocapture be opt-in or opt-out?
2. How to handle very long sessions (>50 turns)?
3. Should we store raw transcript or always summarize?
4. How to deduplicate similar sessions?
