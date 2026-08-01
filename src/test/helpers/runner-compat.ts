/**
 * Shims for the handful of places where `bun test` and `vitest` disagree.
 *
 * The suite is written against vitest (`make test-ts`), but the plugin's own
 * runtime is bun — hooks are executed as `bun src/hooks/*.ts`. Keeping the
 * tests runnable under both means the runtime that ships is a runtime the
 * tests can be checked against.
 *
 * Bun's `vi` is a thin compatibility layer over `bun:test`: it provides
 * `fn`/`mock`/`spyOn` and the timer helpers, but not `importActual`,
 * `mocked`, `resetModules`, or `hoisted`.
 */

import { vi } from "vitest";

/**
 * The real `os` module, for use inside a `vi.mock("os", …)` factory.
 *
 * vitest hands the factory an `importOriginal` callback; bun's shim calls the
 * factory with no arguments, and the obvious fallback — `await import("os")`
 * inside the factory — re-enters the mock being defined. `getBuiltinModule`
 * sidesteps module resolution entirely and returns the builtin regardless of
 * any registered mock, so both runners can take the same path.
 *
 * Being synchronous is the point, not an incidental detail: bun deadlocks on
 * an async factory when the test file also imports the module under test
 * statically, because that import is hoisted above the `vi.mock` call. Keep
 * factories built on this helper synchronous.
 *
 * Spread the result so every other `os` export stays real — a module under
 * test that later reaches for `tmpdir()` or `platform()` would otherwise
 * silently receive `undefined`.
 */
export function actualOs(): typeof import("os") {
  return process.getBuiltinModule("os");
}

/**
 * `vi.resetModules()` where it exists, no-op where it does not.
 *
 * Only vitest can drop the module registry between tests. The suites that call
 * this re-import the module under test after changing a fixture path, and the
 * fixture is read lazily at call time rather than cached at import, so bun
 * losing the reset does not change what the assertions see.
 */
export function resetModules(): void {
  (vi as { resetModules?: () => void }).resetModules?.();
}

/** A mocked function, with the assertion surface both runners share. */
type Mocked<T> = T & {
  mockReturnValue: (v: unknown) => void;
  mockImplementation: (impl: (...args: never[]) => unknown) => void;
  mockClear: () => void;
};

/**
 * `vi.mocked()` — a types-only helper in vitest, absent from bun's shim.
 *
 * The cast is all the original does; this exists so call sites read the same
 * under either runner.
 */
export function mocked<T>(fn: T): Mocked<T> {
  return fn as Mocked<T>;
}
