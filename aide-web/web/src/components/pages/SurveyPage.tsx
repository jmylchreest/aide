import { useState, useMemo } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { FilterBar } from "../shared/FilterBar";
import { SortableTable, type Column } from "../shared/SortableTable";
import { Badge } from "../shared/ExpandableCard";
import type { SurveyItem } from "@/lib/types";

const columns: Column<SurveyItem>[] = [
  {
    key: "analyzer",
    label: "Analyzer",
  },
  {
    key: "kind",
    label: "Kind",
    render: (row) => <Badge label={row.kind} variant="accent" />,
  },
  {
    key: "name",
    label: "Name",
    render: (row) => (
      <span className="font-medium text-aide-text">{row.name}</span>
    ),
  },
  {
    key: "file_path",
    label: "File",
    render: (row) => (
      <code className="bg-transparent px-0 break-all">{row.file_path}</code>
    ),
  },
  {
    key: "title",
    label: "Title",
  },
];

export function SurveyPage() {
  const { project } = useParams<{ project: string }>();
  const [searchParams] = useSearchParams();
  const [query, setQuery] = useState(() => searchParams.get("q") ?? "");
  const [kindFilter, setKindFilter] = useState("");
  const [analyzerFilter, setAnalyzerFilter] = useState("");

  const { data: entries, loading, error } = useApi(
    () => api.listSurvey(project!),
    [project]
  );

  const kindOptions = useMemo(() => {
    if (!entries) return [];
    const unique = [...new Set(entries.map((e) => e.kind))].sort();
    return unique.map((k) => ({ value: k, label: k }));
  }, [entries]);

  const analyzerOptions = useMemo(() => {
    if (!entries) return [];
    const unique = [...new Set(entries.map((e) => e.analyzer))].sort();
    return unique.map((a) => ({ value: a, label: a }));
  }, [entries]);

  const ULID_RE = /^[0-9A-Z]{26}$/;
  const filtered = useMemo(() => {
    if (!entries) return [];
    const q = query.toLowerCase();
    const isId = ULID_RE.test(query.toUpperCase());
    return entries.filter((e) => {
      if (isId) return e.id.toUpperCase() === query.toUpperCase();
      if (kindFilter && e.kind !== kindFilter) return false;
      if (analyzerFilter && e.analyzer !== analyzerFilter) return false;
      if (
        q &&
        !e.name.toLowerCase().includes(q) &&
        !e.kind.toLowerCase().includes(q) &&
        !e.analyzer.toLowerCase().includes(q) &&
        !e.file_path.toLowerCase().includes(q) &&
        !e.title.toLowerCase().includes(q)
      )
        return false;
      return true;
    });
  }, [entries, query, kindFilter, analyzerFilter]);

  return (
    <div>
      <h2 className="text-base font-semibold pb-1.5 border-b border-aide-border mb-3">
        Survey
      </h2>

      <FilterBar
        query={query}
        onQueryChange={setQuery}
        placeholder="Filter survey entries..."
        dropdowns={[
          {
            value: kindFilter,
            onChange: setKindFilter,
            options: kindOptions,
            placeholder: "All kinds",
          },
          {
            value: analyzerFilter,
            onChange: setAnalyzerFilter,
            options: analyzerOptions,
            placeholder: "All analyzers",
          },
        ]}
      />

      {loading && <p className="text-aide-text-dim text-sm">Loading...</p>}
      {error && <p className="text-aide-red text-sm">{error}</p>}

      {!loading && !error && (
        <SortableTable
          data={filtered}
          columns={columns}
          keyFn={(row) => row.id}
          emptyMessage="No survey entries found."
        />
      )}
    </div>
  );
}
