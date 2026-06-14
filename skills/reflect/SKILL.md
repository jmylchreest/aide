---
name: reflect
description: Run the instinct parser catalogue against this session's observe events to surface candidate patterns for promotion to memories. Two-pass: gather candidates, judge intent, write proposals.
triggers:
  - reflect on this
  - reflect on the session
  - find instincts
  - extract instincts
  - propose instincts
---

# Reflect

Extract candidate **instincts** — patterns repeatedly observed in this session
that might be worth promoting to durable memories.

## Your role as the agent running this skill

You **propose, the user approves**. Nothing this skill does writes a memory
or marks anything superseded without an explicit yes from the user.

Detectors emit proposals into a holding bucket (they're never auto-promoted
to memories). When this skill runs, your job is to make those proposals
*reviewable* — to add the judgement that mechanical matching can't:

1. **Classify intent** of user prompts in convergence windows — was the
   user actually correcting the previous edit, or just commenting?
2. **Rewrite the content into a useful memory** — see below. This is the
   most important step; the detector's content is a `[DRAFT — rewrite…]`
   placeholder, never the finished memory.
3. **Judge semantic conflicts** — which existing memories (if any) does
   this proposal supersede?
4. **Recommend an action** — accept (with rewritten content + supersedes),
   reject (why), or leave open for the user to think about.

Then **stop and ask**. Surface each proposal to the user with your
recommendation. Wait for explicit approval before running
`aide reflect accept|reject`. The CLI is the write surface; your role is
to make the user's approval decision as well-informed as possible, not to
make it for them.

Concretely: do not chain "list proposals → accept proposals" in the same
turn. List, summarise, recommend, **wait**, then act on user instruction.

## Why rewriting is the default, not the exception

The detector emits an **observation** ("`cat` was run 5 times in 1 minute").
That's a structural signal, not a useful memory. A useful memory captures:

- **Why** the repetition / convergence happened (the underlying need or
  mistake the agent kept circling around).
- **What** the canonical alternative is (a specific file path, command,
  pattern, or piece of project knowledge).
- **Scope** (this codebase / this kind of task / this directory).

The agent reviewing the proposal does this synthesis by reading the
evidence snapshot. The detector can't — it only knows the count.

Bad memory (the raw template): *"In this project, `cat` is run repeatedly.
Cache its output."*

Good memory (after agent synthesis): *"When investigating the aide
README for plugin/config questions, the file is stable per commit and the
config table sits around lines 11-25 — inject the slice via Read with
offset/limit instead of re-`cat`-ing the whole file."*

The proposal's content field literally starts with `[DRAFT — rewrite with
concrete context before accepting]` to make this obvious. If you accept
without rewriting, you've stored noise.

## How this skill differs from the automatic Stop hook

- The **Stop hook** (`AIDE_REFLECT=1` env or `reflect.enabled=true` in
  `.aide/config/aide.json`) runs `aide reflect run` automatically at session
  end. Deterministic-only: marker matching for convergence, pure counting
  for repetition. No supersession beyond structural `instinct_key:*` matches.
- **This skill** adds a second pass: it lists user-prompt candidates that
  fall in convergence-relevant windows, you judge intent in-context, you
  search for semantically conflicting memories, then you feed everything
  back. Higher-quality output, no extra API cost — uses tokens you'd be
  spending anyway.

## Session resolution

All commands below default to the **current session** — `aide` finds it by
checking, in order: an explicit `--session=<id>` flag, the `AIDE_SESSION_ID`
env var (set by OpenCode automatically; not by Claude Code), then the
session of the most recent observe event. Pass `--session=<id>` to target
a specific session, or run `./.aide/bin/aide reflect current-session` to
see what would be resolved.

## Steps

### 1. Get candidate prompts

```bash
./.aide/bin/aide reflect candidates
```

Returns JSON like:

```json
{
  "session_id": "abc123",
  "guidance": "For each prompt, judge whether it was correcting...",
  "asks": ["intent"],
  "prompts": [
    {
      "id": "01JF...A",
      "timestamp": "...",
      "text": "no don't add async — it should stay sync",
      "preceding_edit": "Edit src/api/users.ts",
      "following_edit": "Edit src/api/users.ts",
      "file_path": "src/api/users.ts"
    }
  ]
}
```

If `prompts` is empty, skip to step 4 (deterministic run only).

### 2. Classify each prompt

For each prompt, judge its `intent` using the surrounding edit context:

- `corrective` — the user was redirecting the assistant's last action
- `positive` — the user was affirming the assistant's last action ("perfect", "ship it", "lgtm")
- `neutral` — neither corrective nor affirming (e.g. a new task, a question)

Build a JSON array:

```json
[
  {"id": "01JF...A", "intent": "corrective", "confidence": 0.95},
  {"id": "01JF...B", "intent": "neutral"}
]
```

### 3. Run with classifications

```bash
./.aide/bin/aide reflect run --llm \
  --classifications-json='[{"id":"01JF...A","intent":"corrective"}]'
```

Returns a JSON summary: `{"proposals_written": N, "shapes": {...}}`.

The `--llm` flag puts the runner in LLM mode, which runs the `RequiresLLM`
detectors (convergence, friction) in addition to repetition. The Stop hook
never passes it, so those detectors only ever surface in this reviewed pass.

### 4. Run in LLM mode without classifications

If step 1 returned no candidates, still run with `--llm` (not bare) so
friction and the marker-based convergence pass fire — only the LLM-graded
convergence intent is skipped:

```bash
./.aide/bin/aide reflect run --llm
```

### 5. List proposals and summarise for the user

```
mcp__plugin_aide_aide__instinct_proposals_list { "status": "open" }
```

Summarise each new proposal's `summary` field for the user. Don't act on
anything yet — let them decide.

### 6. You decide what gets superseded

This is where your judgement matters most. The structural auto-supersession
(same `instinct_key:*` tag) only catches instinct-on-instinct dedup. The
interesting case is when a new instinct contradicts a **manually-set**
memory the user wrote earlier — e.g. a "documentation standard: always run
rustdoc" memory being superseded by a new "rustdoc runs repeatedly, cache
it" instinct.

Mechanical matching can't catch that. You can. Process:

1. Extract the key subject from the proposal (e.g. "rustdoc", a file path,
   a command name).
2. `mcp__plugin_aide_aide__memory_search { "query": "<subject>" }`
3. Read each result. Judge — **you are the judge**:
   - Does the new instinct's guidance *contradict* this memory's guidance?
   - Does it *replace* it with a better recommendation?
   - Or does it just touch the same topic without conflicting?
4. Only collect IDs that genuinely conflict. False positives create silent
   memory loss; bias toward "leave it" when uncertain.
5. Surface your reasoning to the user before acting: "I think proposal X
   supersedes memory Y because Z — accept with `--supersedes=Y`?"

### 7. Rewrite the content, then present recommendations — do not auto-act

For each proposal:

1. **Read the evidence snapshot** via `mcp__plugin_aide_aide__instinct_inspect`.
2. **Synthesise the underlying lesson**. What was the agent (you, or a past
   you) actually trying to achieve? Why did the pattern repeat / converge?
   What's the canonical alternative for this codebase?
3. **Draft the rewritten memory content**. Skip the `[DRAFT — …]` template
   text from the proposal; write a useful instinct from scratch.
4. **Present to the user** with the rewrite inline:

> Proposal `01JF…` (repetition, `rustdoc` × 7 in 5 min). Reading the
> evidence: 6 of the 7 calls were `rustdoc --no-deps` checking the same
> public crate while iterating on a doctest. I'd accept with this
> rewritten content and supersede memory `01JD…` ("always run rustdoc")
> because the new guidance refines, not contradicts, that one:
>
> > "When iterating on a single doctest, `cargo test --doc <module>`
> > re-runs only that doctest in ~1s; reserve `rustdoc --no-deps` for
> > final pre-commit verification across all crates."
>
> Command:
> `aide reflect accept 01JF… --supersedes=01JD… \
>   --content="When iterating on a single doctest…"`

Then **wait**. The user might say:
- "yes" → run the command verbatim
- "yes but tweak the wording to …" → adjust `--content=` and re-present
- "accept but don't supersede" → drop `--supersedes`
- "reject — that's not actually a pattern, I asked you to repeat for testing"
  → run reject with that reason
- "leave them open, I'll review later" → do nothing, you're done

If the evidence doesn't support a meaningful rewrite — e.g. the events are
a test trigger, a one-off, a data artifact — **recommend reject**. A
proposal with no useful synthesis is noise; promoting it pollutes the
memory store.

### 8. Execute the user's decision via the CLI

Only after the user has chosen — writes are CLI-only per aide convention:

```bash
# Accept with rewritten content (the default — see step 7):
./.aide/bin/aide reflect accept <proposal-id> --content="<your synthesised memory>"

# Accept with rewritten content AND supersession:
./.aide/bin/aide reflect accept <proposal-id> \
  --content="<your synthesis>" \
  --supersedes=<mem-id1>,<mem-id2>

# Accept verbatim (the [DRAFT…] template lands as the memory — almost
# never what you want, included only for completeness):
./.aide/bin/aide reflect accept <proposal-id>

# Reject (with reason for the audit trail):
./.aide/bin/aide reflect reject <proposal-id> --reason="not useful"
```

Supersession unions two sources:

1. **Structural (auto)** — instinct memories sharing the same
   `instinct_key:*` tag (cheap dedup; same Bash command for repetition,
   same file path for convergence).
2. **Semantic (via `--supersedes`)** — IDs from step 6. Works for any
   memory including manually-set ones with no instinct tags.

Each superseded record gets `superseded` + `superseded_by:<new-id>` tags;
the new memory gets `supersedes:<csv>` pointing back. Superseded records
stay in the bucket for audit but are filtered out of default queries.

## Inspecting evidence

```
mcp__plugin_aide_aide__instinct_inspect { "id": "<ulid>" }
```

The `evidence.snapshot` array contains the observe events that triggered the
pattern.

## Opt-in toggle for the Stop hook

```bash
export AIDE_REFLECT=1      # also: true, on, yes — any truthy value
```

When unset or set to a falsy value (`0`, `false`, `off`, `no`), the Stop hook
is a no-op. This skill still works regardless — it's manually invoked, not
gated by the env var.

## Shape catalogue

- **repetition** — Bash commands run > N times in a session, suggesting the
  agent forgot it already had the answer. Pure counting, no LLM input needed.
- **convergence** — `Edit A` → user corrective marker → `Edit B` on the same
  file → optional positive signal. Marker-based by default, upgrades to
  LLM-classified when intent labels are provided via step 3.
- **friction** — the same tool failing repeatedly on the same target (a Bash
  command that keeps erroring, an Edit that won't apply to a file). Gathered
  from the observe `Error` field; the lesson worth keeping is usually the
  *fix*, which you supply by reading the evidence. `RequiresLLM` — never
  auto-fires in the Stop hook.

Detectors declare a `RequiresLLM` capability — two tiers:

- **Deterministic tier** (`RequiresLLM=false`: repetition) runs automatically
  in the Stop hook. Must be high-precision; nothing reviews it before it lands
  as a proposal.
- **LLM tier** (`RequiresLLM=true`: convergence, friction) runs only in this
  reviewed pass, when the skill passes `--llm` (step 3/4). Higher recall is
  fine — you judge each proposal before anything is promoted.
