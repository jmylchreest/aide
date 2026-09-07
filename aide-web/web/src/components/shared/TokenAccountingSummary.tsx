import type { TokenAccounting, TokenQuantity } from "../../lib/types";
import { TokenTransformationSummary } from "./TokenTransformations";

function Quantity({
  label,
  quantity,
}: {
  label: string;
  quantity?: TokenQuantity;
}) {
  const known = quantity && quantity.events > 0;
  return (
    <div className="rounded-md border border-aide-border bg-aide-surface px-4 py-3">
      <div className="text-[10px] uppercase tracking-wider text-aide-text-dim mb-1">
        {label}
      </div>
      <div className="text-xl font-semibold text-aide-text">
        {known ? `${quantity.bytes.toLocaleString()} bytes` : "Unknown"}
      </div>
      <div className="text-[11px] text-aide-text-muted mt-1">
        {known
          ? `~${quantity.estimated_tokens.toLocaleString()} tokens · ${quantity.events} ${quantity.events === 1 ? "observation" : "observations"}`
          : "No measured text in this selection"}
      </div>
    </div>
  );
}

export function TokenAccountingSummary({
  accounting,
}: {
  accounting?: TokenAccounting;
}) {
  if (!accounting || accounting.version !== 1)
    return (
      <p className="text-xs text-aide-text-muted mb-4" role="status">
        Accounting unavailable. This server does not provide a supported
        evidence report; historical estimates remain below.
      </p>
    );
  return (
    <section aria-label="Observed text accounting" className="mb-6">
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
        <Quantity
          label="Host result text"
          quantity={accounting.by_stage.host_result}
        />
        <Quantity
          label="Server result text"
          quantity={accounting.by_stage.server_result}
        />
        <Quantity
          label="Generated argument text"
          quantity={accounting.arguments}
        />
      </div>
      <p className="text-[11px] text-aide-text-muted mt-2">
        {accounting.legacy_events} legacy · {accounting.missing_payload} missing
        text · {accounting.missing_identity} missing identity. Coverage
        describes recorded observations; unseen calls are unknown.
      </p>
      <details className="text-[11px] text-aide-text-dim mt-2">
        <summary className="cursor-pointer">Measurement and evidence</summary>
        <div className="mt-2 space-y-2 border-l border-aide-border pl-3">
          <p>
            Bytes count supported UTF-8 text at the observed hook or server
            boundary. Opaque media and provider framing are excluded. Generated
            argument text covers edit/write content fields, not all tool
            arguments. Token estimates use {accounting.estimator}.
          </p>
          <p>
            Server and host observations can overlap; do not add them. A hook
            observation does not confirm what survived subsequent
            transformations or reached the model.
          </p>
          <p>
            Final delivery, full-file episode comparisons and inferred avoidance
            remain unavailable in this report. Historical comparison estimates
            below do not establish avoided calls or provider savings.
          </p>
        </div>
      </details>
      <TokenTransformationSummary report={accounting.transformations} />
    </section>
  );
}
