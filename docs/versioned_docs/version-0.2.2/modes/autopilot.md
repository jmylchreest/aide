---
sidebar_label: Autopilot Mode
sidebar_position: 3
title: Autopilot Mode
---

# Autopilot Mode

Autopilot mode makes the AI keep going until the task is **verified complete**. No stopping early, no "I've made progress", no partial results.

## Activation

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

1. The AI executes the task
2. After each step, it checks whether the task is truly complete
3. If not complete, it continues (no asking for permission)
4. Verification happens through actual commands (running tests, checking build output, etc.)
5. Only stops when all tasks in the todo list are complete
6. Safety cap: auto-releases after 20 iterations

### Platform Behavior

| Platform        | Mechanism                                                                |
| --------------- | ------------------------------------------------------------------------ |
| **Claude Code** | Stop-blocking — prevents the AI from ending the conversation early       |
| **OpenCode**    | Re-prompting — `session.prompt()` is called on idle to keep the AI going |

## When to Use

Autopilot mode is ideal for:

- **Fixing all test failures** — "autopilot fix all failing tests"
- **Complete refactors** — "autopilot refactor all error handling to use Result types"
- **Build fixes** — "autopilot make the build pass"
- **Migration tasks** — "autopilot migrate all components to the new API"

## When NOT to Use

Avoid autopilot mode for:

- **Exploratory tasks** — "investigate why X happens" (no clear completion criteria)
- **Design work** — Use the `design` skill instead
- **Tasks requiring human judgment** — Autopilot will keep going based on automated checks
- **Parallel work** — Use `swarm` mode instead for decomposed parallel stories

## Combining with Other Skills

Autopilot mode works well in combination:

```
autopilot fix the build errors       # autopilot + build-fix behavior
autopilot make all tests pass        # autopilot + implement behavior
```

In swarm mode, individual agents can run in autopilot mode to ensure each story is fully complete before signaling `completion`.
