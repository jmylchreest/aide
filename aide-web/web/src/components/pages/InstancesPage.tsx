import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { SortableTable, type Column } from "../shared/SortableTable";
import { StatusBadge } from "../shared/StatusBadge";
import type { InstanceInfo } from "@/lib/types";

const columns: Column<InstanceInfo>[] = [
  {
    key: "project_name",
    label: "Project",
    sortable: true,
    render: (row) => (
      <span className="font-medium text-aide-text">{row.project_name}</span>
    ),
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
    render: (row) =>
      row.status === "connected" ? (
        <Link
          to={`/instances/${encodeURIComponent(row.project_name)}/status`}
          className="inline-block border border-aide-accent text-aide-accent px-3 py-0.5 rounded-sm text-xs font-semibold hover:bg-aide-accent hover:text-aide-bg transition-all"
        >
          Open
        </Link>
      ) : (
        <span className="text-aide-text-dim">Offline</span>
      ),
  },
];

export function InstancesPage() {
  const { data: instances, loading, error } = useApi(() => api.listInstances());

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
          keyFn={(row) => row.project_name}
          defaultSortKey="project_name"
          emptyMessage="No aide instances discovered. Start aide in a project to see it here. Instances register automatically when aide starts."
        />
      )}
    </div>
  );
}
