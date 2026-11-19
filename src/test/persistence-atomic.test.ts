import { beforeEach, describe, expect, it, vi } from "vitest";
vi.mock("../core/aide-client.js", () => ({
  getState: vi.fn(),
  setState: vi.fn(),
  incrementState: vi.fn(),
}));
vi.mock("../core/todo-checker.js", () => ({
  fetchTodosFromAide: () => [],
  checkTodos: () => ({
    hasIncomplete: true,
    incompleteCount: 1,
    message: "one task remains",
  }),
}));
vi.mock("../lib/logger.js", () => ({ debug: vi.fn() }));
import { getState, setState, incrementState } from "../core/aide-client.js";
import { checkPersistence } from "../core/persistence-logic.js";
import { MAX_PERSISTENCE_ITERATIONS } from "../core/types.js";
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getState).mockReturnValue("autopilot");
});
describe("atomic persistence counter", () => {
  it("uses the stored atomic result in the session namespace", () => {
    vi.mocked(incrementState).mockReturnValue(7);
    expect(
      checkPersistence("aide", "/project", undefined, "session-1")?.reason,
    ).toContain("iteration 7/");
    expect(incrementState).toHaveBeenCalledWith(
      "aide",
      "/project",
      "autopilot_iterations",
      "session-1",
    );
    expect(getState).toHaveBeenCalledTimes(1);
    expect(setState).not.toHaveBeenCalled();
  });
  it("allows stop if atomic counting is unavailable, without a racy fallback", () => {
    vi.mocked(incrementState).mockReturnValue(null);
    expect(
      checkPersistence("aide", "/project", undefined, "session-1"),
    ).toBeNull();
    expect(getState).toHaveBeenCalledTimes(1);
    expect(setState).not.toHaveBeenCalled();
  });
  it("releases the mode at the stored limit", () => {
    vi.mocked(incrementState).mockReturnValue(MAX_PERSISTENCE_ITERATIONS + 1);
    expect(
      checkPersistence("aide", "/project", undefined, "session-1"),
    ).toBeNull();
    expect(setState).toHaveBeenCalledWith("aide", "/project", "mode", "");
  });
});
