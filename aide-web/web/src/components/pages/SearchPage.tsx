import { useState } from "react";
import { Search } from "lucide-react";
import { api } from "@/lib/api";
import { SortableTable, type Column } from "../shared/SortableTable";
import { Badge } from "../shared/ExpandableCard";
import type { SearchResult } from "@/lib/types";

const columns: Column<SearchResult & { _idx: number }>[] = [
  {
    key: "instance",
    label: "Instance",
    sortable: true,
    render: (row) => (
      <span className="text-aide-accent font-medium">{row.instance}</span>
    ),
  },
  {
    key: "type",
    label: "Type",
    sortable: true,
    render: (row) => <Badge label={row.type} variant="muted" />,
  },
  {
    key: "title",
    label: "Title",
    sortable: true,
    render: (row) =>
      row.link ? (
        <a
          href={row.link}
          className="text-aide-accent hover:text-aide-accent-dark font-medium"
        >
          {row.title}
        </a>
      ) : (
        <span className="text-aide-text font-medium">{row.title}</span>
      ),
  },
  {
    key: "detail",
    label: "Detail",
    sortable: true,
    className: "break-all max-w-md",
    render: (row) => <>{row.detail}</>,
  },
];

export function SearchPage() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);

  const handleSearch = async () => {
    if (!query.trim()) return;
    setLoading(true);
    setSearched(true);
    try {
      const r = await api.search(query);
      setResults(r);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  const indexedResults = results.map((r, i) => ({ ...r, _idx: i }));

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Cross-Instance Search
      </h2>

      <div className="flex items-center gap-2 mb-3">
        <div className="relative flex-1">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-aide-text-dim pointer-events-none" />
          <input
            type="search"
            placeholder="Search across all instances..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleSearch()}
            className="w-full bg-aide-surface border border-aide-border rounded pl-7 pr-2 py-1.5 text-xs text-aide-text placeholder:text-aide-text-dim focus:border-aide-accent focus:ring-2 focus:ring-aide-accent/20 outline-none transition"
          />
        </div>
        <button
          onClick={handleSearch}
          disabled={loading}
          className="border border-aide-accent text-aide-accent px-3 py-1.5 rounded-sm text-xs font-semibold hover:bg-aide-accent hover:text-aide-bg transition-all disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Search
        </button>
      </div>

      {loading && <p className="text-aide-text-dim text-sm">Searching...</p>}

      {!loading && searched && (
        <SortableTable
          data={indexedResults}
          columns={columns}
          keyFn={(row) => row._idx}
          emptyMessage="No results found."
        />
      )}

      {!loading && !searched && (
        <div className="text-center py-12 text-aide-text-dim">
          <p>
            Enter a query above to search across memories, decisions, tasks, and
            more.
          </p>
        </div>
      )}
    </div>
  );
}
