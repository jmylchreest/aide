import { useState, useEffect, useMemo } from "react";
import { useParams } from "react-router-dom";
import { RefreshCw } from "lucide-react";
import { FilterBar } from "../shared/FilterBar";
import { SortableTable, type Column } from "../shared/SortableTable";
import { Badge } from "../shared/ExpandableCard";
import { CodeViewer } from "../shared/CodeViewer";
import { api } from "../../lib/api";

interface CodeSymbol {
  name: string;
  kind: string;
  language: string;
  file: string;
  line: number;
  signature: string;
}

export function CodePage() {
  const { project } = useParams<{ project: string }>();
  const [filter, setFilter] = useState("");
  const [allSymbols, setAllSymbols] = useState<CodeSymbol[]>([]);
  const [loading, setLoading] = useState(true);
  const [indexing, setIndexing] = useState(false);
  const [indexResult, setIndexResult] = useState<{
    files_indexed: number;
    symbols_indexed: number;
    files_skipped: number;
  } | null>(null);
  const [indexError, setIndexError] = useState<string | null>(null);
  const [viewerFile, setViewerFile] = useState<string | null>(null);
  const [viewerLine, setViewerLine] = useState<number | undefined>();
  const [viewerTitle, setViewerTitle] = useState<string | undefined>();

  function openViewer(file: string, line: number, name: string) {
    setViewerFile(file);
    setViewerLine(line);
    setViewerTitle(name);
  }

  type IndexedSymbol = CodeSymbol & { _i: number };

  const columns: Column<IndexedSymbol>[] = [
    {
      key: "name",
      label: "Name",
      render: (row: IndexedSymbol) => (
        <button
          onClick={() => openViewer(row.file, row.line, row.name)}
          className="font-medium text-aide-accent hover:text-aide-accent-light hover:underline transition-colors text-left"
        >
          {row.name}
        </button>
      ),
    },
    {
      key: "kind",
      label: "Kind",
      render: (row: IndexedSymbol) => <Badge label={row.kind} variant="muted" />,
    },
    {
      key: "language",
      label: "Language",
    },
    {
      key: "file",
      label: "File",
      render: (row: IndexedSymbol) => (
        <button
          onClick={() => openViewer(row.file, row.line, row.name)}
          className="bg-transparent px-0 break-all text-aide-text-dim hover:text-aide-accent transition-colors text-left font-mono text-[inherit]"
        >
          {row.file}
        </button>
      ),
    },
    {
      key: "line",
      label: "Line",
      sortValue: (row: IndexedSymbol) => row.line,
    },
    {
      key: "signature",
      label: "Signature",
      className: "break-all whitespace-pre-wrap max-w-xs",
      render: (row: IndexedSymbol) => (
        <code className="bg-transparent px-0 text-aide-text-dim text-[0.65rem]">
          {row.signature}
        </code>
      ),
    },
  ];

  const fetchSymbols = async () => {
    setLoading(true);
    try {
      // Use wildcard to get all symbols
      const res = await fetch(
        `/instances/${encodeURIComponent(project!)}/code/search.json?q=*`
      );
      if (res.ok) {
        const data = await res.json();
        setAllSymbols(data.symbols ?? []);
      }
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSymbols();
  }, [project]);

  const handleIndex = async () => {
    if (!project) return;
    setIndexing(true);
    setIndexResult(null);
    setIndexError(null);
    try {
      const stats = await api.runCodeIndex(project);
      setIndexResult(stats);
      // Refresh the symbol list after indexing
      fetchSymbols();
    } catch (e) {
      setIndexError(e instanceof Error ? e.message : "Indexing failed");
    } finally {
      setIndexing(false);
    }
  };

  const filtered = useMemo(() => {
    if (!filter) return allSymbols;
    const q = filter.toLowerCase();
    return allSymbols.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        s.kind.toLowerCase().includes(q) ||
        s.language.toLowerCase().includes(q) ||
        s.file.toLowerCase().includes(q) ||
        s.signature.toLowerCase().includes(q)
    );
  }, [allSymbols, filter]);

  const indexed = filtered.map((r, i) => ({ ...r, _i: i }));

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Code Index
        {!loading && (
          <span className="text-xs font-normal text-aide-text-dim ml-2">
            {allSymbols.length} symbols
          </span>
        )}
      </h2>

      <FilterBar
        query={filter}
        onQueryChange={setFilter}
        placeholder="Filter symbols by name, kind, language, or file..."
        right={
          <button
            onClick={handleIndex}
            disabled={indexing}
            className="border border-aide-border text-aide-text-dim px-3 py-1.5 rounded-sm text-xs font-semibold hover:border-aide-accent hover:text-aide-accent transition-all disabled:opacity-50 flex items-center gap-1.5 shrink-0"
          >
            <RefreshCw className={`w-3 h-3 ${indexing ? "animate-spin" : ""}`} />
            {indexing ? "Indexing..." : "Run Index"}
          </button>
        }
      />

      {indexResult && (
        <div className="mb-3 px-3 py-2 rounded border border-aide-accent/30 bg-aide-accent/5 text-xs text-aide-text">
          Index complete: <span className="font-semibold">{indexResult.files_indexed}</span> files indexed,{" "}
          <span className="font-semibold">{indexResult.symbols_indexed}</span> symbols indexed,{" "}
          <span className="font-semibold">{indexResult.files_skipped}</span> files skipped.
        </div>
      )}

      {indexError && (
        <div className="mb-3 px-3 py-2 rounded border border-red-500/30 bg-red-500/5 text-xs text-red-400">
          {indexError}
        </div>
      )}

      {loading && <p className="text-aide-text-dim text-sm">Loading symbols...</p>}

      {!loading && (
        <SortableTable
          data={indexed}
          columns={columns}
          keyFn={(row) => row._i}
          emptyMessage={
            allSymbols.length === 0
              ? "No symbols indexed yet. Click Run Index to build the code index."
              : "No symbols match your filter."
          }
        />
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
