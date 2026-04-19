import { useState, useMemo } from "react";
import { useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { FilterBar } from "../shared/FilterBar";
import { SortableTable, type Column } from "../shared/SortableTable";
import { DateRangePicker, presetToRange, type DateRangeValue } from "../shared/DateRangePicker";
import type { TokenEventItem } from "@/lib/types";

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

function SavingCard({ label, value, tooltip }: { label: string; value: number; tooltip?: string }) {
  return (
    <div className="rounded-md border border-aide-border bg-aide-bg-secondary px-4 py-3">
      <div className="text-[10px] uppercase tracking-wider text-aide-text-dim mb-1 cursor-help" title={tooltip}>{label}</div>
      <div className="text-lg font-semibold text-green-500">~{formatTokens(value)}</div>
      <div className="text-[10px] text-aide-text-muted mt-0.5">est. tokens saved</div>
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

export function TokensPage() {
  const { project } = useParams<{ project: string }>();
  const [query, setQuery] = useState("");
  const [toolFilter, setToolFilter] = useState("");

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
      key: "file_path",
      label: "File",
      render: (row) => (
        <span className="text-aide-text-dim font-mono text-[11px] truncate max-w-[300px] block">
          {row.file_path}
        </span>
      ),
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

      {/* Savings breakdown */}
      {stats && stats.total_saved > 0 && (
        <div className="mb-6">
          <h3 className="text-xs font-semibold text-aide-text mb-2">
            Savings Breakdown
          </h3>
          <div className="grid grid-cols-3 gap-3">
            {(stats.by_saving_type?.outline ?? 0) > 0 && (
              <SavingCard
                label="Outline Substitutions"
                value={stats.by_saving_type.outline}
                tooltip="Tokens saved when code_outline was used instead of reading the full file. The outline shows file structure at ~5-15% of full token cost."
              />
            )}
            {(stats.by_saving_type?.symbol_read ?? 0) > 0 && (
              <SavingCard
                label="Symbol Reads"
                value={stats.by_saving_type.symbol_read}
                tooltip="Tokens saved when code_read_symbol returned just a function/class body instead of the full file."
              />
            )}
            {(stats.by_saving_type?.read_avoided ?? 0) > 0 && (
              <SavingCard
                label="Avoided Re-reads"
                value={stats.by_saving_type.read_avoided}
                tooltip="Tokens saved when aide detected a file was already read and unchanged, avoiding a redundant full re-read."
              />
            )}
          </div>
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
    </div>
  );
}
