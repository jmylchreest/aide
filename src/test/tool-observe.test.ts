import { describe, it, expect } from "vitest";
import { toolFailureText } from "../core/tool-observe.js";

describe("toolFailureText", () => {
  it("returns empty for a successful call", () => {
    expect(toolFailureText(true, "all good")).toBe("");
    expect(toolFailureText(undefined, { output: "fine" })).toBe("");
  });

  it("detects explicit success=false and returns the output as error text", () => {
    expect(toolFailureText(false, "boom: command not found")).toBe(
      "boom: command not found",
    );
  });

  it("detects is_error on the tool_response object", () => {
    expect(
      toolFailureText(undefined, { is_error: true, content: "String not found" }),
    ).toBe("String not found");
  });

  it("detects an error string field and a failed status", () => {
    expect(toolFailureText(undefined, { error: "ENOENT" })).toBe("ENOENT");
    expect(toolFailureText(undefined, { status: "failed", text: "nope" })).toBe(
      "nope",
    );
  });

  it("falls back to a marker when failed but no text is recoverable", () => {
    expect(toolFailureText(false, undefined)).toBe("tool reported failure");
    expect(toolFailureText(undefined, { is_error: true })).toBe(
      "tool reported failure",
    );
  });

  it("truncates very long error text", () => {
    const long = "x".repeat(1000);
    expect(toolFailureText(false, long).length).toBe(500);
  });
});
