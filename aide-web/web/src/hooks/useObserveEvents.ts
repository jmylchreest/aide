import { useCallback, useMemo, useRef, useState } from "react";
import { api } from "@/lib/api";
import { useApi } from "./use-api";
import { useEventStream, type EventStreamStatus } from "./useEventStream";
import type { ObserveEventItem } from "@/lib/types";

export interface ObserveFilters {
  kind?: string;
  name?: string;
  category?: string;
  session?: string;
}

export interface UseObserveEventsOptions {
  project: string | undefined;
  filters: ObserveFilters;
  /** Max events held in the live buffer (newest-first). Defaults to 1000. */
  maxEvents?: number;
  /** Initial page size pulled via the unary List endpoint. Defaults to 200. */
  initialLimit?: number;
}

export interface UseObserveEventsResult {
  events: ObserveEventItem[];
  loading: boolean;
  error: string | null;
  streamStatus: EventStreamStatus;
  liveTail: boolean;
  setLiveTail: (v: boolean | ((prev: boolean) => boolean)) => void;
}

/**
 * Owns the full observe-events read path: initial unary fetch, optional
 * SSE live tail, dedupe by id, bounded buffer, and reset of the live
 * buffer when filters change (otherwise stale rows leak across queries).
 */
export function useObserveEvents({
  project,
  filters,
  maxEvents = 1000,
  initialLimit = 200,
}: UseObserveEventsOptions): UseObserveEventsResult {
  const { kind, name, category, session } = filters;

  const {
    data: initialEvents,
    loading,
    error,
  } = useApi(
    () =>
      project
        ? api.listObserveEvents(project, {
            kind: kind || undefined,
            name: name || undefined,
            category: category || undefined,
            session: session || undefined,
            limit: initialLimit,
          })
        : Promise.resolve([] as ObserveEventItem[]),
    [project, kind, name, category, session, initialLimit],
  );

  const [liveEvents, setLiveEvents] = useState<ObserveEventItem[]>([]);
  const [liveTail, setLiveTail] = useState(false);

  // Drop live buffer on filter change — otherwise stale rows leak through.
  const lastFiltersRef = useRef("");
  const filterKey = `${kind ?? ""}|${name ?? ""}|${category ?? ""}|${session ?? ""}`;
  if (lastFiltersRef.current !== filterKey) {
    lastFiltersRef.current = filterKey;
    if (liveEvents.length > 0) setLiveEvents([]);
  }

  const handleStreamEvent = useCallback(
    (ev: ObserveEventItem) => {
      setLiveEvents((prev) => {
        const next = [ev, ...prev];
        return next.length > maxEvents ? next.slice(0, maxEvents) : next;
      });
    },
    [maxEvents],
  );

  const watchUrl = useMemo(
    () =>
      project
        ? api.observeWatchUrl(project, {
            kind: kind || undefined,
            category: category || undefined,
            session: session || undefined,
            since_id: initialEvents?.[0]?.id,
          })
        : "",
    [project, kind, category, session, initialEvents],
  );

  const { status: streamStatus } = useEventStream<ObserveEventItem>(watchUrl, {
    enabled: liveTail && !!project,
    onEvent: handleStreamEvent,
  });

  const events = useMemo(() => {
    const seen = new Set<string>();
    const out: ObserveEventItem[] = [];
    for (const ev of liveEvents) {
      if (!seen.has(ev.id)) {
        seen.add(ev.id);
        out.push(ev);
      }
    }
    for (const ev of initialEvents ?? []) {
      if (!seen.has(ev.id)) {
        seen.add(ev.id);
        out.push(ev);
      }
    }
    return out;
  }, [liveEvents, initialEvents]);

  return { events, loading, error, streamStatus, liveTail, setLiveTail };
}
