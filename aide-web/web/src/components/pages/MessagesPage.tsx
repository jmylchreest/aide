import { useState, useMemo } from "react";
import { useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { FilterBar } from "../shared/FilterBar";
import { ExpandableCard, Badge, Tag } from "../shared/ExpandableCard";
import type { MessageItem } from "@/lib/types";

const typeVariant: Record<string, "accent" | "muted" | "green" | "yellow"> = {
  request: "accent",
  response: "green",
  broadcast: "yellow",
};

export function MessagesPage() {
  const { project } = useParams<{ project: string }>();
  const {
    data: messages,
    loading,
    error,
  } = useApi(() => api.listMessages(project!), [project]);

  const [query, setQuery] = useState("");

  const filtered = useMemo(() => {
    if (!messages) return [];
    if (!query) return messages;
    const q = query.toLowerCase();
    return messages.filter((m) => {
      const searchable = `${m.from} ${m.to} ${m.content}`.toLowerCase();
      return searchable.includes(q);
    });
  }, [messages, query]);

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Messages
      </h2>

      <FilterBar
        query={query}
        onQueryChange={setQuery}
        placeholder="Filter by from, to, or content..."
      />

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {!loading && filtered.length > 0 && (
        <div className="flex flex-col gap-2">
          {filtered.map((m) => (
            <ExpandableCard
              key={m.id}
              header={
                <>
                  <span className="text-xs font-medium text-aide-accent">
                    {m.from}
                  </span>
                  <span className="text-aide-text-dim text-[0.65rem]">→</span>
                  <span className="text-xs font-medium text-aide-text-muted">
                    {m.to}
                  </span>
                </>
              }
              headerRight={
                <Badge
                  label={m.type}
                  variant={typeVariant[m.type.toLowerCase()] ?? "muted"}
                />
              }
              footer={
                <span className="text-[0.6rem] text-aide-text-dim tabular-nums">
                  ID: {m.id}
                </span>
              }
            >
              {m.content}
            </ExpandableCard>
          ))}
        </div>
      )}

      {!loading && filtered.length === 0 && !error && (
        <div className="text-center py-12 text-aide-text-dim">
          <p>No messages found.</p>
        </div>
      )}
    </div>
  );
}
