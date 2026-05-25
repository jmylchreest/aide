import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { api } from "@/lib/api";
import { FilterBar } from "../shared/FilterBar";
import { LiveTailButton } from "../shared/LiveTailButton";
import { SessionFilterInput } from "../shared/SessionFilterInput";
import { useObserveEvents } from "@/hooks/useObserveEvents";
import { formatTimestamp, deltaMs } from "@/lib/format";
import type { ObserveEventItem } from "@/lib/types";
import { X, ChevronDown, ChevronRight, ExternalLink } from "lucide-react";

const GROUP_BUCKET_MS = 5000;

interface InjectionGroup {
  key: string;
  firstTimestamp: string;
  lastTimestamp: string;
  source: string;
  sessionId?: string;
  totalTokens: number;
  items: ObserveEventItem[];
}

function groupInjections(events: ObserveEventItem[]): InjectionGroup[] {
  const map = new Map<string, InjectionGroup>();
  for (const ev of events) {
    const t = new Date(ev.timestamp).getTime();
    const bucket = Number.isNaN(t) ? 0 : Math.floor(t / GROUP_BUCKET_MS);
    const session = ev.session_id || "no-session";
    const source = ev.file_path || "no-source";
    const key = `${session}__${source}__${bucket}`;
    const existing = map.get(key);
    if (existing) {
      existing.items.push(ev);
      existing.totalTokens += ev.tokens ?? 0;
      if (ev.timestamp < existing.firstTimestamp) existing.firstTimestamp = ev.timestamp;
      if (ev.timestamp > existing.lastTimestamp) existing.lastTimestamp = ev.timestamp;
    } else {
      map.set(key, {
        key,
        firstTimestamp: ev.timestamp,
        lastTimestamp: ev.timestamp,
        source,
        sessionId: ev.session_id,
        totalTokens: ev.tokens ?? 0,
        items: [ev],
      });
    }
  }
  return Array.from(map.values()).sort((a, b) =>
    a.lastTimestamp > b.lastTimestamp ? -1 : 1,
  );
}

const SOURCE_OPTIONS = [
  { value: "memory", label: "memory" },
  { value: "decision", label: "decision" },
  { value: "skill", label: "skill" },
  { value: "enrichment", label: "enrichment" },
];

const SOURCE_COLOURS: Record<string, string> = {
  memory: "bg-sky-500/10 text-sky-400",
  decision: "bg-violet-500/10 text-violet-400",
  skill: "bg-emerald-500/10 text-emerald-400",
  enrichment: "bg-amber-500/10 text-amber-400",
};

const NEXT_CALLS_WINDOW = 10;

export function InjectionLogPage() {
  const { project } = useParams<{ project: string }>();

  const [sourceFilter, setSourceFilter] = useState("");
  const [sessionFilter, setSessionFilter] = useState("");
  const [textQuery, setTextQuery] = useState("");
  const [selectedID, setSelectedID] = useState<string | null>(null);

  const {
    events: combined,
    loading,
    error,
    streamStatus,
    liveTail,
    setLiveTail,
  } = useObserveEvents({
    project,
    filters: { kind: "injection", session: sessionFilter },
    maxEvents: 500,
  });

  const filtered = useMemo(() => {
    let list = combined;
    if (sourceFilter) {
      list = list.filter(
        (e) => e.subtype === sourceFilter || e.attrs?.source_kind === sourceFilter,
      );
    }
    if (textQuery) {
      const q = textQuery.toLowerCase();
      list = list.filter((e) => {
        const haystack = [
          e.name,
          e.session_id,
          e.subtype,
          e.attrs?.content_preview,
          e.attrs?.source_id,
        ]
          .filter(Boolean)
          .join(" ")
          .toLowerCase();
        return haystack.includes(q);
      });
    }
    return list;
  }, [combined, sourceFilter, textQuery]);

  const selected = useMemo(
    () => filtered.find((e) => e.id === selectedID) ?? null,
    [filtered, selectedID],
  );

  const groups = useMemo(() => groupInjections(filtered), [filtered]);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const toggleGroup = useCallback((key: string) => {
    setCollapsed((m) => ({ ...m, [key]: !m[key] }));
  }, []);

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3 flex items-center justify-between">
        <span>Injections</span>
        <span className="text-[0.65rem] font-normal text-aide-text-dim">
          {filtered.length} events
          {liveTail && <span className="ml-2">· stream: {streamStatus}</span>}
        </span>
      </h2>

      <FilterBar
        query={textQuery}
        onQueryChange={setTextQuery}
        placeholder="Filter by name, session, content preview, source id..."
        dropdowns={[
          {
            value: sourceFilter,
            onChange: setSourceFilter,
            options: SOURCE_OPTIONS,
            placeholder: "All sources",
          },
        ]}
        right={
          <>
            <SessionFilterInput value={sessionFilter} onChange={setSessionFilter} />
            <LiveTailButton
              active={liveTail}
              onToggle={() => setLiveTail((v) => !v)}
              status={streamStatus}
            />
          </>
        }
      />

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {!loading && groups.length > 0 && (
        <div
          className={
            "grid gap-3 items-start " +
            (selected ? "grid-cols-[1fr_1fr]" : "grid-cols-1")
          }
        >
          <div className="space-y-2">
            {groups.map((group) => {
              const isCollapsed = !!collapsed[group.key];
              return (
                <div
                  key={group.key}
                  className="border border-aide-border rounded overflow-hidden"
                >
                  <button
                    type="button"
                    onClick={() => toggleGroup(group.key)}
                    className="w-full flex items-center gap-2 px-3 py-1.5 text-left bg-aide-surface hover:bg-aide-accent/5"
                  >
                    {isCollapsed ? (
                      <ChevronRight className="w-3 h-3 text-aide-text-dim shrink-0" />
                    ) : (
                      <ChevronDown className="w-3 h-3 text-aide-text-dim shrink-0" />
                    )}
                    <span className="text-[0.6rem] text-aide-text-dim tabular-nums w-44 shrink-0">
                      {formatTimestamp(group.firstTimestamp)}
                    </span>
                    <span className="text-xs text-aide-text font-mono truncate flex-1">
                      {group.source}
                    </span>
                    {group.sessionId && (
                      <span
                        className="text-[0.6rem] text-aide-text-dim font-mono shrink-0"
                        title={group.sessionId}
                      >
                        {group.sessionId.slice(0, 8)}
                      </span>
                    )}
                    <span className="text-[0.6rem] text-aide-text-dim shrink-0">
                      {group.items.length} item{group.items.length === 1 ? "" : "s"}
                    </span>
                    {group.totalTokens > 0 && (
                      <span className="text-[0.6rem] text-aide-text-dim tabular-nums shrink-0">
                        {group.totalTokens}t
                      </span>
                    )}
                  </button>
                  {!isCollapsed && (
                    <div>
                      {group.items.map((ev) => {
                        const source = ev.subtype || ev.attrs?.source_kind || "?";
                        const colourClass =
                          SOURCE_COLOURS[source] ?? "bg-aide-surface text-aide-text-muted";
                        const isSelected = ev.id === selectedID;
                        const score = ev.attrs?.score_at_injection;
                        return (
                          <button
                            key={ev.id}
                            type="button"
                            onClick={() =>
                              setSelectedID((cur) => (cur === ev.id ? null : ev.id))
                            }
                            className={
                              "w-full flex items-center gap-2 pl-8 pr-3 py-1.5 text-left border-t border-aide-border " +
                              (isSelected ? "bg-aide-accent/10" : "hover:bg-aide-accent/5")
                            }
                          >
                            <span className={`text-[0.6rem] rounded px-1.5 py-0.5 shrink-0 ${colourClass}`}>
                              {source}
                            </span>
                            <span className="text-xs text-aide-text font-mono truncate flex-1">
                              {ev.name || <em className="text-aide-text-dim">(unnamed)</em>}
                            </span>
                            {score && (
                              <span
                                className="text-[0.6rem] text-aide-text-dim tabular-nums shrink-0"
                                title="score at injection"
                              >
                                ★{score}
                              </span>
                            )}
                            {ev.tokens ? (
                              <span className="text-[0.6rem] text-aide-text-dim tabular-nums shrink-0">
                                {ev.tokens}t
                              </span>
                            ) : null}
                          </button>
                        );
                      })}
                    </div>
                  )}
                </div>
              );
            })}
          </div>

          {selected && project && (
            <div className="sticky top-2 max-h-[calc(100vh-6rem)] overflow-auto">
              <InjectionDetail
                event={selected}
                project={project}
                onClose={() => setSelectedID(null)}
              />
            </div>
          )}
        </div>
      )}

      {!loading && groups.length === 0 && (
        <div className="text-center py-12 text-aide-text-dim text-sm">
          No injections match the current filters.
        </div>
      )}
    </div>
  );
}

function sourceLinkFor(
  project: string,
  event: ObserveEventItem,
): { to: string; label: string } | null {
  const kind = event.subtype || event.attrs?.source_kind;
  const id = event.attrs?.source_id;
  if (!id) return null;
  if (kind === "memory") {
    return {
      to: `/instances/${encodeURIComponent(project)}/memories?q=${encodeURIComponent(id)}`,
      label: "View memory",
    };
  }
  if (kind === "decision") {
    return {
      to: `/instances/${encodeURIComponent(project)}/decisions?q=${encodeURIComponent(id)}`,
      label: "View decision",
    };
  }
  return null;
}

interface InjectionDetailProps {
  event: ObserveEventItem;
  project: string;
  onClose: () => void;
}

function InjectionDetail({ event, project, onClose }: InjectionDetailProps) {
  const [nextCalls, setNextCalls] = useState<ObserveEventItem[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!event.session_id) {
      setNextCalls([]);
      return;
    }
    setLoading(true);
    api
      .listObserveEvents(project, {
        session: event.session_id,
        kind: "tool_call",
        since: event.timestamp,
        limit: NEXT_CALLS_WINDOW + 1,
      })
      .then((events) => {
        // Since= is inclusive; drop the anchor event itself.
        const after = events
          .filter((e) => e.id !== event.id)
          .slice(0, NEXT_CALLS_WINDOW)
          .sort((a, b) => (a.timestamp > b.timestamp ? 1 : -1));
        setNextCalls(after);
      })
      .catch(() => setNextCalls([]))
      .finally(() => setLoading(false));
  }, [event.id, event.timestamp, event.session_id, project]);

  const sourceLink = sourceLinkFor(project, event);

  return (
    <div className="border border-aide-border rounded">
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-aide-border bg-aide-surface">
        <span className="text-xs font-medium text-aide-text-muted">
          {event.name} · {event.timestamp}
        </span>
        <button
          type="button"
          onClick={onClose}
          className="text-aide-text-dim hover:text-aide-text"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>

      {sourceLink && (
        <div className="px-3 py-1.5 border-b border-aide-border">
          <Link
            to={sourceLink.to}
            className="inline-flex items-center gap-1 text-xs text-aide-accent hover:underline"
          >
            <ExternalLink className="w-3 h-3" />
            {sourceLink.label}
          </Link>
        </div>
      )}

      <section className="p-3 border-b border-aide-border">
        <h3 className="text-[0.65rem] uppercase tracking-wide text-aide-text-dim mb-1.5">
          Attributes
        </h3>
        {event.attrs && Object.keys(event.attrs).length > 0 ? (
          <dl className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 text-xs">
            {Object.entries(event.attrs).map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-aide-text-dim font-mono">{k}</dt>
                <dd className="text-aide-text break-words">{v}</dd>
              </div>
            ))}
          </dl>
        ) : (
          <p className="text-xs text-aide-text-dim">No attrs recorded.</p>
        )}
      </section>

      <section className="p-3 border-b border-aide-border">
        <h3 className="text-[0.65rem] uppercase tracking-wide text-aide-text-dim mb-1.5">
          Content preview
        </h3>
        {event.attrs?.content_preview ? (
          <p className="text-xs text-aide-text-muted whitespace-pre-wrap font-mono">
            {event.attrs.content_preview}
          </p>
        ) : (
          <p className="text-xs text-aide-text-dim">
            No preview recorded. (Emitter has not been upgraded to include
            content_preview in attrs.)
          </p>
        )}
      </section>

      <section className="p-3">
        <h3 className="text-[0.65rem] uppercase tracking-wide text-aide-text-dim mb-1.5">
          Next {NEXT_CALLS_WINDOW} tool calls in session
        </h3>
        {!event.session_id ? (
          <p className="text-xs text-aide-text-dim">
            No session id — cannot correlate.
          </p>
        ) : loading ? (
          <p className="text-xs text-aide-text-dim">Loading...</p>
        ) : nextCalls.length === 0 ? (
          <p className="text-xs text-aide-text-dim">
            No tool calls recorded after this injection in this session.
          </p>
        ) : (
          <ol className="space-y-1">
            {nextCalls.map((call) => (
              <li
                key={call.id}
                className="flex items-center gap-2 text-xs text-aide-text-muted"
              >
                <span className="text-[0.6rem] text-aide-text-dim tabular-nums">
                  +{deltaMs(event.timestamp, call.timestamp)}ms
                </span>
                <span className="font-mono text-aide-text">{call.name}</span>
                {call.file_path && (
                  <span className="text-[0.6rem] text-aide-text-dim truncate">
                    {call.file_path}
                  </span>
                )}
                {call.tokens ? (
                  <span className="text-[0.6rem] text-aide-text-dim tabular-nums ml-auto">
                    {call.tokens}t
                  </span>
                ) : null}
              </li>
            ))}
          </ol>
        )}
      </section>
    </div>
  );
}

