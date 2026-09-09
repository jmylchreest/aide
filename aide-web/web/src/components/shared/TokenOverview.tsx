import { useState } from "react";
import type { TokenStats } from "../../lib/types";
import { TokenContributionOverview } from "./TokenContributionOverview";
import { TokenModelUsageOverview } from "./TokenModelUsageOverview";

function number(n: number) {
  return n.toLocaleString(undefined, {
    notation: n >= 10000 ? "compact" : "standard",
    maximumFractionDigits: 1,
  });
}

export function TokenOverview({
  stats,
  onDetails,
  onAccounting,
}: {
  stats: Pick<TokenStats, "sessions" | "event_count" | "accounting">;
  onDetails: () => void;
  onAccounting: () => void;
}) {
  const accounting =
    stats.accounting?.version === 1 ? stats.accounting : undefined;
  const [source, setSource] = useState(
    accounting?.by_stage.host_result
      ? "host_result"
      : accounting?.by_stage.server_result
        ? "server_result"
        : "host_result",
  );
  const quantity = accounting?.by_stage[source];
  const buckets = accounting?.activity?.buckets ?? [];
  const [selected, setSelected] = useState<number | null>(null);
  const measured = buckets.filter((b) => (b.by_stage[source]?.events ?? 0) > 0);
  const max = Math.max(
    1,
    ...measured.map((b) => b.by_stage[source].estimated_tokens),
  );
  const interval = (accounting?.activity?.interval_seconds ?? 60) * 1000;
  const first = buckets.length ? Date.parse(buckets[0].start) : 0;
  const last = buckets.length
    ? Date.parse(buckets[buckets.length - 1].start) + interval
    : first + interval;
  const span = Math.max(interval, last - first);
  const partial =
    accounting &&
    (accounting.legacy_events > 0 ||
      accounting.missing_payload > 0 ||
      accounting.missing_identity > 0);
  const label =
    source === "host_result" ? "Agent tool results" : "Aide server results";
  const detail = selected === null ? undefined : buckets[selected];
  const date = (value: string) =>
    new Date(value).toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      ...(interval < 86400000
        ? { hour: "2-digit" as const, minute: "2-digit" as const }
        : {}),
    });
  return (
    <section aria-label="Token overview">
      <TokenContributionOverview
        accounting={accounting}
        onDetails={onDetails}
        onAccounting={onAccounting}
      />
      <TokenModelUsageOverview
        report={accounting?.model_usage}
        onAccounting={onAccounting}
      />
      <div className="flex items-center justify-between gap-3 flex-wrap mb-3">
        <label className="text-xs text-aide-text-muted">
          Results from{" "}
          <select
            aria-label="Result source"
            value={source}
            onChange={(e) => {
              setSource(e.target.value);
              setSelected(null);
            }}
            className="bg-aide-surface border border-aide-border rounded px-2 py-1 text-aide-text"
          >
            <option value="host_result">Agent tools</option>
            <option value="server_result">Aide server</option>
          </select>
        </label>
        <button
          type="button"
          onClick={onAccounting}
          className="text-[11px] text-aide-text-muted hover:text-aide-accent underline underline-offset-4"
        >
          {!accounting
            ? "Data unavailable"
            : partial
              ? "Partial data"
              : "Collection details"}
        </button>
      </div>
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 mb-4">
        {[
          {
            label: "Estimated result tokens",
            value: quantity?.events
              ? `~${number(quantity.estimated_tokens)}`
              : "Unknown",
            sub: label,
          },
          {
            label: "Recorded events",
            value: number(stats.event_count),
            sub: "All sources",
          },
          {
            label: "Sessions",
            value: number(stats.sessions),
            sub: "Identified sessions",
          },
        ].map((card, index) => (
          <div
            key={card.label}
            className={`rounded-md border border-aide-border bg-aide-surface px-4 py-4 ${index === 0 ? "col-span-2 sm:col-span-1" : ""}`}
          >
            <div className="text-[10px] text-aide-text-dim uppercase tracking-wider">
              {card.label}
            </div>
            <div className="text-2xl font-semibold text-aide-text my-1">
              {card.value}
            </div>
            <div className="text-[11px] text-aide-text-muted">{card.sub}</div>
          </div>
        ))}
      </div>
      <div className="border border-aide-border rounded-md p-4">
        <div className="flex justify-between items-baseline flex-wrap gap-2 mb-4">
          <h3 className="text-xs font-semibold text-aide-text">
            Recorded token activity
          </h3>
          <span className="text-[10px] text-aide-text-dim">
            Estimated tokens · {label.toLowerCase()}
          </span>
        </div>
        {measured.length ? (
          <>
            <svg
              role="img"
              aria-label={`Estimated tokens over time: ${label}`}
              viewBox="0 0 960 120"
              preserveAspectRatio="none"
              className="w-full h-32 overflow-visible"
            >
              <line
                x1="0"
                x2="960"
                y1="119"
                y2="119"
                stroke="currentColor"
                className="text-aide-border"
              />
              {buckets.map((bucket, i) => {
                const q = bucket.by_stage[source];
                if (!q?.events) return null;
                const x = ((Date.parse(bucket.start) - first) / span) * 960;
                const width = Math.max(
                  2,
                  Math.min(48, (interval / span) * 960 - 2),
                );
                const height = Math.max(2, (q.estimated_tokens / max) * 106);
                const description = `${date(bucket.start)}: ~${q.estimated_tokens} tokens, ${q.events} measured observations`;
                return (
                  <rect
                    key={bucket.start}
                    x={x + ((interval / span) * 960 - width) / 2}
                    y={118 - height}
                    width={width}
                    height={height}
                    rx="1"
                    fill={q.estimated_tokens === 0 ? "none" : "currentColor"}
                    stroke="currentColor"
                    className="text-aide-accent focus:text-aide-text outline-none"
                    tabIndex={0}
                    role="img"
                    aria-label={description}
                    onMouseEnter={() => setSelected(i)}
                    onFocus={() => setSelected(i)}
                    onMouseLeave={() => setSelected(null)}
                    onBlur={() => setSelected(null)}
                  >
                    <title>{description}</title>
                  </rect>
                );
              })}
            </svg>
            <div className="flex justify-between text-[10px] text-aide-text-dim mt-2">
              <span>{date(buckets[0].start)}</span>
              <span>{date(new Date(last).toISOString())}</span>
            </div>
            <p
              aria-live="polite"
              className="text-[11px] text-aide-text-muted mt-3 min-h-8"
            >
              {detail
                ? `${date(detail.start)} · ~${detail.by_stage[source]?.estimated_tokens ?? 0} tokens · ${detail.by_stage[source]?.events ?? 0} measured observations`
                : "Recorded estimates only. Gaps indicate no measured data."}
            </p>
          </>
        ) : (
          <p className="h-32 flex items-center text-xs text-aide-text-muted">
            {accounting?.activity
              ? "No measured activity for this source in the selection."
              : "Activity history is unavailable from this server."}
          </p>
        )}
        <button
          type="button"
          onClick={onDetails}
          className="text-xs text-aide-accent hover:underline"
        >
          Explore breakdowns →
        </button>
      </div>
    </section>
  );
}
