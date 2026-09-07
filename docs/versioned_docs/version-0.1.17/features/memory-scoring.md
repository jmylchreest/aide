---
sidebar_position: 2
---

# Memory Scoring

Memory scoring controls which memories are injected into session context. When more memories exist than the injection limit allows, scoring ensures the most relevant ones are included.

## How It Works

Each memory receives a deterministic score in the range [0.0, 1.0] computed from four weighted components:

```
score = category * 0.50 + recency * 0.25 + provenance * 0.15 + access * 0.10
```

Memories are sorted by score (highest first) before injection. When scores are equal, ULID order (chronological) is the tiebreaker.

## Components

### Category (50%)

Base importance by memory category. Higher-priority categories surface first.

| Category    | Score | Description                     |
| ----------- | ----- | ------------------------------- |
| User prefs  | 1.00  | `learning` + `scope:global` tag |
| `abandoned` | 0.90  | Failed approaches to avoid      |
| `blocker`   | 0.85  | Blockers encountered            |
| `issue`     | 0.80  | Issues found                    |
| `gotcha`    | 0.75  | Pitfalls to watch for           |
| `discovery` | 0.70  | New findings                    |
| `decision`  | 0.65  | Architectural choices           |
| `learning`  | 0.60  | General learnings               |
| `pattern`   | 0.60  | Reusable patterns               |
| `session`   | 0.40  | Session context                 |

Unknown categories receive a default score of 0.50.

### Recency (25%)

Exponential decay with a 30-day half-life. A memory created today scores 1.0; a memory created 30 days ago scores 0.5; 60 days ago scores 0.25, and so on.

```
recency = 2^(-age_days / 30)
```

### Provenance (15%)

Additive boosts from provenance tags. Multiple boosts stack (clamped to 1.0).

| Tag                 | Boost | Meaning                            |
| ------------------- | ----- | ---------------------------------- |
| `source:user`       | +0.20 | User explicitly stated this        |
| `verified:true`     | +0.10 | Verified against codebase          |
| `source:discovered` | +0.05 | Agent discovered by examining code |

### Access (10%)

Log-scaled based on how many times the memory has been retrieved via `memory_search`. More-accessed memories score higher.

```
access = log10(1 + access_count) / log10(10)
```

## Manual Override

If a memory's `Priority` field is set to a non-zero value, it replaces the computed score entirely. This allows manual pinning of important memories.

## Environment Variables

| Variable                         | Effect                                                 |
| -------------------------------- | ------------------------------------------------------ |
| `AIDE_MEMORY_SCORING_DISABLED=1` | Disable scoring entirely; use chronological ULID order |
| `AIDE_MEMORY_DECAY_DISABLED=1`   | Scoring is active but recency factor is always 1.0     |

These are useful for debugging or when you prefer time-ordered injection.

## Injection Limits

| Scope            | Limit | Description                                 |
| ---------------- | ----- | ------------------------------------------- |
| Global memories  | 100   | Memories tagged `scope:global`              |
| Project memories | 30    | Memories tagged `project:<name>`            |
| Recent sessions  | 2     | Most recent session groups                  |
| Decisions        | All   | Latest decision per topic (always injected) |

When project memories exceed the limit, the highest-scoring ones are kept and an overflow flag is set.
