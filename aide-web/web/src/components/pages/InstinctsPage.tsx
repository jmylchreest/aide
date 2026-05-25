import { useCallback, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { useEventStream } from "@/hooks/useEventStream";
import { FilterBar } from "../shared/FilterBar";
import { LiveTailButton } from "../shared/LiveTailButton";
import { formatTimestamp } from "@/lib/format";
import type { InstinctProposalItem, InstinctStatus } from "@/lib/types";
import { Check, X, ChevronRight, ChevronDown, Sparkles } from "lucide-react";

type Tab = "open" | "accepted" | "rejected";

const TAB_LABEL: Record<Tab, string> = {
  open: "Proposals",
  accepted: "Accepted",
  rejected: "Rejected",
};

const TAB_STATUS: Record<Tab, InstinctStatus> = {
  open: "open",
  accepted: "accepted",
  rejected: "rejected",
};

const SHAPE_COLOURS: Record<string, string> = {
  repetition: "bg-amber-500/10 text-amber-400",
  convergence: "bg-emerald-500/10 text-emerald-400",
};

export function InstinctsPage() {
  const { project } = useParams<{ project: string }>();
  const [tab, setTab] = useState<Tab>("open");
  const [textQuery, setTextQuery] = useState("");
  const [shapeFilter, setShapeFilter] = useState("");
  const [liveTail, setLiveTail] = useState(false);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const {
    data: initial,
    loading,
    error,
    refresh,
  } = useApi(
    () =>
      project
        ? api.listInstinctProposals(project, {
            status: TAB_STATUS[tab],
            shape: shapeFilter || undefined,
            limit: 200,
          })
        : Promise.resolve([] as InstinctProposalItem[]),
    [project, tab, shapeFilter],
  );

  const [liveAdditions, setLiveAdditions] = useState<InstinctProposalItem[]>([]);
  const handleStreamEvent = useCallback(
    (p: InstinctProposalItem) => {
      if (p.status !== TAB_STATUS[tab]) {
        // Status transition — re-fetch the current tab so accepted/rejected
        // items appear in their new bucket without a manual reload.
        refresh();
        return;
      }
      setLiveAdditions((prev) => [p, ...prev.filter((x) => x.id !== p.id)]);
    },
    [tab, refresh],
  );

  const watchUrl = useMemo(
    () =>
      project
        ? api.instinctWatchUrl(project, { status: TAB_STATUS[tab], shape: shapeFilter || undefined })
        : "",
    [project, tab, shapeFilter],
  );
  const { status: streamStatus } = useEventStream<InstinctProposalItem>(watchUrl, {
    enabled: liveTail && !!project,
    onEvent: handleStreamEvent,
  });

  const combined = useMemo(() => {
    const seen = new Set<string>();
    const out: InstinctProposalItem[] = [];
    for (const p of liveAdditions) {
      if (!seen.has(p.id)) {
        seen.add(p.id);
        out.push(p);
      }
    }
    for (const p of initial ?? []) {
      if (!seen.has(p.id)) {
        seen.add(p.id);
        out.push(p);
      }
    }
    return out;
  }, [liveAdditions, initial]);

  const filtered = useMemo(() => {
    if (!textQuery) return combined;
    const q = textQuery.toLowerCase();
    return combined.filter((p) => {
      const hay = [
        p.summary,
        p.shape,
        p.session_id,
        p.proposed_instinct.content,
        p.rejection_reason,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();
      return hay.includes(q);
    });
  }, [combined, textQuery]);

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3 flex items-center justify-between">
        <span className="inline-flex items-center gap-2">
          <Sparkles className="w-4 h-4 text-aide-accent" />
          Instincts
        </span>
        <span className="text-[0.65rem] font-normal text-aide-text-dim">
          {filtered.length} in {TAB_LABEL[tab].toLowerCase()}
          {liveTail && <span className="ml-2">· stream: {streamStatus}</span>}
        </span>
      </h2>

      <div className="flex items-center gap-1 mb-3 border-b border-aide-border">
        {(Object.keys(TAB_LABEL) as Tab[]).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => {
              setTab(t);
              setLiveAdditions([]);
            }}
            className={
              "px-3 py-1.5 text-xs border-b-2 transition " +
              (tab === t
                ? "border-aide-accent text-aide-accent"
                : "border-transparent text-aide-text-muted hover:text-aide-text")
            }
          >
            {TAB_LABEL[t]}
          </button>
        ))}
      </div>

      <FilterBar
        query={textQuery}
        onQueryChange={setTextQuery}
        placeholder="Filter by summary, content, session..."
        dropdowns={[
          {
            value: shapeFilter,
            onChange: setShapeFilter,
            options: [
              { value: "repetition", label: "repetition" },
              { value: "convergence", label: "convergence" },
            ],
            placeholder: "All shapes",
          },
        ]}
        right={
          <LiveTailButton
            active={liveTail}
            onToggle={() => setLiveTail((v) => !v)}
            status={streamStatus}
          />
        }
      />

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {!loading && filtered.length === 0 && (
        <div className="text-center py-12 text-aide-text-dim text-sm">
          No proposals in this tab.
        </div>
      )}

      {!loading && filtered.length > 0 && (
        <div className="border border-aide-border rounded overflow-hidden">
          {filtered.map((p) => (
            <ProposalRow
              key={p.id}
              proposal={p}
              tab={tab}
              expanded={!!expanded[p.id]}
              onToggle={() => setExpanded((m) => ({ ...m, [p.id]: !m[p.id] }))}
              onChange={refresh}
              project={project!}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface ProposalRowProps {
  proposal: InstinctProposalItem;
  tab: Tab;
  expanded: boolean;
  onToggle: () => void;
  onChange: () => void;
  project: string;
}

function ProposalRow({ proposal, tab, expanded, onToggle, onChange, project }: ProposalRowProps) {
  const [busy, setBusy] = useState<"accept" | "reject" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editContent, setEditContent] = useState<string | null>(null);
  const [rejectReason, setRejectReason] = useState("");

  const colourClass = SHAPE_COLOURS[proposal.shape] ?? "bg-aide-surface text-aide-text-muted";

  async function doAccept() {
    setBusy("accept");
    setError(null);
    try {
      await api.acceptInstinctProposal(project, proposal.id, editContent ?? undefined);
      onChange();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  }

  async function doReject() {
    setBusy("reject");
    setError(null);
    try {
      await api.rejectInstinctProposal(project, proposal.id, rejectReason || undefined);
      onChange();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="border-b border-aide-border last:border-b-0">
      <button
        type="button"
        onClick={onToggle}
        className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-aide-accent/5 text-left"
      >
        {expanded ? (
          <ChevronDown className="w-3 h-3 text-aide-text-dim shrink-0" />
        ) : (
          <ChevronRight className="w-3 h-3 text-aide-text-dim shrink-0" />
        )}
        <span className="text-[0.6rem] text-aide-text-dim tabular-nums w-44 shrink-0">
          {formatTimestamp(proposal.proposed_at)}
        </span>
        <span className={`text-[0.6rem] rounded px-1.5 py-0.5 shrink-0 ${colourClass}`}>
          {proposal.shape}
        </span>
        <span className="text-xs text-aide-text flex-1 truncate">{proposal.summary}</span>
        {proposal.rejection_count ? (
          <span className="text-[0.6rem] text-aide-text-dim shrink-0">×{proposal.rejection_count}</span>
        ) : null}
      </button>

      {expanded && (
        <div className="px-3 pb-3 bg-aide-surface/40 space-y-2 border-t border-aide-border">
          <section className="pt-2">
            <h3 className="text-[0.65rem] uppercase tracking-wide text-aide-text-dim mb-1">
              Proposed memory
            </h3>
            {tab === "open" ? (
              <textarea
                value={editContent ?? proposal.proposed_instinct.content}
                onChange={(e) => setEditContent(e.target.value)}
                rows={4}
                className="w-full bg-aide-surface border border-aide-border rounded px-2 py-1 text-xs text-aide-text font-mono focus:border-aide-accent outline-none"
              />
            ) : (
              <p className="text-xs text-aide-text-muted whitespace-pre-wrap font-mono">
                {proposal.proposed_instinct.content}
              </p>
            )}
            <div className="text-[0.6rem] text-aide-text-dim mt-1">
              tags: {proposal.proposed_instinct.tags?.join(", ") || "(none)"} · priority:{" "}
              {proposal.proposed_instinct.priority?.toFixed(2) ?? "—"}
            </div>
          </section>

          {proposal.evidence.snapshot && proposal.evidence.snapshot.length > 0 && (
            <section>
              <h3 className="text-[0.65rem] uppercase tracking-wide text-aide-text-dim mb-1">
                Evidence snapshot ({proposal.evidence.snapshot.length})
              </h3>
              <ol className="space-y-0.5">
                {proposal.evidence.snapshot.map((ev) => (
                  <li
                    key={ev.id}
                    className="flex items-center gap-2 text-[0.65rem] text-aide-text-muted"
                  >
                    <span className="text-aide-text-dim tabular-nums">
                      {formatTimestamp(ev.timestamp)}
                    </span>
                    <span className="font-mono text-aide-text">{ev.name}</span>
                    {ev.file_path && (
                      <span className="text-aide-text-dim truncate">{ev.file_path}</span>
                    )}
                  </li>
                ))}
              </ol>
            </section>
          )}

          {tab === "rejected" && proposal.rejection_reason && (
            <section>
              <h3 className="text-[0.65rem] uppercase tracking-wide text-aide-text-dim mb-1">
                Rejection reason
              </h3>
              <p className="text-xs text-aide-text-muted">{proposal.rejection_reason}</p>
            </section>
          )}

          {tab === "accepted" && proposal.accepted_memory_id && (
            <section>
              <h3 className="text-[0.65rem] uppercase tracking-wide text-aide-text-dim mb-1">
                Promoted to memory
              </h3>
              <a
                href={`/instances/${encodeURIComponent(project)}/memories?q=${encodeURIComponent(proposal.accepted_memory_id)}`}
                className="text-xs text-aide-accent hover:underline font-mono"
              >
                {proposal.accepted_memory_id} →
              </a>
            </section>
          )}

          {tab === "open" && (
            <div className="flex items-center gap-2 pt-1">
              <button
                type="button"
                onClick={doAccept}
                disabled={busy !== null}
                className="inline-flex items-center gap-1 rounded px-2.5 py-1 text-xs bg-aide-accent text-white hover:opacity-90 disabled:opacity-50"
              >
                <Check className="w-3 h-3" />
                {busy === "accept" ? "Accepting..." : "Accept"}
              </button>
              <input
                type="text"
                placeholder="Rejection reason (optional)"
                value={rejectReason}
                onChange={(e) => setRejectReason(e.target.value)}
                className="bg-aide-surface border border-aide-border rounded px-2 py-1 text-xs flex-1 outline-none focus:border-aide-accent"
              />
              <button
                type="button"
                onClick={doReject}
                disabled={busy !== null}
                className="inline-flex items-center gap-1 rounded px-2.5 py-1 text-xs border border-aide-border text-aide-text hover:bg-aide-accent/5 disabled:opacity-50"
              >
                <X className="w-3 h-3" />
                {busy === "reject" ? "Rejecting..." : "Reject"}
              </button>
            </div>
          )}

          {error && <p className="text-aide-red text-xs">{error}</p>}
        </div>
      )}
    </div>
  );
}
