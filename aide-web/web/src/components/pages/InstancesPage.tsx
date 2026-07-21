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
        const children =
          estate.childCount.get(row.project_root) ?? row.subprojects?.length ?? 0;
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

/** One node in the estate forest: an instance plus its nested children. */
interface EstateNode {
  inst: InstanceInfo;
  children: EstateNode[];
}

/** Separator-tolerant path key for matching roots across platforms. */
function normPath(p: string): string {
  return p.replace(/\\/g, "/").replace(/\/+$/, "");
}

/**
 * Build estate trees from parents[0] containment among known instances.
 * An instance with only surveyed (daemon-less) subprojects is still an
 * estate root — those render as leaf rows inside EstateTreeNode.
 */
function buildEstateForest(instances: InstanceInfo[]): EstateNode[] {
  const byRoot = new Map(instances.map((i) => [i.project_root, i]));
  const childrenOf = new Map<string, InstanceInfo[]>();
  for (const i of instances) {
    const parentRoot = i.parents?.[0];
    if (parentRoot && byRoot.has(parentRoot)) {
      const list = childrenOf.get(parentRoot) ?? [];
      list.push(i);
      childrenOf.set(parentRoot, list);
    }
  }

  const build = (inst: InstanceInfo): EstateNode => ({
    inst,
    children: (childrenOf.get(inst.project_root) ?? [])
      .sort((a, b) => a.project_name.localeCompare(b.project_name))
      .map(build),
  });

  return instances
    .filter(
      (i) =>
        (childrenOf.has(i.project_root) || (i.subprojects?.length ?? 0) > 0) &&
        !(i.parents?.[0] && byRoot.has(i.parents[0])),
    )
    .sort((a, b) => a.project_name.localeCompare(b.project_name))
    .map(build);
}

function EstateTreeNode({
  node,
  depth,
  knownRoots,
}: {
  node: EstateNode;
  depth: number;
  knownRoots: Set<string>;
}) {
  const { inst } = node;
  const online = inst.status === "connected";
  // Surveyed children that are NOT registered instances: daemon-less
  // nodes the survey knows about but the registry doesn't.
  const surveyed = (inst.subprojects ?? []).filter(
    (s) => s.path && !knownRoots.has(normPath(`${inst.project_root}/${s.path}`)),
  );
  return (
    <>
      <div
        className="flex items-center gap-2 py-0.5 text-sm"
        style={{ paddingLeft: `${depth * 1.25}rem` }}
      >
        <StatusBadge status={inst.status} />
        {online ? (
          <Link
            to={`/instances/${encodeURIComponent(inst.slug)}/status`}
            className="text-aide-accent hover:underline font-medium"
          >
            {inst.project_name}
          </Link>
        ) : (
          <span className="font-medium text-aide-text">{inst.project_name}</span>
        )}
        <code className="text-aide-text-dim text-xs bg-transparent px-0">
          {inst.project_root}
        </code>
      </div>
      {node.children.map((c) => (
        <EstateTreeNode
          key={c.inst.slug}
          node={c}
          depth={depth + 1}
          knownRoots={knownRoots}
        />
      ))}
      {surveyed.map((s) => (
        <div
          key={s.path}
          className="flex items-center gap-2 py-0.5 text-sm"
          style={{ paddingLeft: `${(depth + 1) * 1.25}rem` }}
          title={`Surveyed subproject (${s.evidence ?? "unknown evidence"}) — no aide daemon registered`}
        >
          <span className="inline-block w-2 h-2 rounded-full border border-aide-text-dim" />
          <span className="text-aide-text-muted">{s.name}</span>
          <code className="text-aide-text-dim text-xs bg-transparent px-0">
            {s.path}
          </code>
          {!s.has_store && (
            <span className="text-aide-text-dim text-xs">no store</span>
          )}
        </div>
      ))}
    </>
  );
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

      {!loading && !error && instances && buildEstateForest(instances).length > 0 && (
        <div className="mb-4 border border-aide-border rounded-sm p-3">
          <h3 className="text-sm font-semibold text-aide-text-muted mb-1.5">
            Estates
          </h3>
          {buildEstateForest(instances).map((n) => (
            <EstateTreeNode
              key={n.inst.slug}
              node={n}
              depth={0}
              knownRoots={
                new Set(instances.map((i) => normPath(i.project_root)))
              }
            />
          ))}
        </div>
      )}

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
