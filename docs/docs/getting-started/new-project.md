# Getting Started on a New Project

For a greenfield project, use AIDE to set up guardrails and architectural foundations before writing code. This pays dividends as the codebase grows.

## Step 1: Set Guardrails and Record Decisions

Use the decide skill to establish coding standards, architectural patterns, and technology choices as formal decisions. Each one persists across sessions and is automatically enforced — every future session and swarm agent sees them.

You can cover a lot of ground in a single prompt — the decide skill will work through each topic in turn, recording each as a separate decision:

```
Help me decide on the coding standards, error handling strategy, testing approach,
and architecture patterns for this project. I want to enforce SOLID, DRY, Clean Code,
and idiomatic language best practices.
```

The decide skill runs a structured interview for each topic: it explores options, weighs trade-offs, makes a recommendation, and records the decision with rationale once you confirm. A single conversation might produce decisions for:

- **Coding standards** — SOLID, DRY, Clean Code, composition over inheritance, test coverage expectations
- **Language idioms** — Go: accept interfaces, return structs. TypeScript: no `any`, discriminated unions. Python: type hints, dataclasses.
- **Architecture** — Module boundaries, dependency direction, I/O separation
- **Error handling** — Structured errors, fail-fast boundaries, no swallowed errors
- **Testing strategy** — Framework choice, coverage targets, TDD vs test-after

Each is stored as its own decision topic (e.g., `coding-standards`, `error-handling`, `testing-strategy`) so they can be reviewed and updated independently. Run more decision sessions later for database, auth, deployment, or anything else your project needs.

These decisions are injected into every session context automatically. You don't need to repeat yourself.

## Step 2: Plan and Design

Use the design skill to think through architecture before writing code:

```
Design the project structure for a REST API with auth, billing, and notifications.
```

The design skill creates technical specs with interfaces and acceptance criteria:

```
Design the authentication system with JWT tokens and refresh flow.
```

This produces a spec with interfaces, data models, and decisions — not just code. It gives every future session (and swarm agent) a shared understanding of what to build. You can run multiple design sessions to cover different subsystems.

## Step 3: Build with TDD and Verification

Use the implementation workflow that enforces quality:

```
Write tests for the auth module, then implement to make them pass.
```

Or use the full pipeline for larger work:

```
plan swarm for auth, billing, and notification features  # Decompose into stories
swarm 3 implement the planned stories                    # Parallel agents, each with TDD pipeline
verify the implementation                                # Full QA: tests, lint, types, build
```

## Step 4: Keep Quality Visible

As the codebase grows, use findings and patterns to catch drift:

```
Run a code health check and assess the findings.
```

This runs static analysis (complexity, coupling, duplication, secrets), then triages findings — accepting noise and flagging genuine issues. High-churn + high-complexity files are your top refactoring targets.

## Key Principle

Front-load decisions and standards into AIDE's persistent stores (decisions and memories) so every future session — whether you or a swarm of parallel agents — starts with the right context and follows the same rules.
