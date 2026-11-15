export type DecisionApplicability =
  | string
  | {
      kind: "project" | "context";
      any_of?: Array<{
        language?: string;
        framework?: string;
        tools?: string[];
        component?: string;
        concern?: string;
        paths?: string[];
      }>;
    };

export interface ContextualDecision {
  topic: string;
  value: string;
  rationale?: string;
  applicability?: DecisionApplicability;
  supersedes?: string[];
}

export const DECISION_APPLICABILITY_GUIDANCE =
  "Decisions without an applicability label are always available and have no additional applicability restriction; respect any limits in their wording. Explicit applicability, including repository-relative path patterns, is guidance for the agent to interpret against the task, not automatic file filtering. Within a selector, fields apply together and paths are alternatives; OR separates alternative selectors. Precedence and explicit supersession apply only where the decisions are relevant. More specific applicability alone does not override another decision.";

export function applicabilityLabel(a?: DecisionApplicability): string {
  if (!a) return "";
  if (typeof a === "string") return a.trim().replace(/\s+/g, " ");
  if (a.kind === "project") return "Project-wide";
  return (a.any_of ?? [])
    .map((s) =>
      [
        s.language,
        s.framework,
        ...(s.tools ?? []),
        s.component,
        s.concern,
        s.paths?.length
          ? `paths: ${s.paths.length === 1 ? s.paths[0] : `(${s.paths.join(" OR ")})`}`
          : undefined,
      ]
        .filter(Boolean)
        .join(" + "),
    )
    .join(" OR ");
}

// Both session and subagent injection retain all rules: context selection is
// task-dependent, and a narrower rule only supersedes its named targets locally.
export function renderContextualDecision(d: ContextualDecision): string {
  const supersedes = d.supersedes?.length
    ? ` Supersedes ${d.supersedes.join(", ")} only within this applicability.`
    : "";
  const label = applicabilityLabel(d.applicability);
  return `${label ? `[${label}] ` : ""}**${d.topic}**: ${d.value}${d.rationale ? ` (${d.rationale})` : ""}${supersedes}`;
}
