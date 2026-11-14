export interface DecisionApplicability {
  kind: "project" | "context";
  any_of?: Array<{
    language?: string;
    framework?: string;
    tools?: string[];
    component?: string;
    concern?: string;
  }>;
}

export interface ContextualDecision {
  topic: string;
  value: string;
  rationale?: string;
  applicability?: DecisionApplicability;
  supersedes?: string[];
}

export const DECISION_APPLICABILITY_GUIDANCE =
  "Apply each decision only within its stated applicability. Unspecified applicability is not automatically project-wide; consult the decision details when unclear. Precedence and explicit supersession apply only where the decisions are relevant. More specific applicability alone does not override another decision.";

export function applicabilityLabel(a?: DecisionApplicability): string {
  if (!a) return "Applicability unspecified";
  if (a.kind === "project") return "Project-wide";
  return (a.any_of ?? [])
    .map((s) =>
      [s.language, s.framework, ...(s.tools ?? []), s.component, s.concern]
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
  return `[${applicabilityLabel(d.applicability)}] **${d.topic}**: ${d.value}${d.rationale ? ` (${d.rationale})` : ""}${supersedes}`;
}
