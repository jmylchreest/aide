import { describe, it, expect } from "vitest";
import {
  applicabilityLabel,
  renderContextualDecision,
} from "../core/decision-context.js";

describe("decision applicability", () => {
  it("keeps unspecified and project-wide distinct", () => {
    expect(applicabilityLabel()).toBe("Applicability unspecified");
    expect(applicabilityLabel({ kind: "project" })).toBe("Project-wide");
  });
  it("renders conjunctions, alternatives and contextual supersession", () => {
    const text = renderContextualDecision({
      topic: "settings",
      value: "Use framework settings",
      applicability: {
        kind: "context",
        any_of: [
          { language: "python", framework: "django" },
          { tools: ["opentofu", "github-actions"] },
        ],
      },
      supersedes: ["base-settings"],
    });
    expect(text).toContain("[python + django OR opentofu + github-actions]");
    expect(text).toContain(
      "Supersedes base-settings only within this applicability.",
    );
  });
});
