import { useState } from "react";
import type { RetrievalWindow, TokenRetrievals } from "../../lib/types";

const issues: Record<string, string> = {
  filtered_window: "Date filter clips this window",
  missing_payload: "Some result text was not measured",
  unclassified_shell: "Some shell activity has unknown retrieval scope",
  unverified_retrieval: "Some retrievals remain unverified or pending",
  unmatched_target: "A retrieval target has no verified reference",
  unknown_target: "Some retrieval targets are unknown",
  unverified_source_version: "A source version could not be verified",
  invalid_source_reference: "Invalid source reference evidence",
  conflicting_source_reference: "Conflicting sizes for the same source version",
  no_assisted_reference: "No verified outline or symbol reference",
  context_gap: "Some observations lack usable context identity",
  source_limit: "Source evidence limit reached",
  target_limit: "Target evidence limit reached",
  quantity_overflow: "Quantity exceeds the supported range",
};
const boundaries: Record<string, string> = {
  reset_observed: "Later context reset observed",
  continuity_unknown: "Later context continuity unknown",
  open_or_unknown: "Open or end boundary unknown",
};
const statuses: Record<string, string> = {
  referenced: "Source receipt",
  full_file: "Full-file result",
  range: "Matched range",
  search: "Search",
  failed: "Failed",
  pending: "Pending",
  unverified: "Unverified",
  unclassified_shell: "Unclassified shell",
};

function Window({ window: w }: { window: RetrievalWindow }) {
  const delta = w.comparison?.estimated_token_delta;
  const max = Math.max(w.reference.bytes, w.observed.bytes, 1);
  return (
    <details className="group border border-aide-border rounded-md bg-aide-surface/30">
      <summary className="cursor-pointer px-3 py-2 text-xs flex flex-wrap justify-between gap-2">
        <span>
          <span
            aria-hidden="true"
            className="inline-block mr-2 transition-transform group-open:rotate-90"
          >
            ▸
          </span>
          {w.host} · {w.session_id.slice(0, 12)} · {w.events} observations
        </span>
        <span
          className={
            delta !== undefined && delta < 0
              ? "text-amber-400"
              : "text-aide-text-muted"
          }
        >
          {delta === undefined
            ? "Comparison unavailable"
            : `~${Math.abs(delta).toLocaleString()} ${delta < 0 ? "more" : "fewer"} tokens vs reference`}
        </span>
      </summary>
      <div className="border-t border-aide-border p-3 text-xs text-aide-text-muted space-y-3">
        <p>
          {boundaries[w.boundary] ?? "Boundary unknown"}.{" "}
          {new Date(w.first).toLocaleString()} –{" "}
          {new Date(w.last).toLocaleString()}
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          {[
            { label: "Full-file reference", q: w.reference },
            { label: "Observed retrieval text", q: w.observed },
          ].map(({ label, q }) => (
            <div key={label}>
              <div className="flex justify-between gap-2">
                <span>{label}</span>
                <span>
                  {q.events ? `${q.bytes.toLocaleString()} bytes` : "Unknown"}
                </span>
              </div>
              <div className="mt-1 h-1.5 rounded bg-aide-bg" aria-hidden="true">
                <div
                  className="h-full rounded bg-aide-text-dim"
                  style={{ width: `${(q.bytes / max) * 100}%` }}
                />
              </div>
            </div>
          ))}
        </div>
        <p>
          {w.sources.length} file{" "}
          {w.sources.length === 1 ? "version" : "versions"} counted once ·{" "}
          {w.failed_events} failures · {w.search_events} searches.{" "}
          {w.full_read_events
            ? `${w.full_read_events} full-file ${w.full_read_events === 1 ? "result" : "results"} observed.`
            : "No full-file result observed in these records; coverage is partial."}
        </p>
        {w.comparison ? (
          <p>
            Conditional comparison of the referenced full files with recorded
            retrieval text. This can change as more calls arrive; it does not
            establish avoided calls or provider savings.
          </p>
        ) : (
          <ul className="list-disc pl-4">
            {w.issues.map((issue) => (
              <li key={issue}>{issues[issue] ?? "Incomplete evidence"}</li>
            ))}
          </ul>
        )}
        {w.unattributed.events > 0 && (
          <p>
            {w.unattributed.bytes.toLocaleString()} measured bytes from{" "}
            {w.unattributed.events} shell observations are unassigned; they are
            not included in retrieval text.
          </p>
        )}
        {w.missing_payload > 0 && (
          <p>
            {w.missing_payload} results have unknown text size. Displayed bytes
            cover only measured results.
          </p>
        )}
        <div className="overflow-x-auto">
          <table className="w-full text-left text-[11px] min-w-[34rem]">
            <thead>
              <tr className="text-aide-text-dim">
                <th className="py-2 pr-3">Time</th>
                <th className="pr-3">Tool</th>
                <th className="pr-3">Evidence</th>
                <th className="pr-3">Bytes</th>
                <th>Target</th>
              </tr>
            </thead>
            <tbody>
              {w.steps.map((step) => (
                <tr key={step.id} className="border-t border-aide-border">
                  <td className="py-2 pr-3 whitespace-nowrap">
                    {new Date(step.at).toLocaleTimeString()}
                  </td>
                  <td className="pr-3">{step.tool}</td>
                  <td className="pr-3">{statuses[step.status] ?? "Unknown"}</td>
                  <td className="pr-3">
                    {step.text ? step.text.bytes.toLocaleString() : "Unknown"}
                  </td>
                  <td className="break-all">
                    {step.target || "See source references"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {w.steps_limited && (
          <p>
            64 steps shown. Totals include all {w.events} selected observations.
          </p>
        )}
        <details>
          <summary className="cursor-pointer">
            Source and identity evidence
          </summary>
          <div className="mt-2 space-y-2 break-all">
            <p>
              Session: {w.session_id}
              <br />
              Actor: {w.actor_id}
              <br />
              Window: {w.epoch}
            </p>
            {w.sources.map((s) => (
              <p key={s.file + s.sha256}>
                {s.file} · {s.bytes.toLocaleString()} bytes
                <br />
                <span className="font-mono">{s.sha256}</span>
              </p>
            ))}
          </div>
        </details>
      </div>
    </details>
  );
}

export function TokenRetrievalWindows({
  report,
}: {
  report?: TokenRetrievals;
}) {
  const [all, setAll] = useState(false);
  if (!report)
    return (
      <p className="my-4 text-xs text-aide-text-muted">
        Retrieval grouping is unavailable from this server.
      </p>
    );
  if (!report.windows.length && !report.unwindowed_events) return null;
  return (
    <section aria-label="Retrieval sequences" className="my-5 text-aide-text">
      <h3 className="text-xs font-semibold mb-2">Retrieval sequences</h3>
      <p className="text-[11px] text-aide-text-muted mb-3">
        Grouped by context window. Conditional full-file references count each
        version once; host result text counts each call once. Collection
        coverage is partial.
      </p>
      <div className="space-y-2">
        {(all ? report.windows : report.windows.slice(0, 8)).map((w) => (
          <Window
            key={JSON.stringify([w.host, w.session_id, w.actor_id, w.epoch])}
            window={w}
          />
        ))}
      </div>
      {report.windows.length > 8 && (
        <button
          className="text-xs text-aide-accent mt-2"
          onClick={() => setAll(!all)}
        >
          {all
            ? "Show fewer windows"
            : `Show all ${report.windows.length} windows`}
        </button>
      )}
      {report.unwindowed_events > 0 && (
        <p className="text-[11px] text-aide-text-muted mt-2">
          {report.unwindowed_events} retrieval observations could not be
          assigned to an active context window.
        </p>
      )}
      {report.windows_limited && (
        <p className="text-[11px] text-aide-text-muted mt-2">
          Report limited to 64 windows. Each retained window includes all its
          selected observations.
        </p>
      )}
    </section>
  );
}
