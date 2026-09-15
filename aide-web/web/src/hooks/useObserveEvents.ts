import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "@/lib/api";
import { useApi } from "./use-api";
import { useEventStream, type EventStreamStatus } from "./useEventStream";
import type { ObserveEventItem } from "@/lib/types";
import { createObserveEventBatcher, mergeObserveEvents } from "@/lib/observe-groups";

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
  const filterKey = JSON.stringify([project, kind, name, category, session]);
  if (lastFiltersRef.current !== filterKey) {
    lastFiltersRef.current = filterKey;
    if (liveEvents.length > 0) setLiveEvents([]);
  }

  const batcher = useMemo(() => createObserveEventBatcher(
    batch => setLiveEvents(prev => mergeObserveEvents(batch, prev, maxEvents)),
    maxEvents,
  ), [filterKey, maxEvents, liveTail]);
  useEffect(() => () => batcher.cancel(), [batcher]);

  const handleStreamEvent = useCallback(
    (ev: ObserveEventItem) => {
      if (name && ev.name !== name) return;
      batcher.push(ev);
    },
    [batcher, name],
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
    return mergeObserveEvents(liveEvents, initialEvents ?? [], maxEvents);
  }, [liveEvents, initialEvents, maxEvents]);

  return { events, loading, error, streamStatus, liveTail, setLiveTail };
}
