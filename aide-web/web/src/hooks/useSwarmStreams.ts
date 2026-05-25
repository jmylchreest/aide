import { useCallback, useMemo, useRef, useState } from "react";
import { api } from "@/lib/api";
import { useEventStream, type EventStreamStatus } from "./useEventStream";
import type {
  SwarmTaskUpdate,
  SwarmMessageUpdate,
  SwarmStateUpdate,
} from "@/lib/types";

interface WatchTasksOptions {
  project: string | undefined;
  parentSession?: string;
  status?: string;
  enabled?: boolean;
  maxItems?: number;
}

export interface WatchTasksResult {
  tasks: SwarmTaskUpdate[];
  status: EventStreamStatus;
}

// Watches the swarm task stream. Latest update for a given task id wins.
export function useWatchSwarmTasks({
  project,
  parentSession,
  status,
  enabled = true,
  maxItems = 500,
}: WatchTasksOptions): WatchTasksResult {
  const [tasks, setTasks] = useState<Map<string, SwarmTaskUpdate>>(new Map());

  // Reset the accumulator when filters change — otherwise stale rows from
  // the previous filter linger. Same pattern as useObserveEvents.
  const lastFilterRef = useRef("");
  const filterKey = `${parentSession ?? ""}|${status ?? ""}`;
  if (lastFilterRef.current !== filterKey) {
    lastFilterRef.current = filterKey;
    if (tasks.size > 0) setTasks(new Map());
  }

  const onEvent = useCallback(
    (t: SwarmTaskUpdate) => {
      setTasks((prev) => {
        const next = new Map(prev);
        next.set(t.id, t);
        if (next.size > maxItems) {
          const firstKey = next.keys().next().value;
          if (firstKey) next.delete(firstKey);
        }
        return next;
      });
    },
    [maxItems],
  );

  const url = useMemo(
    () =>
      project
        ? api.swarmTasksWatchUrl(project, {
            parent_session: parentSession,
            status,
          })
        : "",
    [project, parentSession, status],
  );

  const { status: streamStatus } = useEventStream<SwarmTaskUpdate>(url, {
    enabled: enabled && !!project,
    onEvent,
  });

  const list = useMemo(() => Array.from(tasks.values()), [tasks]);
  return { tasks: list, status: streamStatus };
}

interface WatchMessagesOptions {
  project: string | undefined;
  parentSession?: string;
  agent?: string;
  priority?: string;
  enabled?: boolean;
  maxItems?: number;
}

export interface WatchMessagesResult {
  messages: SwarmMessageUpdate[];
  status: EventStreamStatus;
}

export function useWatchSwarmMessages({
  project,
  parentSession,
  agent,
  priority,
  enabled = true,
  maxItems = 500,
}: WatchMessagesOptions): WatchMessagesResult {
  const [messages, setMessages] = useState<SwarmMessageUpdate[]>([]);
  const lastFilterRef = useRef("");
  const filterKey = `${parentSession ?? ""}|${agent ?? ""}|${priority ?? ""}`;
  if (lastFilterRef.current !== filterKey) {
    lastFilterRef.current = filterKey;
    if (messages.length > 0) setMessages([]);
  }
  const onEvent = useCallback(
    (m: SwarmMessageUpdate) => {
      setMessages((prev) => {
        const next = [m, ...prev.filter((p) => p.id !== m.id)];
        return next.length > maxItems ? next.slice(0, maxItems) : next;
      });
    },
    [maxItems],
  );
  const url = useMemo(
    () =>
      project
        ? api.swarmMessagesWatchUrl(project, {
            parent_session: parentSession,
            agent,
            priority,
          })
        : "",
    [project, parentSession, agent, priority],
  );
  const { status } = useEventStream<SwarmMessageUpdate>(url, {
    enabled: enabled && !!project,
    onEvent,
  });
  return { messages, status };
}

interface WatchStateOptions {
  project: string | undefined;
  agent?: string;
  keyPrefix?: string;
  enabled?: boolean;
  maxItems?: number;
}

export interface WatchStateResult {
  entries: SwarmStateUpdate[];
  status: EventStreamStatus;
}

// State stream — latest set wins per key; delete events remove the entry.
export function useWatchSwarmState({
  project,
  agent,
  keyPrefix,
  enabled = true,
  maxItems = 500,
}: WatchStateOptions): WatchStateResult {
  const [byKey, setByKey] = useState<Map<string, SwarmStateUpdate>>(new Map());
  const lastFilterRef = useRef("");
  const filterKey = `${agent ?? ""}|${keyPrefix ?? ""}`;
  if (lastFilterRef.current !== filterKey) {
    lastFilterRef.current = filterKey;
    if (byKey.size > 0) setByKey(new Map());
  }
  const onEvent = useCallback(
    (u: SwarmStateUpdate) => {
      setByKey((prev) => {
        const next = new Map(prev);
        if (u.change === "delete") {
          next.delete(u.key);
        } else {
          next.set(u.key, u);
          if (next.size > maxItems) {
            const firstKey = next.keys().next().value;
            if (firstKey) next.delete(firstKey);
          }
        }
        return next;
      });
    },
    [maxItems],
  );
  const url = useMemo(
    () =>
      project
        ? api.swarmStateWatchUrl(project, {
            agent,
            key_prefix: keyPrefix,
          })
        : "",
    [project, agent, keyPrefix],
  );
  const { status } = useEventStream<SwarmStateUpdate>(url, {
    enabled: enabled && !!project,
    onEvent,
  });
  // Sort by key so the State pane is stable across SSE arrival order
  // (Map preserves insertion order, which is non-deterministic from the
  // stream backfill).
  const entries = useMemo(
    () => Array.from(byKey.values()).sort((a, b) => a.key.localeCompare(b.key)),
    [byKey],
  );
  return { entries, status };
}
