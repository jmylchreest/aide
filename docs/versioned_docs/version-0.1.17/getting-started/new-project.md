# Getting Started on a New Project

For a greenfield project, use AIDE to set up guardrails and architectural foundations before writing code. This pays dividends as the codebase grows.

## Step 1: Bootstrap with Blueprints

Blueprints seed your project with curated best-practice decisions for your language and tooling — error handling conventions, testing strategies, CI/CD patterns, and common pitfalls. One command, dozens of decisions:

```bash
# Seed Go best practices + CI/CD patterns
aide blueprint import go go-github-actions

# Or auto-detect from project markers (go.mod, Cargo.toml, .github/workflows, etc.)
aide blueprint import --detect

# Preview before importing
aide blueprint show go
aide blueprint list
```

This imports decisions covering language idioms, tooling, project structure, concurrency, testing, and more. Each is stored as its own decision topic and injected into every future session automatically.

See [Blueprints](/docs/features/blueprints) for the full list of shipped blueprints and how to create your own.

## Step 2: Refine with Project-Specific Decisions

Blueprints cover general best practices. For choices specific to your project — auth strategy, database schema, API design — use the decide skill:

```
Help me decide on the auth strategy, database schema, and deployment approach
for this project.
```

The decide skill runs a structured interview for each topic: it explores options, weighs trade-offs, makes a recommendation, and records the decision with rationale once you confirm. A single conversation might produce decisions for:

- **Architecture** — Module boundaries, dependency direction, I/O separation
- **Auth** — JWT vs sessions, RBAC vs ABAC, token refresh flow
- **Database** — Schema design, migration strategy, connection pooling
- **Deployment** — Container strategy, environment promotion, rollback approach

Each is stored as its own decision topic (e.g., `auth-strategy`, `database-schema`) so they can be reviewed and updated independently. Run more decision sessions later for anything else your project needs.

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
