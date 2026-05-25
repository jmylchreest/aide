export function formatTimestamp(ts: string): string {
  try {
    const d = new Date(ts);
    return d.toISOString().replace("T", " ").replace("Z", "").slice(0, 23);
  } catch {
    return ts;
  }
}

export function deltaMs(from: string, to: string): number {
  try {
    return Math.max(0, new Date(to).getTime() - new Date(from).getTime());
  } catch {
    return 0;
  }
}
