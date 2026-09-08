/** A deliberately narrow set of documented, shape-preserving replacements.
 * Unsupported native tools and mixed media keep their complete original. */
export function replacementTarget(
  tool: string,
  payload: unknown,
): { text: string; replace: (text: string) => unknown } | null {
  if (tool.toLowerCase() === "bash" && payload && typeof payload === "object") {
    const p = payload as Record<string, unknown>;
    if ("output" in p) {
      const metadata = p.metadata;
      if (
        typeof p.output !== "string" ||
        !metadata || typeof metadata !== "object" || Array.isArray(metadata) ||
        !Number.isSafeInteger((metadata as Record<string, unknown>).exit) ||
        p.stdout !== undefined || p.stderr !== undefined ||
        p.content !== undefined || p.structuredContent !== undefined ||
        p.isImage === true
      ) return null;
      return { text: p.output, replace: (text) => ({ ...p, output: text }) };
    }
    if (
      typeof p.stdout !== "string" ||
      typeof p.stderr !== "string" ||
      p.isImage !== false ||
      typeof p.interrupted !== "boolean"
    )
      return null;
    const key = p.stdout.length >= p.stderr.length ? "stdout" : "stderr";
    return {
      text: p[key] as string,
      replace: (text) => ({ ...p, [key]: text }),
    };
  }
  if (!tool.toLowerCase().startsWith("mcp__")) return null;
  if (typeof payload === "string")
    return { text: payload, replace: (text) => text };
  if (!payload || typeof payload !== "object" || Array.isArray(payload))
    return null;
  const p = payload as Record<string, unknown>;
  // structuredContent is another representation and must not contradict a
  // shortened text result. Leave such results untouched.
  if (
    p.structuredContent !== undefined ||
    !Array.isArray(p.content) ||
    p.content.length === 0
  )
    return null;
  const blocks = p.content as Record<string, unknown>[];
  if (
    !blocks.every((b) => b && b.type === "text" && typeof b.text === "string")
  )
    return null;
  return {
    text: blocks.map((b) => b.text).join(""),
    replace: (text) => ({
      ...p,
      content: blocks.map((b, i) => ({ ...b, text: i === 0 ? text : "" })),
    }),
  };
}
