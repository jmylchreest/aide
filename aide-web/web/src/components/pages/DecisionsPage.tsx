import { useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { FilterBar } from "../shared/FilterBar";
import { ExpandableCard, Badge } from "../shared/ExpandableCard";
import { Modal, ConfirmDialog, FormField, inputClass, textareaClass, ModalFooterButtons } from "../shared/Modal";
import { Trash2, Copy, Plus } from "lucide-react";
import { useClipboard } from "@/context/ClipboardContext";
import { ClipboardBanner } from "../shared/ClipboardBanner";
import type { DecisionItem } from "@/lib/types";

export function DecisionsPage() {
  const { project } = useParams<{ project: string }>();
  const [searchParams] = useSearchParams();
  const [query, setQuery] = useState(() => searchParams.get("q") ?? "");
  const [deleteTarget, setDeleteTarget] = useState<DecisionItem | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [formTopic, setFormTopic] = useState("");
  const [formDecision, setFormDecision] = useState("");
  const [formRationale, setFormRationale] = useState("");
  const [creating, setCreating] = useState(false);
  const [pasteConfirmOpen, setPasteConfirmOpen] = useState(false);

  const { item: clipboardItem, copy, clear } = useClipboard();

  const { data: decisions, loading, error, refresh } = useApi(
    () => api.listDecisions(project!),
    [project]
  );

  const lowerQuery = query.toLowerCase();
  const filtered = decisions?.filter(
    (d) =>
      !query ||
      d.topic.toLowerCase().includes(lowerQuery) ||
      d.decision.toLowerCase().includes(lowerQuery) ||
      d.rationale?.toLowerCase().includes(lowerQuery)
  );

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteDecision(project!, deleteTarget.topic);
      setDeleteTarget(null);
      refresh();
    } catch {
      // error is surfaced by the api layer
    } finally {
      setDeleting(false);
    }
  }

  async function handleCreate() {
    if (!formTopic.trim() || !formDecision.trim()) return;
    setCreating(true);
    try {
      await api.createDecision(project!, { topic: formTopic.trim(), decision: formDecision.trim(), rationale: formRationale.trim() || undefined, decided_by: "user" });
      setCreateOpen(false);
      setFormTopic(""); setFormDecision(""); setFormRationale("");
      refresh();
    } catch {
      // error is surfaced by the api layer
    } finally {
      setCreating(false);
    }
  }

  async function handlePaste() {
    if (!clipboardItem || clipboardItem.type !== "decision") return;
    try {
      await api.createDecision(project!, clipboardItem.data as any);
      clear();
      refresh();
    } catch {
      // error surfaced by api layer
    } finally {
      setPasteConfirmOpen(false);
    }
  }

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Decisions
      </h2>

      <FilterBar
        query={query}
        onQueryChange={setQuery}
        placeholder="Filter by topic, decision, or rationale..."
        right={
          <button
            onClick={() => setCreateOpen(true)}
            className="flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium text-aide-accent border border-aide-accent/30 rounded-sm hover:bg-aide-accent/10 transition-colors shrink-0"
          >
            <Plus className="w-3.5 h-3.5" />
            Add Decision
          </button>
        }
      />

      <ClipboardBanner accepts={["decision"]} onPaste={() => setPasteConfirmOpen(true)} />

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {filtered && filtered.length > 0 ? (
        <div className="flex flex-col gap-2">
          {filtered.map((d, i) => (
            <ExpandableCard
              key={`${d.topic}-${i}`}
              header={
                <>
                  <Badge label="Decision" variant="green" />
                  <span className="text-xs font-medium text-aide-text truncate">
                    {d.topic}
                  </span>
                </>
              }
              headerRight={
                <>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      copy({ type: "decision", sourceInstance: project!, data: { topic: d.topic, decision: d.decision, rationale: d.rationale, decided_by: d.decided_by }, label: d.topic });
                    }}
                    className="p-1 rounded-sm text-aide-text-dim hover:text-aide-accent hover:bg-aide-accent/10 transition-colors"
                    title="Copy decision"
                  >
                    <Copy className="w-3.5 h-3.5" />
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setDeleteTarget(d);
                    }}
                    className="p-1 rounded-sm text-aide-text-dim hover:text-aide-red hover:bg-aide-red/10 transition-colors"
                    title="Delete decision"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </>
              }
              footer={
                d.decided_by ? (
                  <span className="text-[0.6rem] text-aide-text-dim">
                    Decided by {d.decided_by}
                  </span>
                ) : undefined
              }
            >
              <div className="space-y-2">
                <div>
                  <span className="text-[0.65rem] font-semibold uppercase tracking-wide text-aide-text-dim">
                    Decision
                  </span>
                  <p className="mt-0.5">{d.decision}</p>
                </div>
                {d.rationale && (
                  <div>
                    <span className="text-[0.65rem] font-semibold uppercase tracking-wide text-aide-text-dim">
                      Rationale
                    </span>
                    <p className="mt-0.5">{d.rationale}</p>
                  </div>
                )}
              </div>
            </ExpandableCard>
          ))}
        </div>
      ) : (
        !loading && (
          <div className="text-center py-12 text-aide-text-dim">
            <p>No decisions recorded.</p>
          </div>
        )
      )}

      {/* Create Decision Modal */}
      <Modal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Add Decision"
        footer={
          <ModalFooterButtons
            onClose={() => setCreateOpen(false)}
            onSubmit={handleCreate}
            submitLabel="Create"
            loading={creating}
          />
        }
      >
        <FormField label="Topic">
          <input
            value={formTopic}
            onChange={(e) => setFormTopic(e.target.value)}
            placeholder="e.g. Authentication strategy"
            className={inputClass}
          />
        </FormField>
        <FormField label="Decision">
          <textarea
            value={formDecision}
            onChange={(e) => setFormDecision(e.target.value)}
            placeholder="What was decided?"
            className={textareaClass}
            rows={3}
          />
        </FormField>
        <FormField label="Rationale">
          <textarea
            value={formRationale}
            onChange={(e) => setFormRationale(e.target.value)}
            placeholder="Why was this decision made? (optional)"
            className={textareaClass}
            rows={3}
          />
        </FormField>
      </Modal>

      {/* Delete Confirm Dialog */}
      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete Decision"
        message={`Are you sure you want to delete the decision "${deleteTarget?.topic ?? ""}"? This action cannot be undone.`}
        confirmLabel="Delete"
        loading={deleting}
      />

      {/* Paste Confirm Dialog */}
      <ConfirmDialog
        open={pasteConfirmOpen}
        onClose={() => setPasteConfirmOpen(false)}
        onConfirm={handlePaste}
        title="Paste Decision"
        message={`Paste decision "${clipboardItem?.label ?? ""}" from ${clipboardItem?.sourceInstance ?? ""} into ${project}?`}
        confirmLabel="Paste"
      />
    </div>
  );
}
