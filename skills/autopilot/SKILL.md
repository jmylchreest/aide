---
name: autopilot
description: Full autonomous execution - keeps working until all tasks are verified complete
triggers:
  - autopilot
  - full auto
  - autonomous
  - keep going
  - finish everything
  - run until complete
---

# Autopilot Mode

**Recommended model tier:** smart (opus) - this mode handles complex multi-step tasks

Full autonomous execution mode. The agent keeps working until all tasks in the todo list are verified complete. No stopping early, no partial results.

## Activation

Type naturally:

```
autopilot build me a web app
autopilot fix all failing tests
autopilot refactor the auth module
```

Or explicitly set the mode:

```bash
aide state set mode autopilot
```

## How It Works

When autopilot mode is active:

1. The persistence hook intercepts stop signals
2. If incomplete tasks exist in the todo list, the agent is re-prompted to continue
3. The agent keeps working through its task list
4. Auto-releases when ALL tasks are complete (terminal state)
5. Safety cap: releases after 20 iterations even if tasks remain

### Platform Behavior

| Platform        | Mechanism                                                          |
| --------------- | ------------------------------------------------------------------ |
| **Claude Code** | Stop-blocking — prevents the AI from ending the conversation early |
| **OpenCode**    | Re-prompting — `session.prompt()` is called on idle to keep going  |

### Task Tracking

Autopilot relies on the todo list to determine completeness:

- **Has incomplete tasks** → Block stop, continue working
- **All tasks complete** → Auto-release, allow stop
- **No tasks exist** → Generic reinforcement (verify your work)

## When to Use

Autopilot mode is ideal for:

- **Multi-step implementations** — "autopilot implement the user dashboard"
- **Fixing all test failures** — "autopilot fix all failing tests"
- **Complete refactors** — "autopilot refactor all error handling to use Result types"
- **Migration tasks** — "autopilot migrate all components to the new API"
- **Build fixes** — "autopilot make the build pass"

## When NOT to Use

Avoid autopilot for:

- **Exploratory tasks** — "investigate why X happens" (no clear completion criteria)
- **Design work** — Use the `design` skill instead (needs human input)
- **Tasks requiring human judgment** — Autopilot continues based on automated checks
- **Parallel work** — Use `swarm` mode instead for decomposed parallel stories

## Combining with Skills

Autopilot works well with any skill:

```
autopilot fix the build errors       # autopilot + build-fix behavior
autopilot make all tests pass        # autopilot + implement behavior
autopilot debug why login fails      # autopilot + debug behavior
```

## Deactivation

To stop autopilot mode early:

```bash
aide state set mode ""
```

Or type "stop" — the keyword detector clears the active mode.

## Instructions

When autopilot mode is activated:

1. **Create a comprehensive todo list** — Break the task into specific, actionable items
2. **Work through items sequentially** — Mark each as `in_progress` then `completed`
3. **Verify each step** — Run tests, check builds, confirm behavior
4. **Do not stop until all items are complete** — The persistence hook will block premature stops
5. **If stuck on an item after 3 attempts**, record a blocker memory and move to the next item:
   ```bash
   ./.aide/bin/aide memory add --category=blocker --tags=project:<name>,session:<id>,source:discovered "Blocked on <task>: <reason>"
   ```

**Binary location:** The aide binary is at `.aide/bin/aide`. If it's on your `$PATH`, you can use `aide` directly.
