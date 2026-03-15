---
sidebar_label: Built-in Skills
sidebar_position: 2
title: Built-in Skills
---

# Built-in Skills

AIDE ships with 22 built-in skills covering the full development lifecycle. Each skill is a markdown file that injects structured instructions when triggered.

## Planning & Design

### plan-swarm

Socratic planning interview that decomposes work into validated stories for swarm execution.

|              |                                                                                   |
| ------------ | --------------------------------------------------------------------------------- |
| **Triggers** | `plan swarm`, `plan stories`, `decompose stories`, `scope swarm`, `socratic plan` |
| **Use when** | You have a large feature to break into parallel work items                        |

### decide

Formal decision-making interview that records architectural choices with rationale.

|              |                                                                                                                                    |
| ------------ | ---------------------------------------------------------------------------------------------------------------------------------- |
| **Triggers** | `decide`, `help me decide`, `help me choose`, `how should we`, `what should we use`, `which option`, `trade-offs`, `pros and cons` |
| **Use when** | Choosing between alternatives (frameworks, patterns, architecture)                                                                 |

### design

Technical spec with interfaces, decisions, and acceptance criteria.

|              |                                                                                                                  |
| ------------ | ---------------------------------------------------------------------------------------------------------------- |
| **Triggers** | `design this`, `design the`, `architect this`, `architect the`, `spec this`, `spec the`, `plan this`, `plan the` |
| **Use when** | Creating a technical design before implementation                                                                |

## Development

### test

Creates a test suite with coverage verification following TDD principles.

|              |                                                      |
| ------------ | ---------------------------------------------------- |
| **Triggers** | `write tests`, `add tests`, `test this`, `run tests` |
| **Use when** | Writing tests before or alongside implementation     |

### implement

TDD implementation — makes failing tests pass.

|              |                                                                                        |
| ------------ | -------------------------------------------------------------------------------------- |
| **Triggers** | `implement this`, `implement the`, `make tests pass`, `dev stage`, `development stage` |
| **Use when** | You have failing tests and need to write the implementation                            |

### build-fix

Iteratively fixes build, lint, and type errors until the build is clean.

|              |                                                                                             |
| ------------ | ------------------------------------------------------------------------------------------- |
| **Triggers** | `fix build`, `fix the build`, `build is broken`, `lint errors`, `type errors`, `fix errors` |
| **Use when** | Your build is broken and you need systematic error fixing                                   |

## Quality & Review

### verify

Full QA validation — tests, lint, types, and debug artifact checks.

|              |                                                                                     |
| ------------ | ----------------------------------------------------------------------------------- |
| **Triggers** | `verify this`, `verify the`, `qa check`, `validate`, `verification`, `verify stage` |
| **Use when** | Doing a final quality check before shipping                                         |

### review

Security-focused code review with audit capabilities.

|              |                                                                            |
| ------------ | -------------------------------------------------------------------------- |
| **Triggers** | `review this`, `review the`, `code review`, `security audit`, `audit this` |
| **Use when** | Reviewing code changes or PRs                                              |

### patterns

Analyzes codebase patterns and surfaces static analysis findings.

|              |                                                                                                                                                     |
| ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Triggers** | `find patterns`, `anti-patterns`, `code smells`, `complexity`, `duplicated code`, `clones`, `secrets`, `coupling`, `static analysis`, `code health` |
| **Use when** | Assessing overall code health or finding specific issues                                                                                            |

### assess-findings

Triages static analysis findings — reads actual code, accepts noise, keeps genuine issues.

|              |                                                                                                                                                             |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Triggers** | `assess findings`, `analyse findings`, `analyze findings`, `triage findings`, `review findings`, `accept findings`, `dismiss findings`, `clean up findings` |
| **Use when** | You have findings from `patterns` and need to separate signal from noise                                                                                    |

## Execution Modes

### swarm

Spawns N parallel agents with SDLC pipeline per story using git worktrees.

|              |                                                              |
| ------------ | ------------------------------------------------------------ |
| **Triggers** | `swarm`, `parallel agents`, `spawn agents`, `multi-agent`    |
| **Use when** | You have multiple independent stories to execute in parallel |

### autopilot

Full autonomous execution — keeps working until all tasks are verified complete.

|              |                                                                                                 |
| ------------ | ----------------------------------------------------------------------------------------------- |
| **Triggers** | `autopilot`, `full auto`, `autonomous`, `keep going`, `finish everything`, `run until complete` |
| **Use when** | You need the AI to keep going until a task is truly done                                        |

### debug

Systematic debugging with hypothesis testing.

|              |                                                                             |
| ------------ | --------------------------------------------------------------------------- |
| **Triggers** | `debug this`, `debug mode`, `fix this bug`, `trace the bug`, `find the bug` |
| **Use when** | Tracking down a specific bug with a structured approach                     |

### perf

Performance profiling and optimization workflow.

|              |                                                                          |
| ------------ | ------------------------------------------------------------------------ |
| **Triggers** | `optimize`, `performance`, `slow`, `too slow`, `speed up`, `make faster` |
| **Use when** | Diagnosing and fixing performance issues                                 |

## Knowledge Management

### memorise

Stores information for future sessions as persistent memories.

|              |                                                                                                                                                |
| ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| **Triggers** | `remember this`, `remember that`, `dont forget`, `memorise`, `note that`, `note this`, `save this`, `store this`, `for future`, `keep in mind` |
| **Use when** | You want AIDE to remember preferences, patterns, or context                                                                                    |

### recall

Searches memories and decisions to answer questions about past context.

|              |                                                                                                                     |
| ------------ | ------------------------------------------------------------------------------------------------------------------- |
| **Triggers** | `what did we`, `what was`, `what is the`, `do you remember`, `did we decide`, `recall`, `from memory`, `previously` |
| **Use when** | You need to look up past decisions or stored knowledge                                                              |

### forget

Soft-deletes or hard-deletes outdated memories.

|              |                                                                                                                                                                                                                                                                   |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Triggers** | `forget this`, `forget that`, `forget memory`, `remove memory`, `delete memory`, `outdated memory`, `wrong memory`, `incorrect memory`, `supersede memory`, `obsolete`, `no longer true`, `no longer relevant`, `not true anymore`, `that was wrong`, `disregard` |
| **Use when** | A stored memory is outdated or incorrect                                                                                                                                                                                                                          |

## Code Navigation

### code-search

Searches code symbols, finds function calls, and analyzes the codebase.

|              |                                                                                                                                                                                |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Triggers** | `find function`, `where is`, `who calls`, `find class`, `find method`, `search code`, `code search`, `find symbol`, `call sites`, `references to`, `what calls`, `show me the` |
| **Use when** | Navigating unfamiliar code or tracing call chains                                                                                                                              |

### git

Expert git operations and worktree management.

|              |                                                                |
| ------------ | -------------------------------------------------------------- |
| **Triggers** | `worktree`, `git worktree`, `create branch`, `manage branches` |
| **Use when** | Complex git operations beyond basic commits                    |

### worktree-resolve

Intelligently merges worktrees with conflict resolution after swarm completion.

|              |                                                                         |
| ------------ | ----------------------------------------------------------------------- |
| **Triggers** | `resolve worktrees`, `merge worktrees`, `cleanup swarm`, `finish swarm` |
| **Use when** | All swarm agents have completed and you need to merge branches          |

## Documentation

### docs

Updates documentation to match implementation.

|              |                                                                                       |
| ------------ | ------------------------------------------------------------------------------------- |
| **Triggers** | `update docs`, `update documentation`, `document this`, `docs stage`, `documentation` |
| **Use when** | Documentation needs to be updated after code changes                                  |

## Utility

### context-usage

Analyzes current session context and token usage (OpenCode only).

|              |                                                                                                                                  |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------- |
| **Triggers** | `context usage`, `token usage`, `session stats`, `how much context`, `context budget`, `how big is this session`, `session size` |
| **Use when** | You want to understand how much context your session is using                                                                    |
