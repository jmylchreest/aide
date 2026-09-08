import type { ModelUsage } from "../../lib/types";

const labels = {
  input_tokens: "Input (includes cache)",
  output_tokens: "Output (includes reasoning)",
  uncached_input_tokens: "Uncached input",
  cache_read_input_tokens: "Cache read input",
  cache_write_input_tokens: "Cache write input",
  reasoning_output_tokens: "Reasoning output",
  reported_output_tokens: "Reported output (overlap unknown)",
  total_tokens: "Reported total",
};
const count = (n: unknown): n is number =>
  typeof n === "number" && Number.isSafeInteger(n) && n >= 0;
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object" && !Array.isArray(v);

/** Do not render missing, unsupported or inconsistent evidence as zero. */
export function supportedModelUsage(value: unknown): ModelUsage | undefined {
  if (
    !object(value) ||
    value.version !== 1 ||
    ![value.observations, value.conflicts, value.invalid].every(count) ||
    !Array.isArray(value.by_source)
  )
    return;
  const identities = new Set<string>();
  for (const row of value.by_source) {
    if (
      !object(row) ||
      typeof row.host !== "string" ||
      !row.host ||
      typeof row.source !== "string" ||
      !row.source ||
      (row.model !== undefined && typeof row.model !== "string") ||
      (row.provider !== undefined && typeof row.provider !== "string") ||
      !count(row.observations) ||
      !count(row.source_timed) ||
      !count(row.observed_timed) ||
      row.observations === 0 ||
      row.source_timed + row.observed_timed !== row.observations ||
      !object(row.counters)
    )
      return;
    const identity = JSON.stringify([
      row.host,
      row.source,
      row.model ?? "",
      row.provider ?? "",
    ]);
    if (identities.has(identity)) return;
    identities.add(identity);
    for (const q of Object.values(row.counters)) {
      if (
        !object(q) ||
        !count(q.tokens) ||
        !count(q.observations) ||
        q.observations === 0 ||
        q.observations > row.observations
      )
        return;
    }
  }
  const report = value as unknown as ModelUsage;
  if (
    report.by_source.reduce((sum, row) => sum + row.observations, 0) !==
    report.observations
  )
    return;
  return report;
}

export function ModelUsageAccounting({
  report,
}: {
  report?: ModelUsage | null;
}) {
  const usage = supportedModelUsage(report);
  return (
    <section aria-label="Host-reported model usage" className="mt-5 mb-5">
      <h3 className="text-xs font-semibold text-aide-text mb-2">
        Host-reported model usage
      </h3>
      {!usage ? (
        <p className="text-xs text-aide-text-muted">
          Model usage accounting is unavailable from this server.
        </p>
      ) : (
        <>
          <p className="text-[11px] text-aide-text-muted mb-3">
            {usage.observations
              ? `${usage.observations.toLocaleString()} unique observations · partial coverage`
              : "No model usage observations in this selection; usage is unknown."}
            {` · Conflicting: ${usage.conflicts.toLocaleString()} · Invalid: ${usage.invalid.toLocaleString()}`}
          </p>
          <div className="grid grid-cols-1 xl:grid-cols-2 gap-3">
            {usage.by_source.map((row) => (
              <div
                key={JSON.stringify([
                  row.host,
                  row.source,
                  row.model,
                  row.provider,
                ])}
                className="border border-aide-border rounded-md overflow-hidden min-w-0"
              >
                <div className="bg-aide-surface px-3 py-2 text-xs text-aide-text break-words">
                  {row.host}
                  {row.model ? ` · ${row.model}` : " · Model unknown"}
                  {row.provider && (
                    <span className="text-aide-text-dim">
                      {" "}
                      · {row.provider}
                    </span>
                  )}
                </div>
                <table className="w-full text-left text-[11px]">
                  <thead className="text-aide-text-dim">
                    <tr>
                      <th className="px-3 py-2 font-medium">Counter</th>
                      <th className="px-2 py-2 font-medium">Tokens</th>
                      <th className="px-2 py-2 font-medium">Coverage</th>
                    </tr>
                  </thead>
                  <tbody>
                    {Object.entries(labels)
                      .filter(
                        ([key]) =>
                          key !== "reported_output_tokens" || row.counters[key],
                      )
                      .map(([key, label]) => {
                        const q = row.counters[key];
                        return (
                          <tr
                            key={key}
                            className="border-t border-aide-border text-aide-text-muted"
                          >
                            <td className="px-3 py-2">{label}</td>
                            <td className="px-2 py-2 tabular-nums">
                              {q ? q.tokens.toLocaleString() : "Unknown"}
                            </td>
                            <td className="px-2 py-2">
                              {q
                                ? `${q.observations}/${row.observations} observations`
                                : "Unavailable"}
                            </td>
                          </tr>
                        );
                      })}
                  </tbody>
                </table>
                <details className="px-3 py-2 text-[11px] text-aide-text-dim border-t border-aide-border break-words">
                  <summary className="cursor-pointer">
                    Source and timing
                  </summary>
                  <p className="mt-2">{row.source}</p>
                  <p>
                    Source time: {row.source_timed.toLocaleString()} ·
                    Observation time: {row.observed_timed.toLocaleString()}
                  </p>
                </details>
              </div>
            ))}
          </div>
          <details className="text-[11px] text-aide-text-dim mt-3">
            <summary className="cursor-pointer">
              Model usage: measurement and coverage
            </summary>
            <div className="mt-2 space-y-2 border-l border-aide-border pl-3">
              <p>
                These are host-reported counters from captured responses or
                steps, separate from text estimates. Coverage counts available
                fields among recorded observations, not all model calls. Unknown
                fields are never filled with zero; a reported zero may reflect
                host defaults.
              </p>
              <p>
                Input includes cache where normalization is supported. Output
                includes reasoning only where that relationship is established.
                Cache and reasoning rows overlap those totals; do not add the
                rows. OpenCode reported output has version-dependent reasoning
                overlap and remains separate.
              </p>
              <p>
                Claude transcript output can be a response-start placeholder and
                is omitted. Transcript collection reads a bounded recent window
                at Stop and can lag the current turn. OpenCode records observed
                step-finish events. Unsupported records, earlier history and
                unseen subagents may be missing.
              </p>
              <p>
                Repeated identities count once. Conflicting and invalid evidence
                is excluded. Time filters use source timestamps when available
                and first observation time otherwise. These counters do not
                establish billed cost, savings or task quality.
              </p>
            </div>
          </details>
        </>
      )}
    </section>
  );
}
