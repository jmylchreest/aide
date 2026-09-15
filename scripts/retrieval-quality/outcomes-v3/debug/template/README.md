# Context-pruning debugging fixture

This standalone extraction contains the real aide tracker, dedup strategy, recovery finalizer, and their types. The tracker is scoped to dedup only; supersede and purge are omitted. It uses Bun and Node built-ins, without installing dependencies or contacting services. One seeded regression is present.

Run `rtk proxy bun test ./reproduce.test.ts`. The source has four files under `src/core/context-pruning/`.
