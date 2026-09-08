import { useState, useMemo } from "react";
import { useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { TokenOverview } from "../shared/TokenOverview";
import { TokenAccountingSummary } from "../shared/TokenAccountingSummary";
import { TokenTransformationWindows } from "../shared/TokenTransformations";
import { TokenRetrievalEvidence } from "../shared/TokenRetrievalEvidence";
import { SessionFilterInput } from "../shared/SessionFilterInput";
import { FilterBar } from "../shared/FilterBar";
import { SortableTable, type Column } from "../shared/SortableTable";
import {
  DateRangePicker,
  presetToRange,
  type DateRangeValue,
} from "../shared/DateRangePicker";
import { CodeViewer } from "../shared/CodeViewer";
import type { TokenEventItem } from "@/lib/types";
import { useProjectRoot } from "@/context/ProjectRootContext";
import { relativeToRoot } from "@/lib/paths";
import { PathLabel } from "../shared/PathLabel";

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

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

function StatCard({
  label,
  value,
  sub,
}: {
  label: string;
  value: string;
  sub?: string;
}) {
  return (
    <div className="rounded-md border border-aide-border bg-aide-surface px-4 py-3">
      <div className="text-[10px] uppercase tracking-wider text-aide-text-dim mb-1">
        {label}
      </div>
      <div className="text-xl font-semibold text-aide-text">{value}</div>
      {sub && (
        <div className="text-[10px] text-aide-text-muted mt-0.5">{sub}</div>
      )}
    </div>
  );
}

function DeliveryCard({
  label,
  value,
  tooltip,
}: {
  label: string;
  value: number;
  tooltip?: string;
}) {
  return (
    <div className="rounded-md border border-aide-border bg-aide-surface px-4 py-3">
      <div
        className="text-[10px] uppercase tracking-wider text-aide-text-dim mb-1 cursor-help"
        title={tooltip}
      >
        {label}
      </div>
      <div className="text-lg font-semibold text-blue-400">
        ~{formatTokens(value)}
      </div>
      <div className="text-[10px] text-aide-text-muted mt-0.5">
        tokens delivered
      </div>
    </div>
  );
}

// Display taxonomy; categories alone do not establish savings.
const TOOL_CATEGORIES: Record<
  string,
  "consume" | "navigate" | "search" | "modify" | "execute" | "network"
> = {
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
    <span
      className={`inline-block px-1.5 py-0.5 rounded text-[9px] font-medium border ${cls}`}
    >
      {category}
    </span>
  );
}

interface PerToolStat {
  tool: string;
  category: string;
  calls: number;
  spent: number;
}

function PerToolObservations({ stats }: { stats: PerToolStat[] }) {
  return (
    <div className="space-y-2">
      {stats.map((s) => (
        <div
          key={s.tool}
          className="rounded-md border border-aide-border bg-aide-surface px-3 py-2 flex items-center gap-2 flex-wrap"
        >
          <span className="font-mono text-xs text-aide-text">{s.tool}</span>
          <ToolCategoryBadge category={s.category} />
          <span className="text-[11px] text-aide-text-dim">
            {s.calls} observations
          </span>
          <span className="ml-auto text-[11px] text-aide-text-muted">
            {s.spent > 0
              ? `~${formatTokens(s.spent)} tokens (mixed methods)`
              : "Token quantity unknown or empty"}
          </span>
        </div>
      ))}
    </div>
  );
}

type ReportView = "overview" | "details" | "accounting";

export function TokensPage() {
  const [view, setView] = useState<ReportView>("overview");
  const { project } = useParams<{ project: string }>();
  const [session, setSession] = useState("");
  const [dateRange, setDateRange] = useState<DateRangeValue>(() => ({
    preset: "30d",
    ...presetToRange("30d"),
  }));
  // Isolate requests and evidence selection across filter changes. A late
  // response from the previous selection cannot populate this report.
  const selection = JSON.stringify([
    project,
    session,
    dateRange.since,
    dateRange.until,
  ]);
  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Token Intelligence
      </h2>
      <div className="mb-4 flex items-center gap-3 flex-wrap">
        <label className="text-[11px] text-aide-text-muted">
          Session <SessionFilterInput value={session} onChange={setSession} />
        </label>
        <DateRangePicker value={dateRange} onChange={setDateRange} />
      </div>
      <TokenReport
        key={selection}
        session={session}
        dateRange={dateRange}
        view={view}
        setView={setView}
      />
    </div>
  );
}

function TokenReport({
  session,
  dateRange,
  view,
  setView,
}: {
  session: string;
  dateRange: DateRangeValue;
  view: ReportView;
  setView: (view: ReportView) => void;
}) {
  const { project } = useParams<{ project: string }>();
  const [query, setQuery] = useState("");
  const [toolFilter, setToolFilter] = useState("");
  const [viewer, setViewer] = useState<{
    file: string;
    line?: number;
    endLine?: number;
  } | null>(null);

  const projectRoot = useProjectRoot();

  const {
    data: stats,
    loading: statsLoading,
    error: statsError,
  } = useApi(
    () =>
      api.getTokenStats(
        project!,
        session || undefined,
        dateRange.since || undefined,
        dateRange.until || undefined,
      ),
    [project, session, dateRange.since, dateRange.until],
  );

  const { data: events, loading: eventsLoading } = useApi(
    () =>
      view === "details"
        ? api.listTokenEvents(
            project!,
            session || undefined,
            200,
            dateRange.since || undefined,
            dateRange.until || undefined,
          )
        : Promise.resolve([] as TokenEventItem[]),
    [project, session, dateRange.since, dateRange.until, view === "details"],
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
    if (!stats) return [];

    // Base rows come from server-aggregated stats — every event in the
    // time window contributes, regardless of the 200-event list cap.
    // Without this, the chart only saw the most-recent 200 events and
    // tools like code_outline/code_symbols (lower-frequency than
    // Bash/Edit/Read) fell off the tail entirely.
    const tools = new Set<string>([
      ...Object.keys(stats.calls_by_tool ?? {}),
      ...Object.keys(stats.by_tool ?? {}),
      ...Object.keys(stats.saved_by_tool ?? {}),
    ]);

    const rows: PerToolStat[] = [];
    for (const tool of tools) {
      const calls = stats.calls_by_tool?.[tool] ?? 0;
      if (calls === 0) continue;
      const row: PerToolStat = {
        tool,
        category: TOOL_CATEGORIES[tool] ?? "other",
        calls,
        spent: stats.by_tool?.[tool] ?? 0,
      };
      rows.push(row);
    }

    return rows.sort((a, b) => a.tool.localeCompare(b.tool));
  }, [stats]);

  const columns: Column<TokenEventItem>[] = [
    {
      key: "timestamp",
      label: "Time",
      width: "10rem",
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
      width: "8rem",
      render: (row) => (
        <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-medium bg-aide-accent/10 text-aide-accent">
          {row.tool}
        </span>
      ),
    },
    {
      key: "event_type",
      label: "Type",
      width: "7rem",
      render: (row) => (
        <span className="text-aide-text-muted text-xs">{row.event_type}</span>
      ),
    },
    {
      key: "tokens",
      label: "Est. Tokens",
      width: "9rem",
      render: (row) => (
        <span className="font-mono text-xs">
          {row.event_type === "transformation"
            ? "See pair"
            : row.attrs?.accounting_version === "1" &&
                row.attrs?.payload_bytes === undefined
              ? "Unknown"
              : row.tokens > 0 || row.attrs?.payload_bytes === "0"
                ? `~${row.tokens}`
                : "Unknown"}
        </span>
      ),
      sortValue: (row) => row.tokens,
    },
    {
      key: "evidence",
      label: "Evidence",
      width: "17rem",
      sortable: false,
      render: (row) => (
        <details className="text-[11px] text-aide-text-muted">
          <summary className="cursor-pointer">
            {row.event_type === "transformation"
              ? "Paired output"
              : row.attrs?.accounting_version === "1"
                ? "Observed text"
                : "Legacy estimate"}
          </summary>
          <div className="mt-1 max-w-64 break-words">
            {row.attrs?.accounting_version === "1" ? (
              <>
                <div>
                  {row.attrs.observation_stage} ·{" "}
                  {row.event_type === "transformation"
                    ? `${row.attrs.before_bytes ?? "Unknown"} → ${row.attrs.after_bytes ?? "Unknown"}`
                    : (row.attrs.payload_bytes ?? "Unknown")}{" "}
                  bytes
                </div>
                <div>UTF-8 text estimate: bytes / 3 (v1)</div>
                <div>Invocation: {row.attrs.invocation_id ?? "Unknown"}</div>
                {row.attrs.raw_tool && <div>Tool: {row.attrs.raw_tool}</div>}
                <div>Window: {row.attrs.context_epoch ?? "Unknown"}</div>
                {row.attrs.recovery_path && (
                  <div>Retained original: {row.attrs.recovery_path}</div>
                )}
                <TokenRetrievalEvidence attrs={row.attrs} />
              </>
            ) : (
              <div>Measurement method and delivery coverage unknown.</div>
            )}
            <div>Event: {row.id}</div>
          </div>
        </details>
      ),
    },
    {
      key: "tokens_saved",
      label: "Legacy comparison",
      width: "12rem",
      render: (row) => (
        <span className="font-mono text-xs text-aide-text-muted">
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
      width: "12rem",
      render: (row) => {
        const value = row.file_path;
        if (!value) {
          return <span className="text-aide-text-dim text-[11px]">-</span>;
        }
        const clickable = looksLikeFilePath(value) && !!project;
        // Strip the project root prefix so common leading path is gone and
        // the filename is what reads. Tooltip keeps the full absolute value.
        const display = row.display_path || relativeToRoot(value, projectRoot);
        if (!clickable) {
          return (
            <PathLabel
              path={value}
              displayPath={row.display_path}
              className="text-[11px] text-aide-text-dim"
            />
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
            title={`Inspect current source (not an event-time snapshot): ${value + lineSuffix}`}
            onClick={() =>
              setViewer({
                // The file API rejects absolute paths (path-traversal
                // guard). Send the project-relative form — same value we
                // already display in the column.
                file: display,
                line: row.start_line || undefined,
                endLine: row.end_line || undefined,
              })
            }
            className="block w-full min-w-0 bg-transparent px-0 text-[11px] text-aide-text-dim hover:text-aide-accent transition-colors"
          >
            <PathLabel
              path={value}
              displayPath={row.display_path}
              suffix={lineSuffix}
            />
          </button>
        );
      },
    },
  ];

  return (
    <div>
      <nav
        aria-label="Token report views"
        className="flex gap-1 border-b border-aide-border mb-4"
      >
        {(["overview", "details", "accounting"] as const).map((tab) => (
          <button
            key={tab}
            type="button"
            aria-pressed={view === tab}
            onClick={() => setView(tab)}
            className={`px-3 py-2 text-xs border-b-2 capitalize ${view === tab ? "text-aide-accent border-aide-accent" : "text-aide-text-muted border-transparent hover:text-aide-text"}`}
          >
            {tab}
          </button>
        ))}
      </nav>
      {statsError ? (
        <p role="alert" className="text-xs text-red-400 mb-4">
          Unable to load accounting: {statsError}
        </p>
      ) : statsLoading ? (
        <p className="text-xs text-aide-text-muted mb-4">Loading accounting…</p>
      ) : view === "overview" && stats ? (
        <TokenOverview
          stats={stats}
          onDetails={() => setView("details")}
          onAccounting={() => setView("accounting")}
        />
      ) : null}
      <div hidden={view !== "accounting"}>
        {!statsLoading && !statsError && (
          <TokenAccountingSummary accounting={stats?.accounting} />
        )}
        <h3 className="text-xs font-semibold text-aide-text mb-2">
          Historical and compatibility estimates
        </h3>
        {/* Legacy totals remain visible, independently of measured text. */}
        <div className="grid grid-cols-2 xl:grid-cols-4 gap-3 mb-6">
          <StatCard
            label="Result token estimates"
            value={stats ? `~${formatTokens(stats.total_read)}` : "-"}
            sub={`${stats?.event_count ?? 0} events`}
          />
          <StatCard
            label="Legacy comparison estimate"
            value={stats ? `~${formatTokens(stats.total_saved)}` : "-"}
            sub="Not verified savings; may overlap"
          />
          <StatCard
            label="Context Delivered"
            value={stats ? `~${formatTokens(stats.total_delivered)}` : "-"}
            sub="proactive injections"
          />
          <StatCard
            label="Sessions Tracked"
            value={stats ? String(stats.sessions) : "-"}
          />
        </div>
      </div>
      <div hidden={view !== "details"}>
        {!statsLoading && !statsError && (
          <TokenTransformationWindows
            report={stats?.accounting?.transformations}
          />
        )}
        {perToolStats.length > 0 && (
          <div className="mb-6">
            <h3 className="text-xs font-semibold text-aide-text mb-2">
              Per-tool observations
            </h3>
            <p className="text-[11px] text-aide-text-dim mb-2">
              All recorded observations in the selected period. Estimates mix
              historical methods and observed text; these are not provider
              totals.
            </p>
            <PerToolObservations stats={perToolStats} />
          </div>
        )}

        {/* Context delivery breakdown */}
        {stats && stats.total_delivered > 0 && (
          <div className="mb-6">
            <h3 className="text-xs font-semibold text-aide-text mb-2">
              Context Delivered
            </h3>
            <p className="text-[10px] text-aide-text-dim mb-2">
              Historical estimates of guidance injected by aide; delivery does
              not establish avoided searches.
            </p>
            <div className="grid grid-cols-2 xl:grid-cols-4 gap-3">
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

        {/* Events table */}
        <h3 className="text-xs font-semibold text-aide-text mb-2">
          Recent Events
        </h3>
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
          <p className="text-xs text-aide-text-dim py-4">
            No token events recorded yet.
          </p>
        )}
        {filteredEvents.length > 0 && (
          <SortableTable
            data={filteredEvents}
            columns={columns}
            minWidth="75rem"
            keyFn={(row) => row.id}
            defaultSortKey="timestamp"
            defaultSortDir="desc"
          />
        )}
      </div>
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
