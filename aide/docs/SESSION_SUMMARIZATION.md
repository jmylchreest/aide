# Session Summarization with Claude

## Overview

Use Claude Haiku to automatically summarize sessions on the Stop hook and store as memories.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Stop Hook Triggered                       │
│                (session ends, user types /exit)              │
└─────────────────────────────┬───────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  session-end.ts hook                         │
│                                                              │
│  1. Read transcript from transcript_path                     │
│  2. Format relevant turns (skip noise)                       │
│  3. Call Claude Haiku API for summarization                  │
│  4. Store summary via `aide memory add --category=session`   │
└─────────────────────────────────────────────────────────────┘
```

## Implementation

### Environment Variables

```bash
# Required for summarization
ANTHROPIC_API_KEY=sk-ant-...

# Optional
AIDE_SUMMARIZE_MODEL=claude-3-haiku-20240307  # Default
AIDE_SUMMARIZE_ENABLED=true                    # Enable/disable
```

### Hook: session-end.ts

```typescript
import Anthropic from '@anthropic-ai/sdk';

const SUMMARIZE_PROMPT = `Summarize this coding session concisely. Include:
- What the user asked for
- Key work performed
- Files modified (if any)
- Outcome/status

Keep it under 200 words. Focus on what would be useful context for future sessions.

Session transcript:
`;

async function summarizeSession(transcript: string): Promise<string | null> {
  const apiKey = process.env.ANTHROPIC_API_KEY;
  if (!apiKey) {
    return null; // Skip summarization if no API key
  }

  const client = new Anthropic({ apiKey });

  const response = await client.messages.create({
    model: process.env.AIDE_SUMMARIZE_MODEL || 'claude-3-haiku-20240307',
    max_tokens: 500,
    messages: [{
      role: 'user',
      content: SUMMARIZE_PROMPT + transcript
    }]
  });

  return response.content[0].type === 'text'
    ? response.content[0].text
    : null;
}

async function main() {
  const input = await readStdin();
  const { transcript_path, session_id, cwd } = input;

  if (!transcript_path || !process.env.AIDE_SUMMARIZE_ENABLED) {
    writeOutput({ continue: true });
    return;
  }

  // 1. Parse and format transcript
  const formatted = formatTranscript(transcript_path, session_id);
  if (!formatted || formatted.length < 100) {
    writeOutput({ continue: true });
    return;
  }

  // 2. Summarize with Haiku
  const summary = await summarizeSession(formatted);
  if (!summary) {
    writeOutput({ continue: true });
    return;
  }

  // 3. Store as memory
  const projectName = getProjectName(cwd);
  await runAide(cwd, [
    'memory', 'add',
    '--category=session',
    `--tags=project:${projectName},session:${session_id.slice(0, 8)}`,
    summary
  ]);

  writeOutput({ continue: true });
}
```

## Why Haiku?

| Model | Speed | Cost | Quality for Summaries |
|-------|-------|------|----------------------|
| Haiku | ~0.5s | $0.25/1M tokens | Excellent |
| Sonnet | ~2s | $3/1M tokens | Overkill |
| Opus | ~5s | $15/1M tokens | Way overkill |

For summarization tasks:
- Haiku is **12x cheaper** than Sonnet
- Haiku is **4x faster** than Sonnet
- Summary quality is essentially the same

**Estimated cost per session:** ~$0.001 (1/10th of a cent)

## Summarization Prompt

```
Summarize this coding session concisely. Include:

1. **Request**: What did the user ask for?
2. **Work Done**: Key actions taken (briefly)
3. **Files Changed**: List modified files (if any)
4. **Outcome**: Was the task completed? Any issues?

Format as markdown. Keep under 200 words.
Focus on information useful for resuming work later.

---
Session transcript:
[transcript here]
```

## Example Output

**Input transcript** (2000 tokens of conversation about fixing auth):

**Haiku summary** (stored as memory):

```markdown
## Session Summary

**Request**: Fix authentication bug where JWT tokens weren't being validated

**Work Done**:
- Investigated auth flow in `src/auth/jwt.ts`
- Found missing `verify()` call in token validation
- Added proper JWT verification with expiry check
- Updated tests to cover edge cases

**Files Changed**:
- `src/auth/jwt.ts` - Added `validateToken()` function
- `src/auth/middleware.ts` - Integrated validation
- `tests/auth.test.ts` - Added token validation tests

**Outcome**: Bug fixed. All tests passing. PR ready for review.
```

## Configuration

```json
// .aide/config/aide.json
{
  "summarization": {
    "enabled": true,
    "model": "claude-3-haiku-20240307",
    "minTurnLength": 100,        // Skip trivial sessions
    "maxInputTokens": 4000,      // Truncate long sessions
    "injectOnStart": true,       // Show recent summaries on start
    "maxRecentSessions": 3
  }
}
```

## Privacy Considerations

1. **API key required** - Summarization only happens if user provides key
2. **Opt-in** - Disabled by default, user must enable
3. **Data sent to Anthropic** - Transcript content is sent for summarization
4. **Summary stored locally** - Result stays in local `.aide/memory/`

## Alternative: Local Summarization

For users who don't want to use cloud APIs:

1. **Ollama integration** - Use local Llama/Mistral for summarization
2. **Simple extraction** - Rule-based extraction without AI
3. **Manual summaries** - Prompt user to write summary

```typescript
// Ollama alternative
async function summarizeLocal(transcript: string): Promise<string> {
  const response = await fetch('http://localhost:11434/api/generate', {
    method: 'POST',
    body: JSON.stringify({
      model: 'mistral',
      prompt: SUMMARIZE_PROMPT + transcript,
      stream: false
    })
  });
  const data = await response.json();
  return data.response;
}
```

## Implementation Steps

1. **Add @anthropic-ai/sdk dependency** to package.json
2. **Create session-end.ts hook** with transcript parsing
3. **Add summarization call** to Haiku
4. **Store via aide memory add**
5. **Update session-start.ts** to inject recent summaries
6. **Add configuration options** to aide.json

## Cost Analysis

Assuming:
- Average session: 2000 input tokens, 200 output tokens
- Haiku pricing: $0.25/1M input, $1.25/1M output

**Per session cost:**
- Input: 2000 × $0.25/1M = $0.0005
- Output: 200 × $1.25/1M = $0.00025
- **Total: ~$0.00075 per session** (less than 1/10th of a cent)

**Monthly cost (20 sessions/day):**
- 20 × 30 × $0.00075 = **$0.45/month**

This is essentially free.
