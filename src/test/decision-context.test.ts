import { describe, it, expect } from "vitest";
import {
  applicabilityLabel,
  DECISION_APPLICABILITY_GUIDANCE,
  renderContextualDecision,
} from "../core/decision-context.js";

describe("decision applicability", () => {
  it("leaves blank decisions in their original format", () => {
    expect(applicabilityLabel()).toBe("");
    expect(applicabilityLabel({ kind: "project" })).toBe("Project-wide");
    for (const applicability of [undefined, "", " \n "]) {
      expect(
        renderContextualDecision({
          topic: "privacy",
          value: "Never log credentials",
          applicability,
        }),
      ).toBe("**privacy**: Never log credentials");
    }
    expect(DECISION_APPLICABILITY_GUIDANCE).toContain(
      "no additional applicability restriction",
    );
    expect(DECISION_APPLICABILITY_GUIDANCE).toContain(
      "not automatic file filtering",
    );
  });
  it("renders freeform guidance without changing the decision", () => {
    expect(
      renderContextualDecision({
        topic: "rendering",
        value: "Use SSR",
        rationale: "Public pages",
        applicability: "Only the customer app;\nexclude admin.",
      }),
    ).toBe(
      "[Only the customer app; exclude admin.] **rendering**: Use SSR (Public pages)",
    );
  });
  it("keeps path alternatives grouped within a selector", () => {
    expect(
      applicabilityLabel({
        kind: "context",
        any_of: [
          {
            language: "go",
            paths: ["services/billing/**", "services/payments/**"],
          },
          { paths: ["apps/admin/**"] },
        ],
      }),
    ).toBe(
      "go + paths: (services/billing/** OR services/payments/**) OR paths: apps/admin/**",
    );
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
