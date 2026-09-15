# Offline experiment harness

Everything in `harness/`, `tests/`, `package.json` and `bunfig.toml` is authored
support for this experiment. Original production files under `src/` and `aide/`
are copied unchanged from the pinned commit before a participant edits them.

`preload.ts` uses Bun module mocks to replace binary execution, host input,
logger output, initialization and summary-storage dependencies. It supplies
inert mocks for the unused `which` and `smol-toml` packages. It does not replace
the model usage parser, scanner, writer, recorder or host event routing. The
mocked `execFileSync` captures transport options and returns configured CLI
output. This tests the actual writer without needing the Go binary or network.

Run only `bun test tests` for the visible suite. Original `src/test` Vitest
files are included as navigation evidence and are not wired to the Bun runner.
The Bun suite validates runtime behavior; it does not perform TypeScript type
checking. Do not modify these authored support files for the exercise.
