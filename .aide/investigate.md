# Investigation Items (Deferred)

Items identified during the context-reduction work that need further
investigation before deciding on an approach.

## 1. Skill Injection Size

**Problem:** Skills are the single largest source of context bloat. Up to 3
full skill markdown files can be injected per turn, totaling 30-60KB. The
largest skills are `swarm` (21KB), `memorise` (10KB). Total
bundled skills across 21 files: ~150KB.

**Questions to investigate:**

- Can skills be split into a "summary" (always injected) and "full" (injected
  on demand)?
- Could skills be loaded lazily — inject a skills index on session start, then
  fetch the full skill only when matched?
- Are there skills that are rarely used and could be removed or merged?
- What's the token overhead per skill? Is the markdown formatting itself
  wasteful (tables, examples, etc.)?

**Potential approaches:**

- Tiered injection: inject skill summary (~200 words) first, full skill only
  after explicit activation
- Skill compression: strip examples and verbose sections from skill files
- Dynamic skill unloading: remove skill context after N turns without
  relevant tool use

## 2. Context Budget Decision-Making

**Problem:** The `context_pressure` signal (0.0 - 1.0) is now exposed via
`state_get`, but nothing acts on it yet. When pressure is high, the system
should do something — but what?

**Questions to investigate:**

- Should the system automatically switch to "eco" mode at high pressure?
- Should it emit a system-level hint telling the model to be more concise?
- Should it increase pruning aggressiveness (e.g., purge more tool outputs)?
- Should it trigger early compaction?
- What pressure thresholds are appropriate? (e.g., 0.5 = warning, 0.8 = critical)

**Constraints:**

- Neither OpenCode nor Claude Code supports modifying message history
- Compaction is controlled by the host platform, not by aide
- Any backpressure must work through system prompts or tool output annotations

**Potential approaches:**

- At pressure > 0.5: inject a system note "Context is growing large, prefer
  concise responses"
- At pressure > 0.8: auto-enable more aggressive pruning (lower dedup
  thresholds, trim superseded reads entirely instead of annotating)
- Expose pressure in the HUD so the user can see it

## 3. MCP Tool Result Size Caps

**Problem:** Some MCP tools return very large results with no truncation:

- `memory_list`: up to 50 items
- `code_search`: up to 20 results with full signatures
- `code_references`: up to 50 results

**Questions to investigate:**

- Should the aide MCP server enforce max result sizes?
- Should truncation happen in the Go backend or in the TS hook layer?
- What are reasonable defaults? (e.g., memory_list → 20, code_references → 25)
- Should results include a "truncated, use --limit to see more" hint?
