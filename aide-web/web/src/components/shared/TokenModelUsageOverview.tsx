import { useState } from "react";
import type { ModelUsage, ModelUsageSource } from "../../lib/types";
import { supportedModelUsage } from "./TokenModelUsage";

const identity = (row: ModelUsageSource) =>
  JSON.stringify([row.host, row.source, row.model ?? "", row.provider ?? ""]);
const sourceLabel = (row: ModelUsageSource) =>
  [row.host, row.model || "Model unknown", row.provider]
    .filter(Boolean)
    .join(" · ");
const inputParts = [
  { key: "uncached_input_tokens", label: "Uncached", color: "bg-amber-400" },
  { key: "cache_read_input_tokens", label: "Cached", color: "bg-blue-400" },
  {
    key: "cache_write_input_tokens",
    label: "Cache write",
    color: "bg-violet-400",
  },
];

/** Aggregate coverage counts cannot establish shared subsets unless all are complete. */
export function modelInputSplit(row: ModelUsageSource) {
  const total = row.counters.input_tokens;
  if (!total || total.observations !== row.observations || total.tokens <= 0)
    return;
  if (
    inputParts.some(
      ({ key }) => row.counters[key]?.observations !== row.observations,
    )
  )
    return;
  const sum = inputParts.reduce(
    (n, { key }) => n + row.counters[key].tokens,
    0,
  );
  if (!Number.isSafeInteger(sum) || sum !== total.tokens) return;
  return inputParts.map((part) => ({
    ...part,
    tokens: row.counters[part.key].tokens,
    percent: (row.counters[part.key].tokens / total.tokens) * 100,
  }));
}

export function TokenModelUsageOverview({
  report,
  onAccounting,
}: {
  report?: ModelUsage | null;
  onAccounting: () => void;
}) {
  const usage = supportedModelUsage(report);
  const [selected, setSelected] = useState("");
  const row =
    usage?.by_source.find((item) => identity(item) === selected) ??
    usage?.by_source[0];
  const split = row && modelInputSplit(row);
  const rawOutput =
    row && !row.counters.output_tokens && !!row.counters.reported_output_tokens;
  const counters = [
    { key: "input_tokens", label: "Input", note: "Includes cache" },
    {
      key: rawOutput ? "reported_output_tokens" : "output_tokens",
      label: rawOutput ? "Reported output" : "Output",
      note: rawOutput
        ? "Reasoning overlap unknown"
        : "Includes reasoning when known",
    },
    {
      key: "cache_read_input_tokens",
      label: "Cached input",
      note: "Cache reads · part of input",
    },
    {
      key: "uncached_input_tokens",
      label: "Uncached input",
      note: "Part of input",
    },
    {
      key: "cache_write_input_tokens",
      label: "Cache write",
      note: "Part of input",
    },
    {
      key: "reasoning_output_tokens",
      label: "Reasoning",
      note: rawOutput
        ? "Overlap with output unknown"
        : "Part of output when known",
    },
  ];
  return (
    <section
      aria-label="Model token usage"
      className="border border-aide-border rounded-md p-4 mb-5 min-w-0"
    >
      <div className="flex justify-between items-center gap-3 flex-wrap mb-3">
        <h3 className="text-xs font-semibold text-aide-text">Model tokens</h3>
        <button
          type="button"
          onClick={onAccounting}
          className="text-xs text-aide-accent hover:underline"
        >
          Usage details →
        </button>
      </div>
      {row ? (
        <>
          {usage!.by_source.length > 1 ? (
            <label className="block text-xs text-aide-text-muted mb-3">
              Usage source
              <select
                aria-label="Model usage source"
                value={identity(row)}
                onChange={(event) => setSelected(event.target.value)}
                className="block w-full min-w-0 mt-1 bg-aide-surface border border-aide-border rounded px-2 py-1 text-aide-text"
              >
                {usage!.by_source.map((item) => (
                  <option key={identity(item)} value={identity(item)}>
                    {sourceLabel(item)}
                    {usage!.by_source.filter(
                      (other) => sourceLabel(other) === sourceLabel(item),
                    ).length > 1
                      ? ` · ${item.source}`
                      : ""}
                  </option>
                ))}
              </select>
            </label>
          ) : (
            <p className="text-xs text-aide-text-muted mb-3 break-words">
              {sourceLabel(row)}
            </p>
          )}
          <dl className="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-6 gap-3">
            {counters.map(({ key, label, note }) => {
              const counter = row.counters[key];
              return (
                <div key={key} className="min-w-0">
                  <dt className="text-[10px] text-aide-text-dim">{label}</dt>
                  <dd
                    className="text-xl font-semibold text-aide-text tabular-nums break-words"
                    title={
                      counter
                        ? `${counter.tokens.toLocaleString()} tokens`
                        : "Unknown"
                    }
                  >
                    {counter ? counter.tokens.toLocaleString() : "Unknown"}
                  </dd>
                  <dd className="text-[10px] text-aide-text-muted">{note}</dd>
                  {counter && counter.observations !== row.observations && (
                    <dd className="text-[10px] text-aide-text-muted">
                      {counter.observations}/{row.observations} observations
                    </dd>
                  )}
                </div>
              );
            })}
          </dl>
          {split && (
            <div className="mt-4">
              <div
                role="img"
                aria-label={`Input breakdown: ${split.map((part) => `${part.label} ${part.tokens.toLocaleString()}`).join(", ")}`}
                className="flex h-2 rounded overflow-hidden"
              >
                {split.map((part) => (
                  <span
                    key={part.key}
                    className={part.color}
                    style={{ width: `${part.percent}%` }}
                  />
                ))}
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 mt-2 text-[10px] text-aide-text-muted">
                {split.map((part) => (
                  <span key={part.key}>
                    <span
                      aria-hidden="true"
                      className={`inline-block w-2 h-2 mr-1 rounded-sm ${part.color}`}
                    />
                    {part.label}
                  </span>
                ))}
              </div>
            </div>
          )}
          <p className="text-[11px] text-aide-text-dim mt-3">
            Host-reported · {row.observations.toLocaleString()} observations ·
            partial coverage. Counters can overlap.
            {!!(usage!.conflicts || usage!.invalid) &&
              " Conflicting or invalid records excluded; see usage details."}
          </p>
        </>
      ) : (
        <p className="text-xs text-aide-text-muted">
          {usage
            ? "No model usage observations in this selection; usage is unknown."
            : "Model usage accounting is unavailable from this server."}
        </p>
      )}
    </section>
  );
}
