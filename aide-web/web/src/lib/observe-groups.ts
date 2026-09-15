import type { ObserveEventItem } from "./types";

export interface ObserveRow {
  id: string;
  type: "event" | "usage";
  events: ObserveEventItem[];
}

/** Newest submissions win; repeated IDs must not evict distinct evidence. */
export function mergeObserveEvents(newest: ObserveEventItem[], older: ObserveEventItem[], limit: number): ObserveEventItem[] {
  if (limit <= 0) return [];
  const byId = new Map<string, ObserveEventItem>();
  for (const event of [...newest, ...older]) {
    if (!byId.has(event.id)) byId.set(event.id, event);
    if (byId.size >= limit) break;
  }
  return [...byId.values()];
}

/** Coalesce transport messages into at most ten render updates per second. */
export function createObserveEventBatcher(publish: (events: ObserveEventItem[]) => void, limit: number) {
  const pending = new Map<string, ObserveEventItem>();
  let timer: ReturnType<typeof setTimeout> | undefined;
  return {
    push(event: ObserveEventItem) {
      if (limit <= 0) return;
      pending.delete(event.id);
      pending.set(event.id, event);
      if (pending.size > limit) pending.delete(pending.keys().next().value!);
      if (timer === undefined) timer = setTimeout(() => {
        timer = undefined;
        const events = [...pending.values()].reverse();
        pending.clear();
        publish(events);
      }, 100);
    },
    cancel() {
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
      pending.clear();
    },
  };
}

/** Presentation only: no accounting totals or stored identities are changed. */
export function groupObserveEvents(events: ObserveEventItem[]): ObserveRow[] {
  const rows: ObserveRow[] = [];
  const groups = new Map<string, ObserveRow>();
  for (const event of mergeObserveEvents(events, [], events.length)) {
    const minute = Math.floor(Date.parse(event.timestamp) / 60_000);
    const host = event.attrs?.host;
    const source = event.attrs?.usage_source;
    if (event.kind !== "session" || event.name !== "model_usage" || !event.session_id || !host || !source || !Number.isFinite(minute)) {
      rows.push({ id: event.id, type: "event", events: [event] });
      continue;
    }
    const key = JSON.stringify([event.session_id, host, source, event.attrs?.context_window_id ?? "", minute]);
    const group = groups.get(key);
    if (group) group.events.push(event);
    else {
      const row: ObserveRow = { id: `usage:${key}`, type: "usage", events: [event] };
      groups.set(key, row);
      rows.push(row);
    }
  }
  return rows;
}
