import type { ObserveRow } from "../../lib/observe-groups";
import { formatTimestamp } from "../../lib/format";

export function ModelUsageEventGroup({ row }: { row: ObserveRow }) {
  const first = row.events[0];
  return (
    <details className="border-b border-aide-border last:border-b-0">
      <summary className="cursor-pointer px-3 py-2 text-xs hover:bg-aide-accent/5">
        <span className="text-aide-text-dim text-[0.6rem] mr-3">{formatTimestamp(first.timestamp)}</span>
        <span className="text-aide-text">{row.events.length} usage {row.events.length === 1 ? "record" : "records"}</span>
        <span className="text-aide-text-dim ml-3">{first.attrs?.host} · {first.session_id?.slice(0, 8)}</span>
      </summary>
      <div className="border-t border-aide-border bg-aide-surface px-3 py-2">
        <p className="text-[0.65rem] text-aide-text-dim mb-2 break-all">
          {first.attrs?.usage_source} · {first.session_id} · Grouped by event minute. Counts cover loaded, matching records.
        </p>
        {row.events.map(event => (
          <details key={event.id} className="py-1">
            <summary className="cursor-pointer text-[0.65rem] text-aide-text-muted break-all">
              {formatTimestamp(event.timestamp)} · {event.id}{event.error ? ` · ${event.error}` : ""}
            </summary>
            <pre className="text-[0.65rem] text-aide-text-muted p-2 overflow-x-auto">{JSON.stringify(event, null, 2)}</pre>
          </details>
        ))}
      </div>
    </details>
  );
}
