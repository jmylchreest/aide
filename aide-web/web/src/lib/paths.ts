/** Shorten only complete root prefixes; sibling projects must stay distinct. */
export function relativeToRoot(filePath: string, root?: string): string {
  if (!root) return filePath;
  const normalized = filePath.replace(/\\/g, "/");
  const base = root.replace(/\\/g, "/").replace(/\/+$/, "");
  const windows = /^[a-z]:\//i.test(normalized) || normalized.startsWith("//");
  const candidate = windows ? normalized.toLowerCase() : normalized;
  const prefix = windows ? base.toLowerCase() : base;
  if (candidate === prefix) return ".";
  return candidate.startsWith(prefix + "/")
    ? normalized.slice(base.length + 1)
    : filePath;
}
