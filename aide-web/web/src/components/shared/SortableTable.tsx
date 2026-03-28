import { useState, useMemo } from "react";
import { cn } from "@/lib/utils";
import {
  ChevronUp,
  ChevronDown,
  ChevronsUpDown,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";

export interface Column<T> {
  key: string;
  label: string;
  sortable?: boolean;
  className?: string;
  headerClassName?: string;
  render?: (row: T) => React.ReactNode;
  sortValue?: (row: T) => string | number;
}

interface SortableTableProps<T> {
  data: T[];
  columns: Column<T>[];
  keyFn: (row: T) => string | number;
  emptyMessage?: string;
  pageSize?: number;
  defaultSortKey?: string;
  defaultSortDir?: SortDir;
}

type SortDir = "asc" | "desc";

const PAGE_SIZE_OPTIONS = [25, 50, 100, 250];

export function SortableTable<T extends Record<string, any>>({
  data,
  columns,
  keyFn,
  emptyMessage = "No data.",
  pageSize: initialPageSize = 50,
  defaultSortKey,
  defaultSortDir = "asc",
}: SortableTableProps<T>) {
  const [sortKey, setSortKey] = useState<string | null>(defaultSortKey ?? null);
  const [sortDir, setSortDir] = useState<SortDir>(defaultSortDir);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(initialPageSize);

  const handleSort = (key: string) => {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("asc");
    }
    setPage(0);
  };

  const sorted = useMemo(() => {
    if (!sortKey) return data;
    const col = columns.find((c) => c.key === sortKey);
    if (!col) return data;

    return [...data].sort((a, b) => {
      const av = col.sortValue ? col.sortValue(a) : (a[sortKey] ?? "");
      const bv = col.sortValue ? col.sortValue(b) : (b[sortKey] ?? "");
      let cmp: number;
      if (typeof av === "number" && typeof bv === "number") {
        cmp = av - bv;
      } else {
        cmp = String(av).localeCompare(String(bv), undefined, {
          sensitivity: "base",
        });
      }
      return sortDir === "desc" ? -cmp : cmp;
    });
  }, [data, sortKey, sortDir, columns]);

  const totalPages = Math.max(1, Math.ceil(sorted.length / pageSize));
  const safePage = Math.min(page, totalPages - 1);
  const paged = sorted.slice(safePage * pageSize, (safePage + 1) * pageSize);
  const showPagination = sorted.length > PAGE_SIZE_OPTIONS[0];

  if (data.length === 0) {
    return (
      <div className="text-center py-12 text-aide-text-dim">
        <p>{emptyMessage}</p>
      </div>
    );
  }

  return (
    <div className="border border-aide-border rounded overflow-hidden mb-6">
      <table className="w-full text-xs">
        <thead>
          <tr className="bg-aide-surface">
            {columns.map((col) => (
              <th
                key={col.key}
                onClick={
                  col.sortable !== false ? () => handleSort(col.key) : undefined
                }
                className={cn(
                  "text-left font-semibold uppercase tracking-wide text-aide-text-dim px-2.5 py-2 border-b-2 border-aide-border whitespace-nowrap",
                  col.sortable !== false &&
                    "cursor-pointer select-none hover:text-aide-text-muted transition-colors",
                  col.headerClassName
                )}
              >
                <span className="inline-flex items-center gap-1">
                  {col.label}
                  {col.sortable !== false && (
                    <span className="inline-flex w-3 h-3">
                      {sortKey === col.key ? (
                        sortDir === "asc" ? (
                          <ChevronUp className="w-3 h-3 text-aide-accent" />
                        ) : (
                          <ChevronDown className="w-3 h-3 text-aide-accent" />
                        )
                      ) : (
                        <ChevronsUpDown className="w-3 h-3 opacity-30" />
                      )}
                    </span>
                  )}
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {paged.map((row) => (
            <tr
              key={keyFn(row)}
              className="border-b border-aide-border last:border-b-0 hover:bg-aide-surface-hover transition-colors"
            >
              {columns.map((col) => (
                <td
                  key={col.key}
                  className={cn("px-2.5 py-1.5 text-aide-text-muted", col.className)}
                >
                  {col.render ? col.render(row) : String(row[col.key] ?? "")}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>

      {showPagination && (
        <div className="flex items-center justify-between px-2.5 py-2 bg-aide-surface border-t border-aide-border text-xs text-aide-text-dim">
          <span className="tabular-nums">
            {safePage * pageSize + 1}–{Math.min((safePage + 1) * pageSize, sorted.length)} of{" "}
            {sorted.length}
          </span>
          <div className="flex items-center gap-2">
            <select
              value={pageSize}
              onChange={(e) => {
                setPageSize(Number(e.target.value));
                setPage(0);
              }}
              className="bg-aide-bg border border-aide-border rounded px-1.5 py-0.5 text-xs text-aide-text focus:border-aide-accent outline-none"
            >
              {PAGE_SIZE_OPTIONS.map((n) => (
                <option key={n} value={n}>
                  {n} / page
                </option>
              ))}
            </select>
            <div className="flex items-center gap-0.5">
              <button
                onClick={() => setPage((p) => Math.max(0, p - 1))}
                disabled={safePage === 0}
                className="p-1 rounded-sm hover:bg-aide-surface-hover disabled:opacity-30 transition-colors"
              >
                <ChevronLeft className="w-3.5 h-3.5" />
              </button>
              <span className="px-1.5 tabular-nums">
                {safePage + 1} / {totalPages}
              </span>
              <button
                onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
                disabled={safePage >= totalPages - 1}
                className="p-1 rounded-sm hover:bg-aide-surface-hover disabled:opacity-30 transition-colors"
              >
                <ChevronRight className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
