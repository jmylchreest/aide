---
sidebar_label: Ralph Mode
sidebar_position: 3
title: Ralph Mode
---

# Ralph Mode

Ralph mode (named after Ralph Wiggum's single-minded persistence) makes the AI keep going until the task is **verified complete**. No stopping early, no "I've made progress", no partial results.

## Activation

```
ralph fix all failing tests
ralph refactor the auth module
ralph until done
```

Or explicitly set the mode:

```bash
aide state set mode ralph
```

## How It Works

When ralph mode is active:

1. The AI executes the task
2. After each step, it checks whether the task is truly complete
3. If not complete, it continues (no asking for permission)
4. Verification happens through actual commands (running tests, checking build output, etc.)
5. Only stops when verification passes

### Platform Behavior

| Platform        | Mechanism                                                                |
| --------------- | ------------------------------------------------------------------------ |
| **Claude Code** | Stop-blocking — prevents the AI from ending the conversation early       |
| **OpenCode**    | Re-prompting — `session.prompt()` is called on idle to keep the AI going |

## When to Use

Ralph mode is ideal for:

- **Fixing all test failures** — "ralph fix all failing tests"
- **Complete refactors** — "ralph refactor all error handling to use Result types"
- **Build fixes** — "ralph make the build pass" (similar to `build-fix` but more persistent)
- **Migration tasks** — "ralph migrate all components to the new API"

## When NOT to Use

Avoid ralph mode for:

- **Exploratory tasks** — "investigate why X happens" (no clear completion criteria)
- **Design work** — Use the `design` skill instead
- **Tasks requiring human judgment** — Ralph will keep going based on automated checks

## Combining with Other Skills

Ralph mode works well in combination:

```
ralph fix the build errors       # ralph + build-fix behavior
ralph make all tests pass        # ralph + implement behavior
```

In swarm mode, individual agents can run in ralph mode to ensure each story is fully complete before signaling `completion`.
