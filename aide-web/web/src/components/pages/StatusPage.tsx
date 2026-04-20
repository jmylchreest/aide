import { useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { StatusBadge } from "../shared/StatusBadge";
import { Badge } from "../shared/ExpandableCard";
import type { InstanceInfo } from "@/lib/types";

function Dt({ children }: { children: React.ReactNode }) {
  return (
    <dt className="font-semibold text-xs uppercase tracking-wide text-aide-text-dim pt-0.5">
      {children}
    </dt>
  );
}

function Dd({ children, border = true }: { children: React.ReactNode; border?: boolean }) {
  return (
    <dd className={`text-aide-text-muted text-xs pb-2 break-all ${border ? "border-b border-aide-border" : ""}`}>
      {children}
    </dd>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-aide-surface border border-aide-border rounded p-4">
      <h3 className="text-xs font-semibold uppercase tracking-wide text-aide-text-dim mb-3 pb-1.5 border-b border-aide-border">
        {title}
      </h3>
      {children}
    </div>
  );
}

function Stat({ label, value, accent }: { label: string; value: string | number; accent?: boolean }) {
  return (
    <div className="text-center">
      <div className={`text-lg font-semibold tabular-nums ${accent ? "text-aide-accent" : "text-aide-text"}`}>
        {value}
      </div>
      <div className="text-[0.6rem] uppercase tracking-wide text-aide-text-dim mt-0.5">{label}</div>
    </div>
  );
}

export function StatusPage() {
  const { project } = useParams<{ project: string }>();
  const { data: instances } = useApi(() => api.listInstances());
  const { data: detailed, loading, error } = useApi(
    () => api.getDetailedStatus(project!),
    [project]
  );

  const instance = instances?.find(
    (i: InstanceInfo) => i.project_name === project
  );

  if (loading) {
    return <p className="text-aide-text-dim text-sm">Loading status...</p>;
  }

  if (error) {
    return <p className="text-aide-red text-sm">{error}</p>;
  }

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Status
      </h2>

      <div className="grid gap-3">
        {/* Server Info */}
        <Section title="Server">
          <dl className="grid grid-cols-[120px_1fr] gap-x-4 gap-y-0.5">
            <Dt>Status</Dt>
            <Dd>
              <span className="inline-flex items-center gap-1.5">
                <StatusBadge status={instance?.status ?? "disconnected"} />
                {instance?.status ?? "unknown"}
                {detailed?.uptime && (
                  <span className="text-aide-text-dim ml-1">(uptime: {detailed.uptime})</span>
                )}
              </span>
            </Dd>
            <Dt>Version</Dt>
            <Dd>{detailed?.version || instance?.version || "unknown"}</Dd>
            <Dt>Path</Dt>
            <Dd>
              <code className="bg-transparent px-0">{instance?.project_root}</code>
            </Dd>
            <Dt>Socket</Dt>
            <Dd border={false}>
              <code className="bg-transparent px-0">{instance?.socket_path}</code>
            </Dd>
          </dl>
        </Section>

        {/* File Watcher */}
        {detailed?.watcher && (
          <Section title="File Watcher">
            <dl className="grid grid-cols-[120px_1fr] gap-x-4 gap-y-0.5">
              <Dt>Status</Dt>
              <Dd>
                <Badge label={detailed.watcher.enabled ? "enabled" : "disabled"} variant={detailed.watcher.enabled ? "green" : "muted"} />
              </Dd>
              <Dt>Directories</Dt>
              <Dd>{detailed.watcher.dirs_watched}</Dd>
              <Dt>Debounce</Dt>
              <Dd>{detailed.watcher.debounce}</Dd>
              <Dt>Pending</Dt>
              <Dd>{detailed.watcher.pending}</Dd>
              <Dt>Subscribers</Dt>
              <Dd border={false}>
                <span className="flex gap-1 flex-wrap">
                  {detailed.watcher.subscribers?.map((s) => (
                    <Badge key={s} label={s} variant="accent" />
                  ))}
                </span>
              </Dd>
            </dl>
          </Section>
        )}

        {/* Code Indexer + Findings + Survey stats row */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
          {/* Code Indexer */}
          {detailed?.code_indexer && (
            <Section title="Code Index">
              <div className="flex justify-around py-2">
                <Stat label="Files" value={detailed.code_indexer.files} />
                <Stat label="Symbols" value={detailed.code_indexer.symbols} accent />
                <Stat label="References" value={detailed.code_indexer.references} />
              </div>
              <div className="text-center mt-2">
                <Badge
                  label={detailed.code_indexer.status || "idle"}
                  variant={detailed.code_indexer.status === "indexing" ? "yellow" : "green"}
                />
              </div>
            </Section>
          )}

          {/* Findings Summary */}
          {detailed?.findings && (
            <Section title="Findings">
              <div className="flex justify-around py-2">
                <Stat label="Total" value={detailed.findings.total} accent />
                {Object.entries(detailed.findings.by_severity || {})
                  .sort(([a], [b]) => {
                    const order: Record<string, number> = { critical: 0, warning: 1, info: 2 };
                    return (order[a] ?? 9) - (order[b] ?? 9);
                  })
                  .map(([severity, count]) => (
                    <Stat key={severity} label={severity} value={count} />
                  ))}
              </div>
            </Section>
          )}

          {/* Survey Summary */}
          {detailed?.survey && (
            <Section title="Survey">
              <div className="flex justify-around py-2">
                <Stat label="Total" value={detailed.survey.total} accent />
                {Object.entries(detailed.survey.by_kind || {})
                  .sort(([, a], [, b]) => b - a)
                  .slice(0, 3)
                  .map(([kind, count]) => (
                    <Stat key={kind} label={kind} value={count} />
                  ))}
              </div>
            </Section>
          )}
        </div>

        {/* Findings Analyzers detail table */}
        {detailed?.findings?.analyzers && Object.keys(detailed.findings.analyzers).length > 0 && (
          <Section title="Analyzers">
            <div className="border border-aide-border rounded overflow-hidden">
              <table className="w-full text-xs">
                <thead>
                  <tr className="bg-aide-bg">
                    {["Analyzer", "Status", "Scope", "Findings", "Last Run", "Duration"].map((h) => (
                      <th key={h} className="text-left font-semibold uppercase tracking-wide text-aide-text-dim px-2.5 py-1.5 border-b-2 border-aide-border">
                        {h}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {Object.entries(detailed.findings.analyzers)
                    .sort(([a], [b]) => a.localeCompare(b))
                    .map(([name, a]) => (
                      <tr key={name} className="border-b border-aide-border last:border-b-0 hover:bg-aide-surface-hover transition-colors">
                        <td className="px-2.5 py-1.5 font-medium text-aide-text">{name}</td>
                        <td className="px-2.5 py-1.5">
                          <Badge label={a.status || "idle"} variant={a.status === "running" ? "yellow" : "green"} />
                        </td>
                        <td className="px-2.5 py-1.5 text-aide-text-dim">
                          <code className="bg-transparent px-0">{a.scope}</code>
                        </td>
                        <td className="px-2.5 py-1.5 text-aide-text-muted tabular-nums">{a.findings}</td>
                        <td className="px-2.5 py-1.5 text-aide-text-dim text-[0.65rem]">{a.last_run || "—"}</td>
                        <td className="px-2.5 py-1.5 text-aide-text-dim">{a.last_duration || "—"}</td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>
          </Section>
        )}

        {/* Stores */}
        {detailed?.stores && detailed.stores.length > 0 && (
          <Section title="Stores">
            <div className="border border-aide-border rounded overflow-hidden">
              <table className="w-full text-xs">
                <thead>
                  <tr className="bg-aide-bg">
                    {["Store", "Path", "Size"].map((h) => (
                      <th key={h} className="text-left font-semibold uppercase tracking-wide text-aide-text-dim px-2.5 py-1.5 border-b-2 border-aide-border">
                        {h}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {detailed.stores.map((s) => (
                    <tr key={s.name} className="border-b border-aide-border last:border-b-0 hover:bg-aide-surface-hover transition-colors">
                      <td className="px-2.5 py-1.5 font-medium text-aide-text">{s.name}</td>
                      <td className="px-2.5 py-1.5 text-aide-text-dim">
                        <code className="bg-transparent px-0">.aide/{s.path}</code>
                      </td>
                      <td className="px-2.5 py-1.5 text-aide-text-muted tabular-nums">{formatSize(s.size)}</td>
                    </tr>
                  ))}
                  <tr className="bg-aide-bg">
                    <td className="px-2.5 py-1.5 font-semibold text-aide-text-dim" colSpan={2}>Total</td>
                    <td className="px-2.5 py-1.5 font-semibold text-aide-text tabular-nums">
                      {formatSize(detailed.stores.reduce((sum, s) => sum + s.size, 0))}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </Section>
        )}

        {/* Grammars */}
        {detailed?.grammars && detailed.grammars.length > 0 && (
          <Section title="Grammars">
            <div className="flex flex-wrap gap-1.5">
              {[...detailed.grammars]
                .sort((a, b) => a.name.localeCompare(b.name))
                .map((g) => (
                <span
                  key={g.name}
                  className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-aide-surface border border-aide-border text-aide-text"
                >
                  {g.name}
                  {g.built_in && (
                    <span className="text-[10px] text-aide-text-dim">(builtin)</span>
                  )}
                </span>
              ))}
            </div>
            <p className="text-xs text-aide-text-dim mt-1.5">
              {detailed.grammars.length} grammar{detailed.grammars.length !== 1 ? "s" : ""} loaded
            </p>
          </Section>
        )}
      </div>
    </div>
  );
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
