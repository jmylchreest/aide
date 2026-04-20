import { useState, useMemo } from "react";
import { useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { FilterBar } from "../shared/FilterBar";
import { SortableTable, type Column } from "../shared/SortableTable";
import { DateRangePicker, presetToRange, type DateRangeValue } from "../shared/DateRangePicker";
import { CodeViewer } from "../shared/CodeViewer";
import type { InstanceInfo, TokenEventItem } from "@/lib/types";

/**
 * Heuristic: is this `file_path` value likely a real on-disk file (vs. a
 * source label like "session-start" or "skill-injector")? Real file paths
 * either contain a slash, OR have an extension. Source labels are short
 * single tokens with no slash and no dot.
 */
function looksLikeFilePath(s: string): boolean {
  if (!s) return false;
  if (s.includes("/")) return true;
  // file.ext form
  return /\.[a-z0-9]+$/i.test(s);
}

/**
 * Trim the project_root prefix off an absolute path so the visible label
 * is meaningfully short. No-op when the path is already relative or the
 * root doesn't match.
 */
function relativeToRoot(filePath: string, root?: string): string {
  if (!root || !filePath.startsWith(root)) return filePath;
  const rest = filePath.slice(root.length);
  return rest.startsWith("/") ? rest.slice(1) : rest;
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

function StatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-md border border-aide-border bg-aide-bg-secondary px-4 py-3">
      <div className="text-[10px] uppercase tracking-wider text-aide-text-dim mb-1">{label}</div>
      <div className="text-xl font-semibold text-aide-text">{value}</div>
      {sub && <div className="text-[10px] text-aide-text-muted mt-0.5">{sub}</div>}
    </div>
  );
}

function DeliveryCard({ label, value, tooltip }: { label: string; value: number; tooltip?: string }) {
  return (
    <div className="rounded-md border border-aide-border bg-aide-bg-secondary px-4 py-3">
      <div className="text-[10px] uppercase tracking-wider text-aide-text-dim mb-1 cursor-help" title={tooltip}>{label}</div>
      <div className="text-lg font-semibold text-blue-400">~{formatTokens(value)}</div>
      <div className="text-[10px] text-aide-text-muted mt-0.5">tokens delivered</div>
    </div>
  );
}

// TOOL_CATEGORIES mirrors the observe taxonomy. "consume" tools replace a
// Read and so have a meaningful "avoided" counterfactual. "navigate" and
// "search" tools spend tokens to find things; their value is indirect
// (smaller downstream Reads) — we don't claim savings for them because
// we can't ground the counterfactual.
const TOOL_CATEGORIES: Record<string, "consume" | "navigate" | "search" | "modify" | "execute" | "network"> = {
  Read: "consume",
  code_outline: "consume",
  code_read_symbol: "consume",
  code_search: "navigate",
  code_symbols: "navigate",
  code_references: "navigate",
  code_top_references: "navigate",
  code_read_check: "navigate",
  code_stats: "navigate",
  Grep: "search",
  Glob: "search",
  Edit: "modify",
  Write: "modify",
  NotebookEdit: "modify",
  Bash: "execute",
  WebFetch: "network",
  WebSearch: "network",
};

function ToolCategoryBadge({ category }: { category: string }) {
  const colors: Record<string, string> = {
    consume: "bg-emerald-500/10 text-emerald-400 border-emerald-500/20",
    navigate: "bg-sky-500/10 text-sky-400 border-sky-500/20",
    search: "bg-amber-500/10 text-amber-400 border-amber-500/20",
    modify: "bg-rose-500/10 text-rose-400 border-rose-500/20",
    execute: "bg-purple-500/10 text-purple-400 border-purple-500/20",
    network: "bg-slate-500/10 text-slate-400 border-slate-500/20",
    other: "bg-aide-border/20 text-aide-text-muted border-aide-border",
  };
  const cls = colors[category] ?? colors.other;
  return (
    <span className={`inline-block px-1.5 py-0.5 rounded text-[9px] font-medium border ${cls}`}>
      {category}
    </span>
  );
}

interface PerToolStat {
  tool: string;
  category: string;
  calls: number;
  spent: number;
  avoided: number;
  /** code_search-only: how many calls were *not* followed by a Grep within
   *  the comparison window (treated as "satisfied" — the index answered
   *  the question and the agent didn't fall back to raw text search). */
  satisfied?: number;
}

/** Window after a code_search in which a follow-up Grep is treated as
 *  evidence the index didn't answer the question. */
const CODE_SEARCH_FOLLOWUP_WINDOW_MS = 60_000;

/**
 * Tools that produce a meaningful "avoided" counterfactual.
 *
 * - code_outline / code_read_symbol: pull a subset of a file the agent
 *   could otherwise have Read whole — counterfactual = what Read would
 *   have cost, set on the recording side.
 * - code_search: counterfactual is "would the agent have run Grep
 *   instead?" — computed client-side by checking whether a Grep
 *   followed within CODE_SEARCH_FOLLOWUP_WINDOW_MS in the same session.
 *
 * Raw Read is consume but cannot claim avoided — it IS the expensive
 * path, with nothing cheaper to compare against.
 */
const AVOIDED_CLAIM_TOOLS = new Set([
  "code_outline",
  "code_read_symbol",
  "code_search",
]);

function PerToolEfficiency({ stats }: { stats: PerToolStat[] }) {
  if (stats.length === 0) return null;
  const max = Math.max(1, ...stats.map((s) => Math.max(s.spent, s.avoided)));
  return (
    <div className="space-y-2">
      {stats.map((s) => {
        const avoidedClaimable = AVOIDED_CLAIM_TOOLS.has(s.tool);
        const spentPct = (s.spent / max) * 100;
        const avoidedPct = (s.avoided / max) * 100;
        const totalCounterfactual = s.spent + s.avoided;
        const eff =
          avoidedClaimable && totalCounterfactual > 0
            ? ((s.avoided / totalCounterfactual) * 100).toFixed(1) + "%"
            : null;
        return (
          <div
            key={s.tool}
            className="rounded-md border border-aide-border bg-aide-bg-secondary px-3 py-2"
          >
            <div className="flex items-center gap-2 mb-1.5">
              <span className="font-mono text-xs text-aide-text font-medium">{s.tool}</span>
              <ToolCategoryBadge category={s.category} />
              <span className="text-[10px] text-aide-text-dim">
                {s.calls} {s.calls === 1 ? "call" : "calls"}
              </span>
              {s.satisfied !== undefined && (
                <span
                  className="text-[10px] text-aide-text-dim"
                  title="Calls not followed by a Grep within 60s in the same session"
                >
                  {s.satisfied}/{s.calls} satisfied
                </span>
              )}
              {eff && (
                <span className="ml-auto text-[10px] text-green-500 font-medium">
                  {eff} efficiency
                </span>
              )}
              {!avoidedClaimable && (
                <span className="ml-auto text-[10px] text-aide-text-dim italic">
                  indirect value — no avoided-read claim
                </span>
              )}
            </div>
            <div className="grid grid-cols-[60px_1fr_auto] gap-2 items-center text-[10px] mb-1">
              <span className="text-aide-text-dim">spent</span>
              <div className="h-2 bg-aide-bg rounded-sm overflow-hidden">
                <div
                  className="h-full bg-blue-500/70"
                  style={{ width: `${spentPct}%` }}
                />
              </div>
              <span className="font-mono text-aide-text-muted w-16 text-right">
                {s.spent > 0 ? `~${formatTokens(s.spent)}` : "-"}
              </span>
            </div>
            {avoidedClaimable && (
              <div className="grid grid-cols-[60px_1fr_auto] gap-2 items-center text-[10px]">
                <span className="text-aide-text-dim">avoided</span>
                <div className="h-2 bg-aide-bg rounded-sm overflow-hidden">
                  <div
                    className="h-full bg-green-500/80"
                    style={{ width: `${avoidedPct}%` }}
                  />
                </div>
                <span className="font-mono text-green-500 w-16 text-right">
                  {s.avoided > 0 ? `~${formatTokens(s.avoided)}` : "-"}
                </span>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

export function TokensPage() {
  const { project } = useParams<{ project: string }>();
  const [query, setQuery] = useState("");
  const [toolFilter, setToolFilter] = useState("");
  const [viewer, setViewer] = useState<{
    file: string;
    line?: number;
    endLine?: number;
  } | null>(null);

  // Used to make absolute file paths relative for display in the Source
  // column. Cheap to fetch and shared across many places in the dashboard.
  const { data: instances } = useApi(() => api.listInstances());
  const projectRoot = useMemo(
    () =>
      instances?.find((i: InstanceInfo) => i.project_name === project)
        ?.project_root,
    [instances, project],
  );

  // Default to last 30 days
  const [dateRange, setDateRange] = useState<DateRangeValue>(() => {
    const { since, until } = presetToRange("30d");
    return { preset: "30d", since, until };
  });

  const { data: stats, loading: statsLoading } = useApi(
    () => api.getTokenStats(project!, undefined, dateRange.since || undefined, dateRange.until || undefined),
    [project, dateRange.since, dateRange.until],
  );

  const { data: events, loading: eventsLoading } = useApi(
    () => api.listTokenEvents(project!, undefined, 200, dateRange.since || undefined, dateRange.until || undefined),
    [project, dateRange.since, dateRange.until],
  );

  const filteredEvents = useMemo(() => {
    if (!events) return [];
    return events.filter((e) => {
      if (toolFilter && e.tool !== toolFilter) return false;
      if (query) {
        const q = query.toLowerCase();
        return (
          e.file_path.toLowerCase().includes(q) ||
          e.tool.toLowerCase().includes(q) ||
          e.event_type.toLowerCase().includes(q)
        );
      }
      return true;
    });
  }, [events, query, toolFilter]);

  const toolOptions = useMemo(() => {
    if (!events) return [];
    return [...new Set(events.map((e) => e.tool))]
      .sort()
      .map((t) => ({ value: t, label: t }));
  }, [events]);

  const perToolStats = useMemo<PerToolStat[]>(() => {
    if (!events) return [];

    // Build a session-keyed timeline of Grep events so we can do an O(N)
    // walk over code_search calls and binary-search the next Grep per
    // session. Events come back newest-first from the API; we want oldest
    // first for forward-looking counterfactual checks.
    const ordered = [...events].sort(
      (a, b) =>
        new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
    );
    const grepBySession = new Map<string, number[]>();
    for (const e of ordered) {
      if (e.tool === "Grep") {
        const arr = grepBySession.get(e.session_id) ?? [];
        arr.push(new Date(e.timestamp).getTime());
        grepBySession.set(e.session_id, arr);
      }
    }
    function followedByGrep(e: TokenEventItem): boolean {
      const greps = grepBySession.get(e.session_id);
      if (!greps) return false;
      const t = new Date(e.timestamp).getTime();
      // Linear scan is fine — typical sessions have few Greps. If this
      // becomes hot, swap in a binary search.
      for (const gt of greps) {
        if (gt > t && gt - t <= CODE_SEARCH_FOLLOWUP_WINDOW_MS) return true;
        if (gt > t) break;
      }
      return false;
    }

    const m = new Map<string, PerToolStat>();
    for (const e of ordered) {
      if (!e.tool) continue;
      // Injection events use the Tool field as the source name (memory /
      // decision / skill / enrichment) — those belong in the delivered
      // section, not the per-tool efficiency chart. Filter them out.
      if (e.event_type === "context_injected") continue;
      const category = TOOL_CATEGORIES[e.tool] ?? "other";
      const b =
        m.get(e.tool) ??
        ({ tool: e.tool, category, calls: 0, spent: 0, avoided: 0 } as PerToolStat);
      b.calls += 1;
      b.spent += e.tokens || 0;
      b.avoided += e.tokens_saved || 0;

      // code_search counterfactual: if this call wasn't followed by a Grep
      // in the same session within the window, treat it as "satisfied" —
      // the index answered the question. Avoided estimate = 3× spent
      // (conservative; a Grep on the same project usually returns several
      // times the bytes a focused symbol search does).
      if (e.tool === "code_search" && !followedByGrep(e)) {
        b.satisfied = (b.satisfied ?? 0) + 1;
        b.avoided += (e.tokens || 0) * 3;
      }
      m.set(e.tool, b);
    }
    return Array.from(m.values()).sort((a, b) => {
      // Consume tools first (they're the efficiency story), sorted by avoided desc.
      // Then everything else sorted by activity (calls desc).
      const aConsume = a.category === "consume" ? 1 : 0;
      const bConsume = b.category === "consume" ? 1 : 0;
      if (aConsume !== bConsume) return bConsume - aConsume;
      if (aConsume) return b.avoided - a.avoided;
      return b.calls - a.calls;
    });
  }, [events]);

  const savingsPct =
    stats && stats.total_read + stats.total_saved > 0
      ? ((stats.total_saved / (stats.total_read + stats.total_saved)) * 100).toFixed(1)
      : "0";

  const totalFileInteractions = (stats?.read_count ?? 0) + (stats?.code_tool_count ?? 0);
  const adoptionPct =
    totalFileInteractions > 0
      ? ((stats!.code_tool_count / totalFileInteractions) * 100).toFixed(0)
      : null;

  const columns: Column<TokenEventItem>[] = [
    {
      key: "timestamp",
      label: "Time",
      render: (row) => (
        <span className="text-aide-text-dim text-[11px] font-mono">
          {new Date(row.timestamp).toLocaleString()}
        </span>
      ),
      sortValue: (row) => row.timestamp,
    },
    {
      key: "tool",
      label: "Tool",
      render: (row) => (
        <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-medium bg-aide-accent/10 text-aide-accent">
          {row.tool}
        </span>
      ),
    },
    {
      key: "event_type",
      label: "Type",
      render: (row) => (
        <span className="text-aide-text-muted text-xs">{row.event_type}</span>
      ),
    },
    {
      key: "tokens",
      label: "Est. Tokens",
      render: (row) => (
        <span className="font-mono text-xs">{row.tokens > 0 ? `~${row.tokens}` : "-"}</span>
      ),
      sortValue: (row) => row.tokens,
    },
    {
      key: "tokens_saved",
      label: "Est. Saved",
      render: (row) => (
        <span className="font-mono text-xs text-green-500">
          {row.tokens_saved > 0 ? `~${row.tokens_saved}` : "-"}
        </span>
      ),
      sortValue: (row) => row.tokens_saved,
    },
    {
      // Column carries either a real file path (Read/Edit/code_*) or a
      // source label (session-start, skill-injector, ...) — hence "Source".
      key: "file_path",
      label: "Source",
      render: (row) => {
        const value = row.file_path;
        if (!value) {
          return <span className="text-aide-text-dim text-[11px]">-</span>;
        }
        const clickable = looksLikeFilePath(value) && !!project;
        // Strip the project root prefix so common leading path is gone and
        // the filename is what reads. Tooltip keeps the full absolute value.
        const display = relativeToRoot(value, projectRoot);
        // RTL truncation trick: when overflow happens we want the *start*
        // ellipsised so the filename stays visible. direction:rtl with
        // unicode-bidi:plaintext keeps the text order untouched while
        // anchoring the overflow on the left edge.
        const truncClasses =
          "font-mono text-[11px] block max-w-[320px] overflow-hidden whitespace-nowrap text-ellipsis [direction:rtl] [unicode-bidi:plaintext] text-left";
        if (!clickable) {
          return (
            <span
              title={value}
              className={`${truncClasses} text-aide-text-dim`}
            >
              {display}
            </span>
          );
        }
        const lineSuffix =
          row.start_line && row.end_line && row.end_line > row.start_line
            ? `:${row.start_line}-${row.end_line}`
            : row.start_line
              ? `:${row.start_line}`
              : "";
        return (
          <button
            type="button"
            title={value + lineSuffix}
            onClick={() =>
              setViewer({
                file: value,
                line: row.start_line || undefined,
                endLine: row.end_line || undefined,
              })
            }
            className={`${truncClasses} bg-transparent px-0 text-aide-text-dim hover:text-aide-accent transition-colors`}
          >
            {display}
            {lineSuffix && (
              <span className="text-aide-text-dim/60">{lineSuffix}</span>
            )}
          </button>
        );
      },
    },
  ];

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Token Intelligence
      </h2>
      <p className="text-[11px] text-aide-text-dim mb-4">
        All token counts are <strong>estimates</strong> based on calibrated per-language character ratios.
      </p>

      <div className="mb-4">
        <DateRangePicker value={dateRange} onChange={setDateRange} />
      </div>

      {/* Headline stats */}
      <div className="grid grid-cols-4 gap-3 mb-6">
        <StatCard
          label="Est. Tokens Read"
          value={stats ? `~${formatTokens(stats.total_read)}` : "-"}
          sub={`${stats?.event_count ?? 0} events`}
        />
        <StatCard
          label="Est. Tokens Saved"
          value={stats ? `~${formatTokens(stats.total_saved)}` : "-"}
          sub={`~${savingsPct}% reduction`}
        />
        <StatCard
          label="Context Delivered"
          value={stats ? `~${formatTokens(stats.total_delivered)}` : "-"}
          sub="proactive injections"
        />
        <StatCard
          label="Sessions Tracked"
          value={stats ? String(stats.sessions) : "-"}
          sub={adoptionPct ? `${adoptionPct}% code tool adoption` : undefined}
        />
      </div>

      {/* Per-tool efficiency chart with concrete methodology */}
      {perToolStats.length > 0 && (
        <div className="mb-6">
          <div className="flex items-baseline justify-between mb-2">
            <h3 className="text-xs font-semibold text-aide-text">
              Per-tool efficiency
            </h3>
            <span className="text-[10px] text-aide-text-dim">
              spent = actual tokens &middot; avoided = counterfactual − spent
            </span>
          </div>
          <PerToolEfficiency stats={perToolStats} />
          <details className="mt-3 text-[11px] text-aide-text-dim">
            <summary className="cursor-pointer hover:text-aide-text-muted">
              How "avoided" is computed
            </summary>
            <div className="mt-2 space-y-1.5 pl-4 border-l border-aide-border">
              <p>
                All token counts use calibrated chars-per-token ratios per
                language (<code className="text-aide-text">pkg/code/tokens.go</code>),
                measured against Anthropic's <code className="text-aide-text">count_tokens</code> API.
              </p>
              <p>
                <strong className="text-aide-text">Consume tools</strong> (code_outline,
                code_read_symbol): <em>avoided</em> = tokens in the full file the
                agent asked about, minus what we actually sent. Grounded — the
                agent explicitly targeted this file/symbol, so a raw
                <code className="text-aide-text"> Read</code> is the concrete counterfactual.
              </p>
              <p>
                <strong className="text-aide-text">Raw Read</strong>: avoided = 0. The
                agent chose the expensive path; there's nothing cheaper to
                compare against.
              </p>
              <p>
                <strong className="text-aide-text">code_search</strong>: counts a
                call as <em>satisfied</em> when no Grep follows within 60s in the
                same session — the index answered the question and the agent
                didn't fall back to raw text search. Avoided estimate = 3× spent
                (a Grep on the same project typically returns several times the
                bytes a focused symbol search does). Calls followed by Grep
                claim no avoided.
              </p>
              <p>
                <strong className="text-aide-text">Other navigation / search</strong>
                (code_references, Grep, Glob): we report only what they cost.
                Their value is indirect — they let the agent find the right file
                before reading — and we can't claim a specific "avoided" amount
                without speculating about what the agent would have done
                otherwise.
              </p>
              <p>
                <strong className="text-aide-text">Output-sized tools</strong>
                (Bash, WebFetch, WebSearch, Grep): spent = bytes of
                tool_response that flowed back into context, divided by the
                same per-language ratio. 0 when the harness didn't pass a
                response payload (some hooks strip it for size).
              </p>
            </div>
          </details>
        </div>
      )}

      {/* Context delivery breakdown */}
      {stats && stats.total_delivered > 0 && (
        <div className="mb-6">
          <h3 className="text-xs font-semibold text-aide-text mb-2">
            Context Delivered
          </h3>
          <p className="text-[10px] text-aide-text-dim mb-2">
            Tokens aide proactively injected so the agent didn't need to search for them.
          </p>
          <div className="grid grid-cols-4 gap-3">
            {(stats.by_delivery?.memory ?? 0) > 0 && (
              <DeliveryCard
                label="Memories"
                value={stats.by_delivery.memory}
                tooltip="Tokens from project and global memories injected at session start."
              />
            )}
            {(stats.by_delivery?.decision ?? 0) > 0 && (
              <DeliveryCard
                label="Decisions"
                value={stats.by_delivery.decision}
                tooltip="Tokens from architectural decisions injected at session start."
              />
            )}
            {(stats.by_delivery?.skill ?? 0) > 0 && (
              <DeliveryCard
                label="Skills"
                value={stats.by_delivery.skill}
                tooltip="Tokens from matched skill instructions injected on user prompts."
              />
            )}
            {(stats.by_delivery?.enrichment ?? 0) > 0 && (
              <DeliveryCard
                label="Search Enrichment"
                value={stats.by_delivery.enrichment}
                tooltip="Tokens from code index context appended to Grep searches."
              />
            )}
          </div>
        </div>
      )}

      {/* Tool adoption */}
      {stats && totalFileInteractions > 0 && (
        <div className="mb-6">
          <h3 className="text-xs font-semibold text-aide-text mb-2">
            Tool Adoption
          </h3>
          <div className="flex items-center gap-3">
            <div className="flex-1 h-2 rounded-full bg-aide-bg-secondary overflow-hidden">
              <div
                className="h-full rounded-full bg-aide-accent"
                style={{ width: `${adoptionPct}%` }}
              />
            </div>
            <span className="text-xs text-aide-text-muted whitespace-nowrap">
              {stats.code_tool_count} code tools / {stats.read_count} reads ({adoptionPct}%)
            </span>
          </div>
          <p className="text-[10px] text-aide-text-dim mt-1">
            Ratio of efficient code tool calls (outline, symbol_read) vs raw file reads.
          </p>
        </div>
      )}

      {/* Events table */}
      <h3 className="text-xs font-semibold text-aide-text mb-2">Recent Events</h3>
      <FilterBar
        query={query}
        onQueryChange={setQuery}
        placeholder="Filter events..."
        dropdowns={[
          {
            value: toolFilter,
            onChange: setToolFilter,
            options: toolOptions,
            placeholder: "All tools",
          },
        ]}
      />
      {(statsLoading || eventsLoading) && (
        <p className="text-xs text-aide-text-dim py-4">Loading...</p>
      )}
      {!statsLoading && !eventsLoading && filteredEvents.length === 0 && (
        <p className="text-xs text-aide-text-dim py-4">No token events recorded yet.</p>
      )}
      {filteredEvents.length > 0 && (
        <SortableTable
          data={filteredEvents}
          columns={columns}
          keyFn={(row) => row.id}
          defaultSortKey="timestamp"
          defaultSortDir="desc"
        />
      )}

      {viewer && project && (
        <CodeViewer
          open={!!viewer}
          onClose={() => setViewer(null)}
          project={project}
          filePath={viewer.file}
          line={viewer.line}
          endLine={viewer.endLine}
        />
      )}
    </div>
  );
}
