import { useState, useMemo } from "react";
import { useParams } from "react-router-dom";
import { FilterBar } from "../shared/FilterBar";
import { LiveTailButton } from "../shared/LiveTailButton";
import { SessionFilterInput } from "../shared/SessionFilterInput";
import { useObserveEvents } from "@/hooks/useObserveEvents";
import { formatTimestamp } from "@/lib/format";
import { ChevronRight, ChevronDown } from "lucide-react";

const KIND_OPTIONS = [
  { value: "tool_call", label: "tool_call" },
  { value: "injection", label: "injection" },
  { value: "hook", label: "hook" },
  { value: "span", label: "span" },
  { value: "session", label: "session" },
];

const CATEGORY_OPTIONS = [
  { value: "modify", label: "modify" },
  { value: "execute", label: "execute" },
  { value: "search", label: "search" },
  { value: "consume", label: "consume" },
  { value: "inject", label: "inject" },
  { value: "coordinate", label: "coordinate" },
  { value: "navigate", label: "navigate" },
];

const KIND_COLOURS: Record<string, string> = {
  tool_call: "bg-aide-accent/10 text-aide-accent",
  injection: "bg-emerald-500/10 text-emerald-400",
  hook: "bg-violet-500/10 text-violet-400",
  span: "bg-amber-500/10 text-amber-400",
  session: "bg-sky-500/10 text-sky-400",
};

interface ObservePageProps {
  /** When set, locks the Kind filter and hides the Kind dropdown. */
  fixedKind?: string;
  /** Heading override; defaults to "Observe". */
  title?: string;
}

export function ObservePage({ fixedKind, title }: ObservePageProps = {}) {
  const { project } = useParams<{ project: string }>();

  const [kindFilter, setKindFilter] = useState(fixedKind ?? "");
  const [categoryFilter, setCategoryFilter] = useState("");
  const [sessionFilter, setSessionFilter] = useState("");
  const [textQuery, setTextQuery] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const {
    events: combined,
    loading,
    error,
    streamStatus,
    liveTail,
    setLiveTail,
  } = useObserveEvents({
    project,
    filters: { kind: kindFilter, category: categoryFilter, session: sessionFilter },
    maxEvents: 1000,
  });

  const filtered = useMemo(() => {
    if (!textQuery) return combined;
    const q = textQuery.toLowerCase();
    return combined.filter((e) => {
      const haystack = [
        e.name,
        e.file_path,
        e.session_id,
        e.subtype,
        e.error,
        e.attrs ? Object.entries(e.attrs).map(([k, v]) => `${k}=${v}`).join(" ") : "",
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [combined, textQuery]);

  const dropdowns = useMemo(() => {
    const list = [
      {
        value: categoryFilter,
        onChange: setCategoryFilter,
        options: CATEGORY_OPTIONS,
        placeholder: "All categories",
      },
    ];
    if (!fixedKind) {
      list.unshift({
        value: kindFilter,
        onChange: setKindFilter,
        options: KIND_OPTIONS,
        placeholder: "All kinds",
      });
    }
    return list;
  }, [kindFilter, categoryFilter, fixedKind]);

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3 flex items-center justify-between">
        <span>{title ?? "Observe"}</span>
        <span className="text-[0.65rem] font-normal text-aide-text-dim">
          {filtered.length} events
          {liveTail && (
            <span className="ml-2">· stream: {streamStatus}</span>
          )}
        </span>
      </h2>

      <FilterBar
        query={textQuery}
        onQueryChange={setTextQuery}
        placeholder="Filter by name, file, session, attrs..."
        dropdowns={dropdowns}
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

      {!loading && filtered.length > 0 && (
        <div className="border border-aide-border rounded overflow-hidden">
          {filtered.map((ev) => {
            const isOpen = !!expanded[ev.id];
            const kindClass = KIND_COLOURS[ev.kind] ?? "bg-aide-surface text-aide-text-muted";
            return (
              <div key={ev.id} className="border-b border-aide-border last:border-b-0">
                <button
                  type="button"
                  onClick={() => setExpanded((m) => ({ ...m, [ev.id]: !m[ev.id] }))}
                  className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-aide-accent/5 text-left"
                >
                  {isOpen ? (
                    <ChevronDown className="w-3 h-3 text-aide-text-dim shrink-0" />
                  ) : (
                    <ChevronRight className="w-3 h-3 text-aide-text-dim shrink-0" />
                  )}
                  <span className="text-[0.6rem] text-aide-text-dim tabular-nums w-44 shrink-0">
                    {formatTimestamp(ev.timestamp)}
                  </span>
                  <span className={`text-[0.6rem] rounded px-1.5 py-0.5 shrink-0 ${kindClass}`}>
                    {ev.kind}
                  </span>
                  {ev.category && (
                    <span className="text-[0.6rem] text-aide-text-dim shrink-0">
                      {ev.category}
                      {ev.subtype ? `/${ev.subtype}` : ""}
                    </span>
                  )}
                  <span className="text-xs text-aide-text font-mono truncate flex-1">
                    {ev.name || <em className="text-aide-text-dim">(unnamed)</em>}
                  </span>
                  {ev.tokens ? (
                    <span className="text-[0.6rem] text-aide-text-dim tabular-nums shrink-0">
                      {ev.tokens}t
                    </span>
                  ) : null}
                  {ev.file_path && (
                    <span className="text-[0.6rem] text-aide-text-dim truncate max-w-[18rem] shrink-0">
                      {ev.file_path}
                    </span>
                  )}
                </button>
                {isOpen && (
                  <pre className="bg-aide-surface text-[0.65rem] text-aide-text-muted p-3 overflow-x-auto border-t border-aide-border">
                    {JSON.stringify(ev, null, 2)}
                  </pre>
                )}
              </div>
            );
          })}
        </div>
      )}

      {!loading && filtered.length === 0 && (
        <div className="text-center py-12 text-aide-text-dim text-sm">
          No observe events match the current filters.
        </div>
      )}
    </div>
  );
}
