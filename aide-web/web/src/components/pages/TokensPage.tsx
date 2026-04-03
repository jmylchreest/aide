import { useState, useMemo } from "react";
import { useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { FilterBar } from "../shared/FilterBar";
import { SortableTable, type Column } from "../shared/SortableTable";
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

function SavingsBar({ label, value, max }: { label: string; value: number; max: number }) {
  const pct = max > 0 ? Math.min((value / max) * 100, 100) : 0;
  return (
    <div className="mb-2">
      <div className="flex justify-between text-xs mb-0.5">
        <span className="text-aide-text-muted">{label}</span>
        <span className="text-aide-text font-medium">~{formatTokens(value)}</span>
      </div>
      <div className="w-full bg-aide-bg-tertiary rounded-full h-2">
        <div
          className="bg-aide-accent h-2 rounded-full transition-all"
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}

export function TokensPage() {
  const { project } = useParams<{ project: string }>();
  const [query, setQuery] = useState("");
  const [toolFilter, setToolFilter] = useState("");

  const { data: stats, loading: statsLoading } = useApi(
    () => api.getTokenStats(project!),
    [project],
  );

  const { data: events, loading: eventsLoading } = useApi(
    () => api.listTokenEvents(project!, undefined, 200),
    [project],
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

  const savingsMax = stats
    ? Math.max(
        stats.by_saving_type?.outline ?? 0,
        stats.by_saving_type?.read_avoided ?? 0,
        1,
      )
    : 1;

  const savingsPct =
    stats && stats.total_read + stats.total_saved > 0
      ? ((stats.total_saved / (stats.total_read + stats.total_saved)) * 100).toFixed(1)
      : "0";

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
          sub={`~${savingsPct}% savings`}
        />
        <StatCard
          label="Est. Tokens Written"
          value={stats ? `~${formatTokens(stats.total_written)}` : "-"}
        />
        <StatCard
          label="Sessions Tracked"
          value={stats ? String(stats.sessions) : "-"}
        />
      </div>

      {/* Savings breakdown */}
      {stats && stats.total_saved > 0 && (
        <div className="mb-6 rounded-md border border-aide-border bg-aide-bg-secondary p-4">
          <h3 className="text-xs font-semibold text-aide-text mb-3">
            Estimated Savings Breakdown
          </h3>
          <SavingsBar
            label="Outline substitutions"
            value={stats.by_saving_type?.outline ?? 0}
            max={savingsMax}
          />
          <SavingsBar
            label="Avoided re-reads"
            value={stats.by_saving_type?.read_avoided ?? 0}
            max={savingsMax}
          />
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
