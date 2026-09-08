const labels: Record<string, string> = {
  full_file: "Full-file text matched",
  range: "Source range matched",
  referenced: "Source receipt matched",
  unverified: "Source delivery unverified",
  failed: "Retrieval failed",
  pending: "Retrieval still running",
  search: "Search observed; file delivery unverified",
  unclassified_shell: "Shell retrieval coverage unknown",
};

export function TokenRetrievalEvidence({
  attrs,
}: {
  attrs: Record<string, string>;
}) {
  if (!attrs.retrieval_status && !attrs.source_references) return null;
  let references: { file: string; bytes: number; sha256: string }[] = [];
  try {
    const parsed = JSON.parse(attrs.source_references ?? "null");
    if (Array.isArray(parsed) && parsed.length <= 10)
      references = parsed.filter(
        (ref) =>
          ref &&
          typeof ref.file === "string" &&
          Number.isSafeInteger(ref.bytes) &&
          ref.bytes >= 0 &&
          typeof ref.sha256 === "string" &&
          /^[a-f0-9]{64}$/.test(ref.sha256),
      );
  } catch {
    /* Old or malformed evidence stays unavailable. */
  }
  return (
    <div className="mt-2 space-y-1 border-t border-aide-border pt-2">
      <div>
        {labels[attrs.retrieval_status] ??
          "Server source reference; host delivery unknown"}
      </div>
      {attrs.retrieval_method && <div>Method: {attrs.retrieval_method}</div>}
      {attrs.retrieval_target && <div>Target: {attrs.retrieval_target}</div>}
      {attrs.source_verification === "current_range_match" && (
        <div>
          Lines {attrs.delivered_start_line}–{attrs.delivered_end_line} matched
          the current file. Undisplayed source version is unverified.
        </div>
      )}
      {references.map((ref) => (
        <div key={ref.file}>
          <div>
            Conditional reference: {ref.file} · {ref.bytes.toLocaleString()}{" "}
            bytes
          </div>
          <div className="font-mono break-all">SHA-256: {ref.sha256}</div>
        </div>
      ))}
      {attrs.retrieval_id && <div>Receipt: {attrs.retrieval_id}</div>}
      {references.length > 0 && (
        <div>Reference size is not evidence of an avoided read.</div>
      )}
    </div>
  );
}
