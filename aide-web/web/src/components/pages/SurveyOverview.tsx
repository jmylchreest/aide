import { useMemo } from "react";
import type { SurveyItem } from "@/lib/types";

interface Props {
  entries: SurveyItem[];
  onModuleSelect: (topDir: string) => void;
}

interface ModuleRow {
  name: string;
  size: number;
  hub: string;
  cohesion: string;
  topDir: string;
}

// The Overview answers "what is this codebase" in one screen: the module
// map (what belongs together), per-analyzer freshness, and churn hotspots.
export function SurveyOverview({ entries, onModuleSelect }: Props) {
  const modules = useMemo<ModuleRow[]>(
    () =>
      entries
        .filter((e) => e.analyzer === "modules" && e.kind === "module")
        .map((e) => ({
          name: e.name,
          size: Number(e.metadata?.size ?? 0),
          hub: e.metadata?.hub ?? "",
          cohesion: e.metadata?.cohesion ?? "",
          topDir: (e.metadata?.top_dirs ?? "").split(",")[0] ?? "",
        }))
        .sort((a, b) => b.size - a.size),
    [entries]
  );

  const analyzers = useMemo(() => {
    const byAnalyzer = new Map<string, { count: number; commit: string }>();
    for (const e of entries) {
      const cur = byAnalyzer.get(e.analyzer) ?? { count: 0, commit: "" };
      cur.count++;
      if (!cur.commit && e.metadata?.run_commit) {
        cur.commit = e.metadata.run_commit.slice(0, 8);
      }
      byAnalyzer.set(e.analyzer, cur);
    }
    return [...byAnalyzer.entries()].sort((a, b) =>
      a[0].localeCompare(b[0])
    );
  }, [entries]);

  const churn = useMemo(
    () =>
      entries
        .filter((e) => e.kind === "churn" && e.file_path)
        .map((e) => ({
          file: e.file_path,
          commits: Number(e.metadata?.commits ?? 0),
        }))
        .sort((a, b) => b.commits - a.commits)
        .slice(0, 6),
    [entries]
  );

  const total = modules.reduce((a, m) => a + m.size, 0);

  if (entries.length === 0) {
    return (
      <p className="text-aide-text-dim text-sm">
        No survey data yet. Run <code>aide survey run</code> (or the
        survey_run MCP tool) to populate it.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap gap-2">
        {analyzers.map(([name, info]) => (
          <span
            key={name}
            className="inline-flex items-center gap-2 px-2.5 py-1 rounded border border-aide-border bg-aide-surface text-xs"
          >
            <span className="text-aide-text font-medium">{name}</span>
            <span className="text-aide-text-dim">{info.count} entries</span>
            {info.commit && (
              <span className="text-aide-accent-dim">@ {info.commit}</span>
            )}
          </span>
        ))}
      </div>

      {modules.length > 0 && (
        <div>
          <h3 className="text-sm font-medium text-aide-text mb-2">
            Module map
            <span className="text-aide-text-dim font-normal ml-2">
              {modules.length} structural modules from clustering the import
              graph — files that belong together, regardless of directory.
              Click a module to browse its entries.
            </span>
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
            {modules.map((m) => (
              <button
                key={m.name}
                onClick={() => onModuleSelect(m.topDir || m.name)}
                title={`Browse ${m.name}'s files in the list view`}
                className="text-left p-3 rounded-md bg-aide-surface border border-aide-border hover:border-aide-accent-dim transition-colors"
              >
                <div className="flex items-baseline justify-between gap-2">
                  <span className="text-sm text-aide-text font-medium truncate">
                    {m.name}
                  </span>
                  <span className="text-xs text-aide-text-dim tabular-nums shrink-0">
                    {m.size} files
                  </span>
                </div>
                <code className="block bg-transparent px-0 text-[11px] text-aide-text-dim truncate mt-0.5">
                  hub {m.hub}
                </code>
                <div
                  className="mt-2 h-1 bg-aide-surface-hover rounded-full overflow-hidden"
                  title={`${Math.round((m.size / total) * 100)}% of clustered files`}
                >
                  <div
                    className="h-full bg-aide-accent-dim rounded-full"
                    style={{ width: `${(m.size / total) * 100}%` }}
                  />
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {churn.length > 0 && (
        <div>
          <h3 className="text-sm font-medium text-aide-text mb-2">
            Change hotspots
            <span className="text-aide-text-dim font-normal ml-2">
              most-touched files in recent history
            </span>
          </h3>
          <div className="flex flex-col gap-1">
            {churn.map((c) => {
              const max = churn[0].commits || 1;
              return (
                <div key={c.file} className="flex items-center gap-3 text-xs">
                  <div className="w-40 sm:w-64 shrink-0 truncate">
                    <code className="bg-transparent px-0 text-aide-text-muted">
                      {c.file}
                    </code>
                  </div>
                  <div className="flex-1 h-3 bg-aide-surface rounded-sm overflow-hidden">
                    <div
                      className="h-full bg-aide-accent-dim/70 rounded-sm"
                      style={{ width: `${(c.commits / max) * 100}%` }}
                    />
                  </div>
                  <span className="text-aide-text-dim tabular-nums w-24 text-right shrink-0 whitespace-nowrap">
                    {c.commits} commits
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
