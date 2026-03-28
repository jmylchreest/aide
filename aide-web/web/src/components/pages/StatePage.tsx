import { useState, useMemo } from "react";
import { useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { FilterBar } from "../shared/FilterBar";
import { SortableTable, type Column } from "../shared/SortableTable";
import { ConfirmDialog } from "../shared/Modal";
import { Trash2 } from "lucide-react";
import type { StateItem } from "@/lib/types";

export function StatePage() {
  const { project } = useParams<{ project: string }>();
  const [query, setQuery] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<StateItem | null>(null);
  const [deleting, setDeleting] = useState(false);

  const { data: states, loading, error, refresh } = useApi(
    () => api.listState(project!),
    [project]
  );

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteState(project!, deleteTarget.key);
      setDeleteTarget(null);
      refresh();
    } catch {
      // error surfaced by api layer
    } finally {
      setDeleting(false);
    }
  }

  const columns: Column<StateItem>[] = useMemo(
    () => [
      {
        key: "key",
        label: "Key",
        render: (row: StateItem) => (
          <span className="font-medium text-aide-text">{row.key}</span>
        ),
      },
      {
        key: "value",
        label: "Value",
        className: "break-all whitespace-pre-wrap max-w-md",
      },
      {
        key: "agent",
        label: "Agent",
        render: (row: StateItem) => <>{row.agent || "\u2014"}</>,
        sortValue: (row: StateItem) => row.agent || "",
      },
      {
        key: "_actions",
        label: "",
        sortable: false,
        className: "w-8",
        render: (row: StateItem) => (
          <button
            onClick={() => setDeleteTarget(row)}
            className="p-1 rounded-sm text-aide-text-dim hover:text-aide-red hover:bg-aide-red/10 transition-colors"
            title="Delete state entry"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        ),
      },
    ],
    []
  );

  const filtered = useMemo(() => {
    if (!states) return [];
    const q = query.toLowerCase();
    if (!q) return states;
    return states.filter(
      (s) =>
        s.key.toLowerCase().includes(q) ||
        s.value.toLowerCase().includes(q) ||
        (s.agent && s.agent.toLowerCase().includes(q))
    );
  }, [states, query]);

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        State
      </h2>

      <FilterBar
        query={query}
        onQueryChange={setQuery}
        placeholder="Filter by key, value, or agent..."
      />

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {!loading && !error && (
        <SortableTable
          data={filtered}
          columns={columns}
          keyFn={(row) => row.key}
          emptyMessage="No state entries found."
        />
      )}

      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete State Entry"
        message={`Delete state key "${deleteTarget?.key ?? ""}"? This cannot be undone.`}
        confirmLabel="Delete"
        loading={deleting}
      />
    </div>
  );
}
