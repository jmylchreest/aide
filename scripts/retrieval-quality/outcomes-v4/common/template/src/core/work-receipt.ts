/** Join evidence for an observed MCP response, not proof of model delivery. */
import { createHash } from "crypto";

const object = (value: unknown): Record<string, unknown> | undefined =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;

// These are the existing aide connection names used by the three adapters.
// A similarly named native tool or another MCP server is not aide work.
export function aideWorkTool(name: string): string | undefined {
  return /^(?:mcp__(?:aide|plugin_aide_aide)__|aide_)([a-z][a-z0-9_]{0,127})$/.exec(
    name,
  )?.[1];
}

export function workReceiptEvidence(
  tool: string | undefined,
  text: string | undefined,
  response: unknown,
): Record<string, string> {
  const receipt = object(object(object(response)?._meta)?.["aide/work"]);
  if (
    !tool ||
    !receipt ||
    receipt.version !== 1 ||
    receipt.tool !== tool ||
    text === undefined ||
    typeof receipt.id !== "string" ||
    !receipt.id.trim() ||
    receipt.id.length > 128 ||
    Array.from(receipt.id).some(
      (c) => c.charCodeAt(0) < 32 || c.charCodeAt(0) === 127,
    ) ||
    receipt.text_sha256 !== createHash("sha256").update(text).digest("hex")
  )
    return {};
  return {
    work_receipt_version: "1",
    work_id: receipt.id,
    work_text_sha256: receipt.text_sha256 as string,
  };
}
