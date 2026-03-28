import { useState, useMemo, useEffect } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { FilterBar } from "../shared/FilterBar";
import { ExpandableCard, Badge, Tag } from "../shared/ExpandableCard";
import { TagInput } from "../shared/TagInput";
import {
  Modal,
  ConfirmDialog,
  FormField,
  textareaClass,
  ModalFooterButtons,
} from "../shared/Modal";
import { Trash2, Plus, Pencil, Copy } from "lucide-react";
import { useClipboard } from "@/context/ClipboardContext";
import { ClipboardBanner } from "../shared/ClipboardBanner";
import type { MemoryItem } from "@/lib/types";

// Official categories — matches aide/pkg/memory/types.go constants
const CATEGORIES = [
  { value: "learning", label: "Learning", desc: "Technical discoveries and lessons learned" },
  { value: "decision", label: "Decision", desc: "Choices made with rationale" },
  { value: "issue", label: "Issue", desc: "Known problems and workarounds" },
  { value: "blocker", label: "Blocker", desc: "Things that stopped progress" },
  { value: "abandoned", label: "Abandoned", desc: "Failed/rejected approaches" },
  { value: "discovery", label: "Discovery", desc: "Shared findings across agents" },
] as const;

const FILTER_OPTIONS = CATEGORIES.map((c) => ({ value: c.value, label: c.label }));

// Tags automatically added to user-created memories
const AUTO_TAGS = ["source:user", "verified:true"];

// Well-known tags users can pick from
const TAG_SUGGESTIONS = [
  "scope:global",
  "scope:project",
];

// ULID: 26 uppercase alphanumeric characters
const ULID_RE = /^[0-9A-Z]{26}$/;

export function MemoriesPage() {
  const { project } = useParams<{ project: string }>();
  const [searchParams] = useSearchParams();
  const [query, setQuery] = useState(() => searchParams.get("q") ?? "");
  const [categoryFilter, setCategoryFilter] = useState("");
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<MemoryItem | null>(null);
  const [editTarget, setEditTarget] = useState<MemoryItem | null>(null);
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [saving, setSaving] = useState(false);

  const { item: clipboardItem, copy, clear } = useClipboard();

  // Form state
  const [formCategory, setFormCategory] = useState("learning");
  const [formContent, setFormContent] = useState("");
  const [formTags, setFormTags] = useState<string[]>([]);

  const isIdQuery = ULID_RE.test(query.toUpperCase());

  // Fetch by ID when the query is a ULID (deep link from search), otherwise list all
  const { data: memories, loading, error, refresh } = useApi(
    () =>
      isIdQuery
        ? api.getMemory(project!, query).then((m) => (m ? [m] : []))
        : api.listMemories(project!, categoryFilter || undefined),
    [project, categoryFilter, isIdQuery ? query : ""]
  );

  const lowerQuery = query.toLowerCase();
  const filtered = isIdQuery
    ? memories
    : memories?.filter(
        (m) =>
          !query ||
          m.content.toLowerCase().includes(lowerQuery) ||
          m.category.toLowerCase().includes(lowerQuery) ||
          m.tags?.some((t) => t.toLowerCase().includes(lowerQuery))
      );

  // Derive project-specific tag suggestions
  const tagSuggestions = useMemo(() => {
    const projectTag = project ? `project:${project}` : null;
    const base = [...TAG_SUGGESTIONS];
    if (projectTag) base.unshift(projectTag);
    return base;
  }, [project]);

  function openCreateModal() {
    setFormCategory("learning");
    setFormContent("");
    setFormTags([]);
    setShowCreateModal(true);
  }

  function buildTags(userTags: string[]): string[] {
    // Merge auto-tags + user tags, deduplicate
    return [...new Set([...AUTO_TAGS, ...userTags])];
  }

  async function handleCreate() {
    if (!formContent.trim()) return;
    setCreating(true);
    try {
      await api.createMemory(project!, {
        category: formCategory,
        content: formContent.trim(),
        tags: buildTags(formTags),
      });
      setShowCreateModal(false);
      refresh();
    } catch {
      // error is surfaced by the api layer
    } finally {
      setCreating(false);
    }
  }

  function openEditModal(m: MemoryItem) {
    setFormCategory(m.category);
    setFormContent(m.content);
    // Strip auto-tags from editable list so they don't duplicate
    setFormTags((m.tags ?? []).filter((t) => !AUTO_TAGS.includes(t)));
    setEditTarget(m);
  }

  async function handleEdit() {
    if (!editTarget || !formContent.trim()) return;
    setSaving(true);
    try {
      await api.deleteMemory(project!, editTarget.id);
      await api.createMemory(project!, {
        category: formCategory,
        content: formContent.trim(),
        tags: buildTags(formTags),
      });
      setEditTarget(null);
      refresh();
    } catch {
      // error surfaced by api layer
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteMemory(project!, deleteTarget.id);
      setDeleteTarget(null);
      refresh();
    } catch {
      // error is surfaced by the api layer
    } finally {
      setDeleting(false);
    }
  }

  async function handlePaste() {
    if (!clipboardItem || clipboardItem.type !== "memory") return;
    const { category, content, tags } = clipboardItem.data as {
      category: string;
      content: string;
      tags: string[];
    };
    try {
      await api.createMemory(project!, { category, content, tags });
      clear();
      refresh();
    } catch {
      // error surfaced by api layer
    }
  }

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Memories
      </h2>

      <FilterBar
        query={query}
        onQueryChange={setQuery}
        placeholder="Filter memories..."
        dropdowns={[
          {
            value: categoryFilter,
            onChange: setCategoryFilter,
            options: FILTER_OPTIONS,
            placeholder: "All categories",
          },
        ]}
        right={
          <button
            onClick={openCreateModal}
            className="flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium text-aide-accent border border-aide-accent/30 rounded-sm hover:bg-aide-accent/10 transition-colors shrink-0"
          >
            <Plus className="w-3.5 h-3.5" />
            Add Memory
          </button>
        }
      />

      <ClipboardBanner accepts={["memory"]} onPaste={handlePaste} />

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {filtered && filtered.length > 0 ? (
        <div className="flex flex-col gap-2">
          {filtered.map((m) => (
            <ExpandableCard
              key={m.id}
              header={
                <>
                  <Badge label={m.category} />
                </>
              }
              headerRight={
                <>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      copy({
                        type: "memory",
                        sourceInstance: project!,
                        data: { category: m.category, content: m.content, tags: m.tags },
                        label: m.content.split("\n")[0].slice(0, 60),
                      });
                    }}
                    className="p-1 rounded-sm text-aide-text-dim hover:text-aide-accent hover:bg-aide-accent/10 transition-colors"
                    title="Copy memory"
                  >
                    <Copy className="w-3.5 h-3.5" />
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      openEditModal(m);
                    }}
                    className="p-1 rounded-sm text-aide-text-dim hover:text-aide-accent hover:bg-aide-accent/10 transition-colors"
                    title="Edit memory"
                  >
                    <Pencil className="w-3.5 h-3.5" />
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setDeleteTarget(m);
                    }}
                    className="p-1 rounded-sm text-aide-text-dim hover:text-aide-red hover:bg-aide-red/10 transition-colors"
                    title="Delete memory"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </>
              }
              footer={
                <div className="flex items-center justify-between gap-2">
                  <div className="flex flex-wrap gap-1">
                    {m.tags?.map((tag) => (
                      <Tag key={tag} label={tag} />
                    ))}
                  </div>
                  <span className="text-[0.6rem] text-aide-text-dim font-mono shrink-0">
                    {m.id}
                  </span>
                </div>
              }
            >
              {m.content}
            </ExpandableCard>
          ))}
        </div>
      ) : (
        !loading && (
          <div className="text-center py-12 text-aide-text-dim">
            <p>No memories found.</p>
          </div>
        )
      )}

      {/* Create Modal */}
      <Modal
        open={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        title="Add Memory"
        footer={
          <ModalFooterButtons
            onClose={() => setShowCreateModal(false)}
            onSubmit={handleCreate}
            submitLabel="Create"
            loading={creating}
          />
        }
      >
        <MemoryForm
          category={formCategory}
          onCategoryChange={setFormCategory}
          content={formContent}
          onContentChange={setFormContent}
          tags={formTags}
          onTagsChange={setFormTags}
          tagSuggestions={tagSuggestions}
        />
      </Modal>

      {/* Edit Modal */}
      <Modal
        open={!!editTarget}
        onClose={() => setEditTarget(null)}
        title="Edit Memory"
        footer={
          <ModalFooterButtons
            onClose={() => setEditTarget(null)}
            onSubmit={handleEdit}
            submitLabel="Save"
            loading={saving}
          />
        }
      >
        <MemoryForm
          category={formCategory}
          onCategoryChange={setFormCategory}
          content={formContent}
          onContentChange={setFormContent}
          tags={formTags}
          onTagsChange={setFormTags}
          tagSuggestions={tagSuggestions}
        />
      </Modal>

      {/* Delete Confirm Dialog */}
      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete Memory"
        message={`Are you sure you want to delete this memory? This action cannot be undone.`}
        confirmLabel="Delete"
        loading={deleting}
      />
    </div>
  );
}

/* ---- Memory Form (shared between create and edit modals) ---- */

interface MemoryFormProps {
  category: string;
  onCategoryChange: (v: string) => void;
  content: string;
  onContentChange: (v: string) => void;
  tags: string[];
  onTagsChange: (v: string[]) => void;
  tagSuggestions: string[];
}

function MemoryForm({
  category,
  onCategoryChange,
  content,
  onContentChange,
  tags,
  onTagsChange,
  tagSuggestions,
}: MemoryFormProps) {
  return (
    <>
      <FormField label="Category">
        <div className="grid grid-cols-2 gap-1.5">
          {CATEGORIES.map((c) => (
            <button
              key={c.value}
              type="button"
              onClick={() => onCategoryChange(c.value)}
              className={`text-left px-2.5 py-2 rounded border text-xs transition-all ${
                category === c.value
                  ? "border-aide-accent bg-aide-accent/10 text-aide-accent"
                  : "border-aide-border bg-aide-bg text-aide-text-muted hover:border-aide-border-light"
              }`}
            >
              <div className="font-semibold">{c.label}</div>
              <div className="text-[0.6rem] mt-0.5 opacity-70">{c.desc}</div>
            </button>
          ))}
        </div>
      </FormField>
      <FormField label="Content">
        <textarea
          value={content}
          onChange={(e) => onContentChange(e.target.value)}
          placeholder="What should be remembered?"
          className={textareaClass}
          rows={5}
        />
      </FormField>
      <FormField label="Tags">
        <TagInput
          tags={tags}
          onChange={onTagsChange}
          suggestions={tagSuggestions}
          autoTags={AUTO_TAGS}
          placeholder="Type a tag and press Enter..."
        />
      </FormField>
    </>
  );
}
