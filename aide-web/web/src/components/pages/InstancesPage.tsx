import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { SortableTable, type Column } from "../shared/SortableTable";
import { StatusBadge } from "../shared/StatusBadge";
import type { InstanceInfo } from "@/lib/types";

/** Estate relationships derived from anchor-chain parents. */
function estateLookup(instances: InstanceInfo[]) {
  const byRoot = new Map(instances.map((i) => [i.project_root, i]));
  const childCount = new Map<string, number>();
  for (const i of instances) {
    const parent = i.parents?.[0];
    if (parent) childCount.set(parent, (childCount.get(parent) ?? 0) + 1);
  }
  return { byRoot, childCount };
}

function makeColumns(
  onRemove: (project: string) => void,
  estate: ReturnType<typeof estateLookup>,
): Column<InstanceInfo>[] {
  return [
    {
      key: "project_name",
      label: "Project",
      sortable: true,
      render: (row) => (
        <span className="font-medium text-aide-text">{row.project_name}</span>
      ),
    },
    {
      key: "estate",
      label: "Estate",
      sortable: false,
      render: (row) => {
        const parentRoot = row.parents?.[0];
        if (parentRoot) {
          const parent = estate.byRoot.get(parentRoot);
          const label = parent?.project_name ?? parentRoot.split("/").pop();
          return (
            <span
              className="text-aide-text-muted"
              title={`Nested inside ${parentRoot} (own store; parent decisions cascade into its sessions)`}
            >
              {"↳ in "}
              {parent ? (
                <Link
                  to={`/instances/${encodeURIComponent(parent.slug)}/status`}
                  className="text-aide-accent hover:underline"
                >
                  {label}
                </Link>
              ) : (
                label
              )}
            </span>
          );
        }
        const children = estate.childCount.get(row.project_root);
        if (children) {
          return (
            <span
              className="text-aide-text-muted"
              title="Estate root: these subprojects have their own stores; this project's decisions cascade into their sessions"
            >
              {children} subproject{children > 1 ? "s" : ""}
            </span>
          );
        }
        return <span className="text-aide-text-dim">{"—"}</span>;
      },
    },
    {
      key: "status",
      label: "Status",
      sortable: true,
      render: (row) => (
        <span className="inline-flex items-center gap-1.5 text-aide-text-muted">
          <StatusBadge status={row.status} />
          {row.status}
        </span>
      ),
    },
    {
      key: "project_root",
      label: "Path",
      sortable: true,
      render: (row) => (
        <code className="text-aide-text-dim bg-transparent px-0">
          {row.project_root}
        </code>
      ),
    },
    {
      key: "version",
      label: "Version",
      sortable: true,
      render: (row) => <>{row.version || "\u2014"}</>,
    },
    {
      key: "actions",
      label: "Actions",
      sortable: false,
      headerClassName: "text-right",
      render: (row) => {
        const isOnline = row.status === "connected";
        return (
          <div className="flex items-center gap-2 justify-end">
            {isOnline ? (
              <Link
                to={`/instances/${encodeURIComponent(row.slug)}/status`}
                className="w-[72px] text-center border border-aide-accent text-aide-accent px-3 py-0.5 rounded-sm text-xs font-semibold hover:bg-aide-accent hover:text-aide-bg transition-all"
              >
                Open
              </Link>
            ) : (
              <span className="w-[72px] text-center text-aide-text-dim text-xs py-0.5">
                Offline
              </span>
            )}
            <button
              onClick={() => {
                if (!isOnline && confirm(`Remove ${row.project_name} from the instance list?`)) {
                  onRemove(row.slug);
                }
              }}
              disabled={isOnline}
              className={`w-[72px] text-center border px-3 py-0.5 rounded-sm text-xs font-semibold transition-all ${
                isOnline
                  ? "border-aide-border text-aide-text-dim/30 cursor-not-allowed"
                  : "border-red-500/50 text-red-400 hover:bg-red-500/10 cursor-pointer"
              }`}
              title={isOnline ? "Cannot remove a connected instance" : "Remove this offline instance from the registry"}
            >
              Remove
            </button>
          </div>
        );
      },
    },
  ];
}

export function InstancesPage() {
  const { data: instances, loading, error, refresh } = useApi(() => api.listInstances());

  const handleRemove = async (project: string) => {
    try {
      await api.deleteInstance(project);
      refresh();
    } catch (e) {
      alert(`Failed to remove: ${e instanceof Error ? e.message : e}`);
    }
  };

  const columns = makeColumns(handleRemove, estateLookup(instances ?? []));

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Instances
      </h2>

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {!loading && !error && instances && (
        <SortableTable
          data={instances}
          columns={columns}
          keyFn={(row) => row.slug}
          defaultSortKey="project_name"
          emptyMessage="No aide instances discovered. Start aide in a project to see it here. Instances register automatically when aide starts."
        />
      )}
    </div>
  );
}
