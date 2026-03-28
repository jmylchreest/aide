import { useState, useMemo } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { cn } from "@/lib/utils";
import { FilterBar } from "../shared/FilterBar";
import { SortableTable, type Column } from "../shared/SortableTable";
import { Badge } from "../shared/ExpandableCard";
import { CodeViewer } from "../shared/CodeViewer";
import { Eye, EyeOff, Check } from "lucide-react";
import type { FindingItem } from "@/lib/types";

const severityStyles: Record<string, string> = {
  critical: "text-aide-red font-semibold",
  high: "text-orange-400 font-medium",
  warning: "text-aide-yellow",
  info: "text-aide-text-dim",
};

const severityOrder: Record<string, number> = {
  critical: 0,
  high: 1,
  warning: 2,
  info: 3,
};

const severityOptions = [
  { value: "critical", label: "Critical" },
  { value: "high", label: "High" },
  { value: "warning", label: "Warning" },
  { value: "info", label: "Info" },
];

export function FindingsPage() {
  const { project } = useParams<{ project: string }>();
  const [searchParams] = useSearchParams();
  const [query, setQuery] = useState(() => searchParams.get("q") ?? "");
  const [severityFilter, setSeverityFilter] = useState("");
  const [analyzerFilter, setAnalyzerFilter] = useState("");
  const [showAccepted, setShowAccepted] = useState(false);
  const [viewerFile, setViewerFile] = useState<string | null>(null);
  const [viewerLine, setViewerLine] = useState<number | undefined>();
  const [viewerTitle, setViewerTitle] = useState<string | undefined>();

  const { data: findings, loading, error, refresh } = useApi(
    () => api.listFindings(project!, undefined, undefined),
    [project]
  );

  async function handleAccept(id: string) {
    try {
      await api.acceptFindings(project!, [id]);
      refresh();
    } catch {
      // error surfaced by api layer
    }
  }

  const columns: Column<FindingItem>[] = [
    {
      key: "severity",
      label: "Severity",
      sortValue: (row: FindingItem) => severityOrder[row.severity] ?? 99,
      render: (row: FindingItem) => (
        <span className={cn("text-xs", severityStyles[row.severity] ?? "text-aide-text-muted")}>
          {row.severity}
        </span>
      ),
    },
    {
      key: "analyzer",
      label: "Analyzer",
      render: (row: FindingItem) => <Badge label={row.analyzer} variant="accent" />,
    },
    {
      key: "category",
      label: "Category",
      render: (row: FindingItem) => <Badge label={row.category} variant="muted" />,
    },
    {
      key: "title",
      label: "Title",
      render: (row: FindingItem) => (
        <span className={cn("font-medium", row.accepted ? "text-aide-text-dim line-through" : "text-aide-text")}>
          {row.title}
        </span>
      ),
    },
    {
      key: "file_path",
      label: "File",
      render: (row: FindingItem) => (
        <button
          onClick={() => {
            setViewerFile(row.file_path);
            setViewerLine(row.line);
            setViewerTitle(row.title);
          }}
          className="bg-transparent px-0 break-all text-aide-text-dim hover:text-aide-accent transition-colors text-left font-mono text-[inherit]"
        >
          {row.file_path}
        </button>
      ),
    },
    {
      key: "line",
      label: "Line",
      sortValue: (row: FindingItem) => row.line,
    },
    {
      key: "_actions",
      label: "",
      sortable: false,
      className: "w-8",
      render: (row: FindingItem) =>
        row.accepted ? (
          <span className="text-aide-green" title="Accepted">
            <Check className="w-3.5 h-3.5" />
          </span>
        ) : (
          <button
            onClick={() => handleAccept(row.id)}
            className="p-1 rounded-sm text-aide-text-dim hover:text-aide-green hover:bg-aide-green/10 transition-colors"
            title="Accept (mark as reviewed)"
          >
            <Check className="w-3.5 h-3.5" />
          </button>
        ),
    },
  ];

  const analyzerOptions = useMemo(() => {
    if (!findings) return [];
    const unique = [...new Set(findings.map((f) => f.analyzer))].sort();
    return unique.map((a) => ({ value: a, label: a }));
  }, [findings]);

  const ULID_RE = /^[0-9A-Z]{26}$/;
  const filtered = useMemo(() => {
    if (!findings) return [];
    const q = query.toLowerCase();
    const isId = ULID_RE.test(query.toUpperCase());
    return findings.filter((f) => {
      if (isId) return f.id.toUpperCase() === query.toUpperCase();
      if (!showAccepted && f.accepted) return false;
      if (severityFilter && f.severity !== severityFilter) return false;
      if (analyzerFilter && f.analyzer !== analyzerFilter) return false;
      if (
        q &&
        !f.title.toLowerCase().includes(q) &&
        !f.file_path.toLowerCase().includes(q) &&
        !f.analyzer.toLowerCase().includes(q) &&
        !f.category.toLowerCase().includes(q) &&
        !f.severity.toLowerCase().includes(q)
      )
        return false;
      return true;
    });
  }, [findings, query, severityFilter, analyzerFilter, showAccepted]);

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Findings
      </h2>

      <FilterBar
        query={query}
        onQueryChange={setQuery}
        placeholder="Filter findings..."
        dropdowns={[
          {
            value: severityFilter,
            onChange: setSeverityFilter,
            options: severityOptions,
            placeholder: "All severities",
          },
          {
            value: analyzerFilter,
            onChange: setAnalyzerFilter,
            options: analyzerOptions,
            placeholder: "All analyzers",
          },
        ]}
        right={
          <button
            onClick={() => setShowAccepted((v) => !v)}
            className={cn(
              "flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium rounded-sm border transition-colors shrink-0",
              showAccepted
                ? "text-aide-accent border-aide-accent/30 bg-aide-accent/10"
                : "text-aide-text-dim border-aide-border hover:text-aide-text-muted"
            )}
            title={showAccepted ? "Showing accepted findings" : "Accepted findings hidden"}
          >
            {showAccepted ? <Eye className="w-3.5 h-3.5" /> : <EyeOff className="w-3.5 h-3.5" />}
            Accepted
          </button>
        }
      />

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {!loading && !error && filtered.length > 0 && (
        <SortableTable
          data={filtered}
          columns={columns}
          keyFn={(row) => row.id}
          emptyMessage="No findings match your filters."
        />
      )}

      {!loading && !error && filtered.length === 0 && (
        <div className="text-center py-12 text-aide-text-dim">
          {findings && findings.length > 0 ? (
            <p>No findings match your filters.</p>
          ) : (
            <>
              <p>No findings yet.</p>
              <p className="mt-1 text-[0.7rem]">
                Findings are populated by static analysis — run{" "}
                <code className="text-aide-accent-light bg-aide-accent/10 px-1.5 py-0.5 rounded">
                  aide findings
                </code>{" "}
                or enable file watching.
              </p>
            </>
          )}
        </div>
      )}

      {viewerFile && (
        <CodeViewer
          open={!!viewerFile}
          onClose={() => setViewerFile(null)}
          project={project!}
          filePath={viewerFile}
          line={viewerLine}
          title={viewerTitle}
        />
      )}
    </div>
  );
}
