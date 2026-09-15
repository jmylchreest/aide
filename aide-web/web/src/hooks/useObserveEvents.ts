import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "@/lib/api";
import { useEventStream, type EventStreamStatus } from "./useEventStream";
import type { ObserveEventItem } from "@/lib/types";
import {
  createObserveEventBatcher,
  mergeObserveEvents,
} from "@/lib/observe-groups";

export interface ObserveFilters {
  kind?: string;
  name?: string;
  category?: string;
  session?: string;
}

export interface UseObserveEventsOptions {
  project: string | undefined;
  filters: ObserveFilters;
  /** Max events held in the merged list and live buffer (newest-first). Defaults to 1000. */
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

  const queryKey = JSON.stringify([
    project,
    kind,
    name,
    category,
    session,
    initialLimit,
  ]);
  const [initial, setInitial] = useState<{
    key: string;
    events: ObserveEventItem[];
    error: string | null;
  } | null>(null);
  useEffect(() => {
    let active = true;
    const request = project
      ? api.listObserveEvents(project, {
          kind: kind || undefined,
          name: name || undefined,
          category: category || undefined,
          session: session || undefined,
          limit: initialLimit,
        })
      : Promise.resolve([] as ObserveEventItem[]);
    request.then(
      (events) => {
        if (active) setInitial({ key: queryKey, events, error: null });
      },
      (error) => {
        if (active)
          setInitial({
            key: queryKey,
            events: [],
            error: String(error?.message ?? error),
          });
      },
    );
    return () => {
      active = false;
    };
  }, [project, kind, name, category, session, initialLimit, queryKey]);
  // A previous query must never supply rows or the new stream's cursor.
  const initialEvents = initial?.key === queryKey ? initial.events : null;
  const loading = initial?.key !== queryKey;
  const error = initial?.key === queryKey ? initial.error : null;

  const [liveEvents, setLiveEvents] = useState<ObserveEventItem[]>([]);
  const [liveTail, setLiveTail] = useState(false);

  // Drop live buffer on filter change — otherwise stale rows leak through.
  const lastFiltersRef = useRef("");
  const filterKey = JSON.stringify([project, kind, name, category, session]);
  if (lastFiltersRef.current !== filterKey) {
    lastFiltersRef.current = filterKey;
    if (liveEvents.length > 0) setLiveEvents([]);
  }

  const batcher = useMemo(
    () =>
      createObserveEventBatcher(
        (batch) =>
          setLiveEvents((prev) => mergeObserveEvents(batch, prev, maxEvents)),
        maxEvents,
      ),
    [filterKey, maxEvents, liveTail],
  );
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
