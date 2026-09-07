import type { TokenChange, TokenTransformations } from "../../lib/types";

const labels: Record<string, string> = {
  rewrite_candidate: "Proposed rewrites",
  adapter_change: "Adapter changes",
};
const signed = (n: number) => `${n > 0 ? "+" : ""}${n.toLocaleString()}`;

function Change({ label, change }: { label: string; change?: TokenChange }) {
  const q = change?.events ? change : undefined;
  const max = q ? Math.max(q.before_bytes, q.after_bytes, 1) : 1;
  return (
    <div className="rounded-md border border-aide-border bg-aide-surface p-4">
      <h4 className="text-xs text-aide-text-muted">{label}</h4>
      <div className="text-lg font-semibold text-aide-text mt-1">
        {q
          ? `${Math.abs(q.delta_bytes).toLocaleString()} bytes ${q.delta_bytes < 0 ? "added" : "less"}`
          : "Unknown"}
      </div>
      <p className="text-[11px] text-aide-text-dim">
        {q
          ? `${Math.abs(q.estimated_token_delta).toLocaleString()} estimated tokens ${q.estimated_token_delta < 0 ? "added" : "less"} · ${q.events} measured ${q.events === 1 ? "pair" : "pairs"}`
          : "No measured pairs in this selection"}
      </p>
      {q && (
        <div className="mt-3 space-y-2">
          {(
            [
              ["Before", q.before_bytes],
              ["After", q.after_bytes],
            ] as const
          ).map(([name, bytes]) => (
            <div key={name}>
              <div className="flex justify-between text-[10px] text-aide-text-muted">
                <span>{name}</span>
                <span>{bytes.toLocaleString()} bytes</span>
              </div>
              <div className="h-1.5 bg-aide-bg rounded mt-1" aria-hidden="true">
                <div
                  className={`h-full rounded ${name === "Before" ? "bg-aide-text-dim" : q.delta_bytes < 0 ? "bg-amber-400" : "bg-aide-accent"}`}
                  style={{ width: `${(bytes / max) * 100}%` }}
                />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function TokenTransformationSummary({
  report,
}: {
  report?: TokenTransformations;
}) {
  if (!report)
    return (
      <p className="text-xs text-aide-text-muted my-4">
        Paired output accounting is unavailable from this server.
      </p>
    );
  return (
    <section aria-label="Paired output changes" className="my-5">
      <h3 className="text-xs font-semibold text-aide-text mb-2">
        Output changes
      </h3>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {Object.entries(labels).map(([stage, label]) => (
          <Change key={stage} label={label} change={report.by_stage[stage]} />
        ))}
      </div>
      <p className="text-[11px] text-aide-text-muted mt-2">
        Proposed rewrites may be ignored by the host. Adapter changes measure
        aide’s output before later processing. These stages remain separate;
        neither confirms final delivery or provider usage.
      </p>
    </section>
  );
}

export function TokenTransformationWindows({
  report,
}: {
  report?: TokenTransformations;
}) {
  if (!report) return null;
  return (
    <section aria-label="Output changes by context window" className="mb-6">
      <h3 className="text-xs font-semibold text-aide-text mb-2">
        Output changes by context window
      </h3>
      <p className="text-[11px] text-aide-text-muted mb-2">
        Each pair counts once. Positive reductions mean less text; negative
        values mean added text. {report.unwindowed_events} without an active
        window · {report.invalid_events} invalid pairs.
      </p>
      {!report.windows.length ? (
        <p className="text-xs text-aide-text-dim">
          No paired observations with an active context window in this
          selection.
        </p>
      ) : (
        <div className="overflow-x-auto rounded border border-aide-border">
          <table className="w-full text-left text-[11px] whitespace-nowrap">
            <thead className="bg-aide-surface text-aide-text-muted">
              <tr>
                {[
                  "Context window",
                  "Boundary",
                  "Last observed",
                  "Pairs",
                  "Byte reduction",
                  "Est. token reduction",
                ].map((h) => (
                  <th key={h} className="px-3 py-2 font-medium">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {report.windows.map((w) => (
                <tr
                  key={JSON.stringify([
                    w.host,
                    w.session_id,
                    w.actor_id,
                    w.epoch,
                    w.stage,
                  ])}
                  className="border-t border-aide-border text-aide-text"
                >
                  <td className="px-3 py-2">
                    <details>
                      <summary className="cursor-pointer">
                        {w.host} · {w.epoch.slice(0, 8)}
                      </summary>
                      <div className="mt-1 text-aide-text-dim">
                        Session: {w.session_id}
                        <br />
                        Actor: {w.actor_id}
                        <br />
                        Window: {w.epoch}
                      </div>
                    </details>
                  </td>
                  <td className="px-3 py-2">{labels[w.stage] ?? w.stage}</td>
                  <td className="px-3 py-2">
                    {new Date(w.last).toLocaleString()}
                  </td>
                  <td className="px-3 py-2">{w.change.events}</td>
                  <td className="px-3 py-2">{signed(w.change.delta_bytes)}</td>
                  <td className="px-3 py-2">
                    ~{signed(w.change.estimated_token_delta)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {report.windows_limited && (
        <p className="text-[11px] text-aide-text-muted mt-2">
          Additional windows omitted; stage totals include all selected pairs.
        </p>
      )}
    </section>
  );
}
