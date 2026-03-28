import { createContext, useContext, useState, useCallback } from "react";

export type ClipboardItemType = "memory" | "decision";

export interface ClipboardEntry {
  type: ClipboardItemType;
  sourceInstance: string;
  data: Record<string, unknown>;
  label: string; // human-readable summary
}

interface ClipboardContextValue {
  item: ClipboardEntry | null;
  copy: (entry: ClipboardEntry) => void;
  clear: () => void;
}

const ClipboardContext = createContext<ClipboardContextValue>({
  item: null,
  copy: () => {},
  clear: () => {},
});

export function ClipboardProvider({ children }: { children: React.ReactNode }) {
  const [item, setItem] = useState<ClipboardEntry | null>(null);

  const copy = useCallback((entry: ClipboardEntry) => {
    setItem(entry);
  }, []);

  const clear = useCallback(() => {
    setItem(null);
  }, []);

  return (
    <ClipboardContext.Provider value={{ item, copy, clear }}>
      {children}
    </ClipboardContext.Provider>
  );
}

export function useClipboard() {
  return useContext(ClipboardContext);
}
