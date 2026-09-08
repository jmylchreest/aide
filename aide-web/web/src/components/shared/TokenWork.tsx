import type { TokenWork, TokenWorkQuantity } from "../../lib/types";

const counters = [
  "calls",
  "returned",
  "reported_errors",
  "unknown_outcomes",
  "elapsed_ms",
  "measured_durations",
  "missing_durations",
  "unassigned_sessions",
  "missing_payload",
] as const;
const count = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const object = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === "object" && !Array.isArray(value);

function quantity(value: unknown): value is TokenWorkQuantity {
  if (!object(value) || !counters.every((key) => count(value[key]))) return false;
  const text = value.returned_text;
  if (
    !object(text) ||
    ![text.bytes, text.estimated_tokens, text.events].every(count)
  )
    return false;
  const q = value as unknown as TokenWorkQuantity;
  return (
    q.returned + q.reported_errors + q.unknown_outcomes === q.calls &&
    q.measured_durations + q.missing_durations === q.calls &&
    q.returned_text.events + q.missing_payload === q.calls &&
    q.unassigned_sessions <= q.calls &&
    (q.measured_durations > 0 || q.elapsed_ms === 0) &&
    (q.returned_text.events > 0 ||
      (q.returned_text.bytes === 0 && q.returned_text.estimated_tokens === 0))
  );
}

/** Unsupported or malformed evidence must never become a reassuring zero. */
export function supportedTokenWork(value: unknown): TokenWork | undefined {
  if (
    !object(value) ||
    value.version !== 1 ||
    !quantity(value) ||
    !object(value.by_tool)
  )
    return;
  const rows = Object.values(value.by_tool);
  if (!rows.every(quantity)) return;
  const work = value as unknown as TokenWork;
  if (
    !counters.every(
      (key) => rows.reduce((sum, row) => sum + row[key], 0) === work[key],
    )
  )
    return;
  if (
    !["bytes", "estimated_tokens", "events"].every(
      (key) =>
        rows.reduce(
          (sum, row) =>
            sum + row.returned_text[key as keyof typeof row.returned_text],
          0,
        ) === work.returned_text[key as keyof typeof work.returned_text],
    )
  )
    return;
  return work;
}

function elapsed(q: TokenWorkQuantity) {
  if (!q.measured_durations) return "Unknown";
  return `${q.elapsed_ms.toLocaleString()} ms${q.missing_durations ? " (partial)" : ""}`;
}
function bytes(q: TokenWorkQuantity) {
  if (!q.returned_text.events) return "Unknown";
  return `${q.returned_text.bytes.toLocaleString()} bytes${q.missing_payload ? " (partial)" : ""}`;
}

export function TokenWorkDetails({ report }: { report?: TokenWork | null }) {
  const work = supportedTokenWork(report);
  return (
    <section aria-label="Aide work" className="mb-6">
      <h3 className="text-xs font-semibold text-aide-text mb-2">Aide work</h3>
      {!work ? (
        <p className="text-xs text-aide-text-muted">
          Aide work accounting is unavailable from this server.
        </p>
      ) : work.calls === 0 ? (
        <p className="text-xs text-aide-text-muted">
          No recorded Aide operations in this selection.
        </p>
      ) : (
        <>
          <p className="text-[11px] text-aide-text-muted mb-2">
            {work.calls.toLocaleString()} recorded MCP{" "}
            {work.calls === 1 ? "operation" : "operations"} · Reported errors:{" "}
            {work.reported_errors.toLocaleString()}
            {work.unknown_outcomes > 0 &&
              ` · ${work.unknown_outcomes.toLocaleString()} unknown outcomes`}
          </p>
          <div className="overflow-x-auto rounded-md border border-aide-border">
            <table className="w-full text-left text-[11px] min-w-[35rem]">
              <thead className="text-aide-text-dim bg-aide-surface">
                <tr>
                  {[
                    "Operation",
                    "Recorded",
                    "Reported errors",
                    "Elapsed tool time",
                    "Returned text",
                  ].map((label) => (
                    <th key={label} className="px-3 py-2 font-medium">
                      {label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {Object.entries(work.by_tool)
                  .sort(([a], [b]) => a.localeCompare(b))
                  .map(([tool, q]) => (
                    <tr
                      key={tool}
                      className="border-t border-aide-border text-aide-text-muted"
                    >
                      <td className="px-3 py-2 font-mono text-aide-text">
                        {tool}
                      </td>
                      <td className="px-3 py-2">{q.calls.toLocaleString()}</td>
                      <td className="px-3 py-2">
                        {q.reported_errors.toLocaleString()}
                        {q.unknown_outcomes > 0 && (
                          <span className="block text-aide-text-dim">
                            Outcomes unknown:{" "}
                            {q.unknown_outcomes.toLocaleString()}
                          </span>
                        )}
                      </td>
                      <td className="px-3 py-2">{elapsed(q)}</td>
                      <td className="px-3 py-2">{bytes(q)}</td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          <p className="text-[11px] text-aide-text-dim mt-2">
            Operations describe work performed by Aide. Returning without an
            error does not establish task quality.
          </p>
        </>
      )}
    </section>
  );
}

export function TokenWorkAccounting({ report }: { report?: TokenWork | null }) {
  const work = supportedTokenWork(report);
  if (!work) return null;
  return (
    <details className="text-[11px] text-aide-text-dim mt-3">
      <summary className="cursor-pointer">
        Aide work: measurement and coverage
      </summary>
      <div className="mt-2 space-y-2 border-l border-aide-border pl-3">
        <p>
          Only recorded MCP server operations are counted. Host observations are
          excluded to avoid counting the same operation at both boundaries.
          Background work and unseen calls are not covered.
        </p>
        <p>
          Elapsed time is measured service-call wall time, which can overlap
          across operations. It is not CPU time, task duration, or model time
          saved. Returned bytes measure supported text at the server boundary,
          not confirmed model delivery. This is the same server result text
          shown above, not additional usage. These measurements do not establish
          avoided model work.
        </p>
        <p>
          Measured durations: {work.measured_durations.toLocaleString()} ·
          Missing durations: {work.missing_durations.toLocaleString()} · Missing
          text measurements: {work.missing_payload.toLocaleString()} ·
          Operations without session attribution:{" "}
          {work.unassigned_sessions.toLocaleString()}.
        </p>
        <p>
          Session filters include only attributed operations; unassigned project
          activity is not borrowed into a session. A reported tool error is an
          execution outcome. Completed-task correctness requires separate
          verification evidence.
        </p>
      </div>
    </details>
  );
}
