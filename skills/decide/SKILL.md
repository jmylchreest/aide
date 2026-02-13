---
name: decide
description: Formal decision-making interview for architectural choices
triggers:
  - decide
  - help me decide
  - help me choose
  - how should we
  - what should we use
  - which option
  - trade-offs
  - pros and cons
allowed-tools: Bash(./.aide/bin/aide decision set *)
---

# Decision Mode

**Recommended model tier:** smart (opus) - this skill requires complex reasoning

Formal decision-making workflow for architectural and technical choices.

## Purpose

When facing a significant technical decision, this workflow guides you through structured analysis to make an informed choice that will be recorded and respected by all future sessions and agents.

## Workflow

### Phase 1: IDENTIFY

Ask the user to clarify:

- What decision needs to be made?
- What is the context? (new system, migration, constraint?)
- What are the requirements? (scale, team, timeline?)
- Are there any hard constraints?

**Example questions:**

- "What technical decision do you need to make?"
- "What problem are you trying to solve?"
- "Are there any constraints I should know about?"

### Phase 2: EXPLORE

Research and propose options:

- List 3-5 viable alternatives
- Include the obvious choices AND less common ones
- For each option, note what it's best suited for

**Output format:**

```markdown
## Options

1. **[Option A]** - Brief description
   - Best for: [use case]

2. **[Option B]** - Brief description
   - Best for: [use case]

3. **[Option C]** - Brief description
   - Best for: [use case]
```

### Phase 3: ANALYZE

For each option, evaluate:

- **Pros**: What are the benefits?
- **Cons**: What are the drawbacks?
- **Fit**: How well does it match the requirements?

**Output format:**

```markdown
## Analysis

| Option   | Pros         | Cons           | Fit                   |
| -------- | ------------ | -------------- | --------------------- |
| Option A | Fast, simple | Limited scale  | Good for MVP          |
| Option B | Scalable     | Complex setup  | Good for growth       |
| Option C | Flexible     | Learning curve | Good if team knows it |
```

Consider:

- Complexity (implementation, maintenance)
- Team familiarity
- Scalability
- Security implications
- Cost (time, money, resources)
- Reversibility (how hard to change later?)

### Phase 4: RECOMMEND

Provide a clear recommendation with rationale:

```markdown
## Recommendation

I recommend **[Option]** because:

1. [Primary reason tied to requirements]
2. [Secondary benefit]
3. [Risk mitigation]

This choice [addresses constraint X] while [enabling future Y].
```

Ask for confirmation:

- "Does this recommendation align with your goals?"
- "Would you like to proceed with this decision?"
- "Any concerns before we record this?"

### Phase 5: CONFIRM

Wait for explicit user approval before recording:

- User says "yes", "go ahead", "let's do it" → proceed to record
- User has concerns → address them, refine recommendation
- User wants different option → adjust and confirm

### Phase 6: RECORD

Store the decision using the `./.aide/bin/aide decision set` CLI command via Bash:

```bash
./.aide/bin/aide decision set "<topic-slug>" "<decision summary>" --rationale="<why this was chosen>"
```

**Binary location:** The aide binary is at `.aide/bin/aide`. If it's on your `$PATH`, you can use `aide` directly.

**Topic naming:**

- Use kebab-case: `auth-strategy`, `database-choice`, `api-design`
- Be specific: `user-auth-method` not just `auth`
- Include scope if relevant: `payments-queue-strategy`

**Verify** the decision was stored:

```bash
./.aide/bin/aide decision get "<topic-slug>"
```

## Example Session

````
User: help me decide how to handle background jobs

Claude: [IDENTIFY]
What kind of background jobs do you need to handle?
- Scheduled tasks (cron-like)?
- Async processing (triggered by events)?
- Long-running workers?

What's your current stack and scale expectations?

User: Async processing for email sending and webhook deliveries.
Stack is Node.js/TypeScript, expecting 10K jobs/day initially.

Claude: [EXPLORE]
## Options

1. **BullMQ (Redis)** - Production-ready Node.js queue
   - Best for: Node.js apps needing reliability + Redis features

2. **AWS SQS + Lambda** - Serverless queue
   - Best for: AWS-native apps, variable load

3. **Database-backed queue** - Simple polling
   - Best for: Low volume, no new infrastructure

4. **RabbitMQ** - Enterprise message broker
   - Best for: Complex routing, multi-language

[ANALYZE]
| Option | Pros | Cons | Fit |
|--------|------|------|-----|
| BullMQ | Native TS, retries, dashboard | Needs Redis | Excellent |
| SQS+Lambda | Serverless, scales | Cold starts, AWS lock-in | Good |
| DB queue | Simple, no new infra | Polling overhead, scale limits | MVP only |
| RabbitMQ | Powerful routing | Overkill, operational burden | Over-engineered |

[RECOMMEND]
I recommend **BullMQ with Redis** because:
1. Native TypeScript support matches your stack
2. Built-in retries, delays, and rate limiting for email/webhooks
3. Bull Board provides visibility without custom tooling
4. 10K/day is well within comfortable range, scales to millions

Redis adds infrastructure but enables other features (caching, sessions).

Do you want to proceed with this decision?

User: yes, let's go with BullMQ

Claude: [RECORD]
[Calls ./.aide/bin/aide decision set via Bash]

```bash
./.aide/bin/aide decision set "background-jobs-queue" "Use BullMQ with Redis for background job processing (emails, webhooks)" --rationale="Native TypeScript support, built-in retries/delays/rate-limiting, Bull Board for visibility, 10K/day well within capacity. Rejected: SQS+Lambda (AWS lock-in, cold starts), DB queue (limited scale), RabbitMQ (over-engineered)"
````

Decision recorded. Future sessions will follow this architectural choice.

```

## Integration

Decisions stored via `./.aide/bin/aide decision set` are:
1. Persisted in the aide memory database
2. Injected into future session contexts at startup
3. Visible to swarm agents (they won't contradict decisions)
4. Queryable via `./.aide/bin/aide decision get`, `./.aide/bin/aide decision list`, or the MCP tools `decision_get`/`decision_list`

## When to Use This Skill

**Good candidates for formal decisions:**
- Authentication/authorization strategy
- Database technology choice
- API design patterns
- State management approach
- Testing strategy
- Deployment architecture
- Third-party service selection

**NOT needed for:**
- Minor implementation details
- Obvious choices with no trade-offs
- Temporary/experimental code
- Personal preferences (use memories instead)

## Changing Decisions

Decisions can be superseded:
1. Run `/aide:decide` again for the same topic
2. New decision replaces old (history preserved)
3. Use `mcp__plugin_aide_aide__decision_history` to see evolution
```
