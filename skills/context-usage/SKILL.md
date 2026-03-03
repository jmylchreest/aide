---
name: context-usage
description: Analyze current session context and token usage from OpenCode SQLite database
platforms:
  - opencode
triggers:
  - context usage
  - token usage
  - session stats
  - how much context
  - context budget
  - how big is this session
  - session size
---

# Context Usage Analysis

**Recommended model tier:** balanced (sonnet) - straightforward SQL queries

Analyze the current session's context window consumption, tool usage breakdown,
and token costs by querying the OpenCode SQLite database directly.

## Prerequisites

- The `$AIDE_SESSION_ID` environment variable must be set (injected automatically by aide on OpenCode).
- The OpenCode database is at `~/.local/share/opencode/opencode.db`.
- `sqlite3` must be available on the system.

If `$AIDE_SESSION_ID` is not set, abort with a message explaining this skill
only works on OpenCode.

## Workflow

Run the following queries **sequentially** in a single Bash call (chain with `&&`).
Present results to the user in a formatted summary after all queries complete.

### Step 1: Validate environment

```bash
test -n "$AIDE_SESSION_ID" && echo "Session: $AIDE_SESSION_ID" || echo "ERROR: AIDE_SESSION_ID not set"
```

If `AIDE_SESSION_ID` is not set, stop and inform the user.

### Step 2: Session overview

```bash
sqlite3 ~/.local/share/opencode/opencode.db "
SELECT
  s.title,
  s.slug,
  ROUND((julianday('now') - julianday(datetime(s.time_created/1000, 'unixepoch'))) * 24, 1) as hours_old,
  (SELECT COUNT(*) FROM message m WHERE m.session_id = s.id) as messages,
  CASE WHEN s.time_compacting IS NOT NULL THEN 'yes' ELSE 'no' END as compacted
FROM session s
WHERE s.id = '$AIDE_SESSION_ID';
"
```

### Step 3: Token totals

Sum tokens from `step-finish` parts (each represents one LLM turn):

```bash
sqlite3 ~/.local/share/opencode/opencode.db "
SELECT
  SUM(json_extract(data, '$.tokens.input')) as input_tokens,
  SUM(json_extract(data, '$.tokens.output')) as output_tokens,
  SUM(json_extract(data, '$.tokens.cache.read')) as cache_read_tokens,
  SUM(json_extract(data, '$.tokens.cache.write')) as cache_write_tokens,
  SUM(json_extract(data, '$.tokens.total')) as total_tokens,
  COUNT(*) as llm_turns
FROM part
WHERE session_id = '$AIDE_SESSION_ID'
  AND json_extract(data, '$.type') = 'step-finish';
"
```

### Step 4: Tool output breakdown

Show tool usage ranked by total output size:

```bash
sqlite3 ~/.local/share/opencode/opencode.db "
SELECT
  json_extract(data, '$.tool') as tool,
  COUNT(*) as calls,
  SUM(length(json_extract(data, '$.state.output'))) as total_output_bytes,
  ROUND(AVG(length(json_extract(data, '$.state.output')))) as avg_bytes,
  MAX(length(json_extract(data, '$.state.output'))) as max_bytes
FROM part
WHERE session_id = '$AIDE_SESSION_ID'
  AND json_extract(data, '$.type') = 'tool'
GROUP BY tool
ORDER BY total_output_bytes DESC;
"
```

### Step 5: Total session size

```bash
sqlite3 ~/.local/share/opencode/opencode.db "
SELECT
  SUM(length(json_extract(data, '$.state.output'))) as tool_output_bytes,
  SUM(length(json_extract(data, '$.state.input'))) as tool_input_bytes,
  SUM(length(data)) as total_part_bytes
FROM part
WHERE session_id = '$AIDE_SESSION_ID'
  AND json_extract(data, '$.type') = 'tool';
"
```

## Output Format

Present the results as a structured summary:

```
## Session Context Usage

**Session:** <title> (<slug>)
**Age:** <hours> hours | **Messages:** <count> | **Compacted:** yes/no

### Token Usage
| Metric | Count |
|--------|-------|
| Input tokens | <n> |
| Output tokens | <n> |
| Cache read | <n> |
| Cache write | <n> |
| **Total tokens** | **<n>** |
| LLM turns | <n> |

### Tool Output Breakdown (by total bytes)
| Tool | Calls | Total Output | Avg/call | Max |
|------|-------|-------------|----------|-----|
| ... | ... | ... | ... | ... |

### Session Size
- Tool outputs: <n> KB
- Tool inputs: <n> KB
- Total part storage: <n> KB (includes JSON metadata overhead)
```

Format byte values as KB (divide by 1024, round to 1 decimal).
Highlight the top 3 tools by total output as the biggest context consumers.
If any single tool call exceeds 20KB, flag it as a potential optimization target.
