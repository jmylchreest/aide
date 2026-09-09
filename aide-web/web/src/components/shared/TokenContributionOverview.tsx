import { useState } from "react";
import type { TokenAccounting, TokenChange } from "../../lib/types";
import {
  summarizeRetrievalContribution,
  validatedTransformationChange,
} from "../../lib/token-contribution";
import { supportedTokenWork } from "./TokenWork";

const number = (n: number) =>
  n.toLocaleString(undefined, {
    notation: Math.abs(n) >= 10000 ? "compact" : "standard",
    maximumFractionDigits: 1,
  });
const direction = (n: number) => (n < 0 ? "added" : n > 0 ? "less" : "change");

function TextChange({ change }: { change: TokenChange }) {
  const max = Math.max(change.before_bytes, change.after_bytes, 1);
  return (
    <>
      <div
        className={`text-xl font-semibold mt-1 ${change.delta_bytes < 0 ? "text-amber-400" : "text-aide-text"}`}
        title={`${Math.abs(change.estimated_token_delta).toLocaleString()} estimated tokens ${direction(change.estimated_token_delta)}`}
      >
        ~{number(Math.abs(change.estimated_token_delta))} tokens{" "}
        {direction(change.estimated_token_delta)}
      </div>
      <p className="text-[11px] text-aide-text-muted mt-1">
        Estimated tokens · {number(Math.abs(change.delta_bytes))} measured bytes{" "}
        {direction(change.delta_bytes)}
      </p>
      <div
        role="img"
        aria-label={`Text comparison: reference ${change.before_bytes.toLocaleString()} bytes, result ${change.after_bytes.toLocaleString()} bytes`}
        className="mt-3 space-y-1.5"
      >
        {[
          {
            label: "Reference",
            bytes: change.before_bytes,
            color: "bg-aide-text-dim",
          },
          {
            label: "Result",
            bytes: change.after_bytes,
            color: change.delta_bytes < 0 ? "bg-amber-400" : "bg-aide-accent",
          },
        ].map(({ label, bytes, color }) => (
          <div
            key={label}
            className="flex items-center gap-2 text-[10px] text-aide-text-dim"
          >
            <span className="w-14 shrink-0">{label}</span>
            <div className="h-1.5 bg-aide-bg rounded flex-1 overflow-hidden">
              <div
                className={`h-full rounded ${color}`}
                style={{ width: `${(bytes / max) * 100}%` }}
              />
            </div>
          </div>
        ))}
      </div>
    </>
  );
}

export function TokenContributionOverview({
  accounting,
  onDetails,
  onAccounting,
}: {
  accounting?: TokenAccounting;
  onDetails: () => void;
  onAccounting: () => void;
}) {
  const report = accounting?.version === 1 ? accounting : undefined;
  const measured = validatedTransformationChange(
    report?.transformations,
    "adapter_change",
  );
  const proposed = validatedTransformationChange(
    report?.transformations,
    "rewrite_candidate",
  );
  const [preferProposed, setPreferProposed] = useState(false);
  const isProposed = !!proposed && (!measured || preferProposed);
  const change = isProposed ? proposed : measured;
  const retrieval = summarizeRetrievalContribution(report?.retrievals);
  const work = supportedTokenWork(report?.work);
  const context = report?.by_stage.aide_context;
  const contextKnown =
    context &&
    context.events > 0 &&
    [context.bytes, context.estimated_tokens, context.events].every(
      (n) => Number.isSafeInteger(n) && n >= 0,
    );
  return (
    <section aria-label="Aide contribution" className="mb-5">
      <div className="flex justify-between items-center flex-wrap gap-2 mb-3">
        <h3 className="text-xs font-semibold text-aide-text">
          Aide contribution
        </h3>
        <button
          type="button"
          onClick={onDetails}
          className="text-xs text-aide-accent hover:underline"
        >
          Contribution evidence →
        </button>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <div className="rounded-md border border-aide-border bg-aide-surface p-4 min-w-0">
          <h4 className="text-xs text-aide-text-muted">
            {isProposed
              ? "Proposed output reduction"
              : "Measured output reduction"}
          </h4>
          {measured && proposed && (
            <select
              aria-label="Output comparison source"
              value={isProposed ? "proposed" : "measured"}
              onChange={(e) => setPreferProposed(e.target.value === "proposed")}
              className="text-xs mt-2 w-full min-w-0 bg-aide-bg border border-aide-border rounded px-2 py-1"
            >
              <option value="measured">Measured changes</option>
              <option value="proposed">Proposed rewrites</option>
            </select>
          )}
          {change ? (
            <TextChange change={change} />
          ) : (
            <>
              <div className="text-xl font-semibold mt-1">Not measured</div>
              <p className="text-[11px] text-aide-text-muted mt-1">
                No supported before/after pairs
              </p>
            </>
          )}
          {change && (
            <p className="text-[11px] text-aide-text-dim mt-3">
              {change.events.toLocaleString()} recorded{" "}
              {change.events === 1 ? "pair" : "pairs"} ·{" "}
              {isProposed
                ? "host may ignore proposals"
                : "before later host processing"}
            </p>
          )}
        </div>
        <div className="rounded-md border border-aide-border bg-aide-surface p-4 min-w-0">
          <h4 className="text-xs text-aide-text-muted">
            Potential retrieval reduction
          </h4>
          {retrieval?.change ? (
            <TextChange change={retrieval.change} />
          ) : (
            <>
              <div className="text-xl font-semibold mt-1">Not established</div>
              <p className="text-[11px] text-aide-text-muted mt-1">
                {retrieval
                  ? "No comparable context windows"
                  : "Comparison evidence unavailable"}
              </p>
            </>
          )}
          {retrieval && (
            <p className="text-[11px] text-aide-text-dim mt-3">
              {retrieval.eligible_windows}/{retrieval.shown_windows} shown
              windows comparable
              {retrieval.windows_limited ? " · more windows omitted" : ""}
              {retrieval.unwindowed_events > 0
                ? " · unassigned observations"
                : ""}
            </p>
          )}
          {retrieval?.change && (
            <p className="text-[11px] text-aide-text-muted mt-1">
              Conditional full-file baseline; avoided reads unproven.
            </p>
          )}
        </div>
        <div className="rounded-md border border-aide-border bg-aide-surface p-4 min-w-0">
          <h4 className="text-xs text-aide-text-muted">
            Work performed by aide
          </h4>
          <div className="text-xl font-semibold mt-1">
            {work
              ? `${number(work.calls)} ${work.calls === 1 ? "operation" : "operations"}`
              : "Unknown"}
          </div>
          <p className="text-[11px] text-aide-text-muted mt-1">
            {work
              ? `${number(work.reported_errors)} reported errors · ${number(work.unknown_outcomes)} unknown outcomes`
              : "Operation evidence unavailable"}
          </p>
          <p className="text-[11px] text-aide-text-dim mt-3">
            Recorded operations; task quality requires separate checks.
          </p>
        </div>
      </div>
      <div className="flex flex-wrap justify-between gap-2 mt-3 text-[11px] text-aide-text-dim">
        <p>
          Partial coverage. Text comparisons can overlap; they are not model or
          billing savings.
        </p>
        <button
          type="button"
          onClick={onAccounting}
          className="text-aide-text-muted hover:underline"
          title={
            contextKnown
              ? `${context.bytes.toLocaleString()} measured bytes; ${context.estimated_tokens.toLocaleString()} estimated tokens`
              : undefined
          }
        >
          Prepared aide context:{" "}
          {contextKnown
            ? `~${number(context.estimated_tokens)} estimated tokens`
            : "unknown"}{" "}
          →
        </button>
      </div>
    </section>
  );
}
