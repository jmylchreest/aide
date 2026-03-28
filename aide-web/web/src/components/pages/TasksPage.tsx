import { useState, useMemo } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { FilterBar } from "../shared/FilterBar";
import { Badge } from "../shared/ExpandableCard";
import {
  Modal,
  ConfirmDialog,
  FormField,
  inputClass,
  textareaClass,
  ModalFooterButtons,
} from "../shared/Modal";
import { Trash2, Plus } from "lucide-react";
import type { TaskItem } from "@/lib/types";

const STATUSES = ["pending", "claimed", "done", "blocked"] as const;
type TaskStatus = (typeof STATUSES)[number];

const statusVariant: Record<TaskStatus, "muted" | "yellow" | "green" | "red"> = {
  pending: "muted",
  claimed: "yellow",
  done: "green",
  blocked: "red",
};

const statusLabel: Record<TaskStatus, string> = {
  pending: "Pending",
  claimed: "Claimed",
  done: "Done",
  blocked: "Blocked",
};

function normalizeStatus(raw: string): TaskStatus {
  const lower = raw.toLowerCase().replace(/[\s_-]+/g, "");
  if (lower === "claimed" || lower === "inprogress" || lower === "in_progress") return "claimed";
  if (lower === "done" || lower === "completed" || lower === "complete") return "done";
  if (lower === "blocked" || lower === "failed") return "blocked";
  return "pending";
}

export function TasksPage() {
  const { project } = useParams<{ project: string }>();
  const {
    data: tasks,
    loading,
    error,
    refresh,
  } = useApi(() => api.listTasks(project!), [project]);

  const [searchParams] = useSearchParams();
  const [query, setQuery] = useState(() => searchParams.get("q") ?? "");
  const [statusFilter, setStatusFilter] = useState("");

  // Create modal state
  const [createOpen, setCreateOpen] = useState(false);
  const [createTitle, setCreateTitle] = useState("");
  const [createDesc, setCreateDesc] = useState("");
  const [creating, setCreating] = useState(false);

  // Delete confirm state
  const [deleteTarget, setDeleteTarget] = useState<TaskItem | null>(null);
  const [deleting, setDeleting] = useState(false);

  const ULID_RE = /^[0-9A-Z]{26}$/;
  const filtered = useMemo(() => {
    if (!tasks) return [];
    const isId = ULID_RE.test(query.toUpperCase());
    return tasks.filter((t) => {
      if (isId) return t.id.toUpperCase() === query.toUpperCase();
      const norm = normalizeStatus(t.status);
      if (statusFilter && norm !== statusFilter) return false;
      if (query) {
        const q = query.toLowerCase();
        const searchable = `${t.title} ${t.description ?? ""} ${t.claimed_by ?? ""}`.toLowerCase();
        if (!searchable.includes(q)) return false;
      }
      return true;
    });
  }, [tasks, query, statusFilter]);

  // Group by normalized status for kanban columns
  const grouped = useMemo(() => {
    const groups: Record<TaskStatus, TaskItem[]> = {
      pending: [],
      claimed: [],
      done: [],
      blocked: [],
    };
    filtered.forEach((t) => {
      const norm = normalizeStatus(t.status);
      groups[norm].push(t);
    });
    return groups;
  }, [filtered]);

  async function handleCreate() {
    if (!createTitle.trim()) return;
    setCreating(true);
    try {
      await api.createTask(project!, {
        title: createTitle.trim(),
        description: createDesc.trim() || undefined,
      });
      setCreateOpen(false);
      setCreateTitle("");
      setCreateDesc("");
      refresh();
    } catch {
      // error is surfaced via the hook on next refresh
    } finally {
      setCreating(false);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteTask(project!, deleteTarget.id);
      setDeleteTarget(null);
      refresh();
    } catch {
      // silent
    } finally {
      setDeleting(false);
    }
  }

  const statusOptions = STATUSES.map((s) => ({
    value: s,
    label: statusLabel[s],
  }));

  const hasAnyTasks = STATUSES.some((s) => grouped[s].length > 0);

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Tasks
      </h2>

      <FilterBar
        query={query}
        onQueryChange={setQuery}
        placeholder="Filter tasks..."
        dropdowns={[
          {
            value: statusFilter,
            onChange: setStatusFilter,
            options: statusOptions,
            placeholder: "All statuses",
          },
        ]}
        right={
          <button
            onClick={() => setCreateOpen(true)}
            className="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-medium text-aide-accent border border-aide-accent/30 rounded-sm hover:bg-aide-accent/10 transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
            Add Task
          </button>
        }
      />

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {!loading && hasAnyTasks && (
        <div className="grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-3">
          {STATUSES.map((status) => (
            <div
              key={status}
              className="bg-aide-surface border border-aide-border rounded p-3 flex flex-col"
            >
              <div className="flex items-center justify-between mb-2.5 pb-2 border-b border-aide-border">
                <h4 className="text-xs font-semibold uppercase tracking-wide text-aide-text-dim">
                  {statusLabel[status]}
                </h4>
                <span className="text-[0.6rem] text-aide-text-dim tabular-nums">
                  {grouped[status].length}
                </span>
              </div>
              <div className="flex flex-col gap-2 flex-1">
                {grouped[status].length === 0 ? (
                  <p className="text-[0.65rem] text-aide-text-dim italic py-3 text-center">
                    No tasks
                  </p>
                ) : (
                  grouped[status].map((t) => (
                    <TaskCard
                      key={t.id}
                      task={t}
                      onDelete={() => setDeleteTarget(t)}
                    />
                  ))
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {!loading && !hasAnyTasks && !error && (
        <div className="text-center py-12 text-aide-text-dim">
          <p>No tasks found.</p>
        </div>
      )}

      {/* Create Task Modal */}
      <Modal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Add Task"
        footer={
          <ModalFooterButtons
            onClose={() => setCreateOpen(false)}
            onSubmit={handleCreate}
            submitLabel="Create"
            loading={creating}
          />
        }
      >
        <FormField label="Title">
          <input
            className={inputClass}
            placeholder="Task title"
            value={createTitle}
            onChange={(e) => setCreateTitle(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleCreate()}
            autoFocus
          />
        </FormField>
        <FormField label="Description">
          <textarea
            className={textareaClass}
            placeholder="Optional description..."
            value={createDesc}
            onChange={(e) => setCreateDesc(e.target.value)}
          />
        </FormField>
      </Modal>

      {/* Delete Confirm Dialog */}
      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete Task"
        message={`Delete "${deleteTarget?.title ?? ""}"? This action cannot be undone.`}
        confirmLabel="Delete"
        loading={deleting}
      />
    </div>
  );
}

/* ---------- Task Card ---------- */

function TaskCard({
  task,
  onDelete,
}: {
  task: TaskItem;
  onDelete: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const norm = normalizeStatus(task.status);

  return (
    <div className="bg-aide-bg border border-aide-border rounded p-2.5 group">
      <div className="flex items-start justify-between gap-1.5">
        <div className="min-w-0 flex-1">
          <p className="text-xs font-medium text-aide-text leading-snug mb-1">
            {task.title}
          </p>
          <div className="flex items-center gap-1.5 flex-wrap">
            <Badge label={statusLabel[norm]} variant={statusVariant[norm]} />
            {task.claimed_by && (
              <span className="text-[0.6rem] text-aide-text-dim">
                by {task.claimed_by}
              </span>
            )}
          </div>
        </div>
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          className="p-1 rounded-sm text-aide-text-dim opacity-0 group-hover:opacity-100 hover:text-aide-red hover:bg-aide-red/10 transition-all"
          title="Delete task"
        >
          <Trash2 className="w-3 h-3" />
        </button>
      </div>

      {task.description && (
        <>
          <button
            onClick={() => setExpanded((e) => !e)}
            className="text-[0.6rem] text-aide-accent hover:underline mt-1.5"
          >
            {expanded ? "collapse" : "expand"}
          </button>
          {expanded && (
            <p className="text-[0.65rem] text-aide-text-muted leading-relaxed whitespace-pre-wrap break-words mt-1 pt-1 border-t border-aide-border">
              {task.description}
            </p>
          )}
        </>
      )}

      {task.result && expanded && (
        <div className="mt-1 pt-1 border-t border-aide-border">
          <p className="text-[0.6rem] font-semibold uppercase tracking-wide text-aide-text-dim mb-0.5">
            Result
          </p>
          <p className="text-[0.65rem] text-aide-text-muted leading-relaxed whitespace-pre-wrap break-words">
            {task.result}
          </p>
        </div>
      )}
    </div>
  );
}
