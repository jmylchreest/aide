import { useClipboard } from "@/context/ClipboardContext";
import { useParams } from "react-router-dom";
import { Clipboard, X, ClipboardPaste } from "lucide-react";
import { cn } from "@/lib/utils";

interface ClipboardBannerProps {
  /** Which item types this page can accept */
  accepts: ("memory" | "decision")[];
  onPaste: () => void;
}

export function ClipboardBanner({ accepts, onPaste }: ClipboardBannerProps) {
  const { item, clear } = useClipboard();
  const { project } = useParams();

  if (!item) return null;
  if (!accepts.includes(item.type)) return null;

  const isSameInstance = item.sourceInstance === project;

  return (
    <div
      className={cn(
        "flex items-center gap-2 px-3 py-2 mb-3 rounded border text-xs",
        isSameInstance
          ? "bg-aide-surface border-aide-border text-aide-text-dim"
          : "bg-aide-accent/5 border-aide-accent/20 text-aide-accent"
      )}
    >
      <Clipboard className="w-3.5 h-3.5 shrink-0" />
      <span className="flex-1 min-w-0 truncate">
        Copied {item.type} from <strong>{item.sourceInstance}</strong>: {item.label}
      </span>
      {!isSameInstance && (
        <button
          onClick={onPaste}
          className="inline-flex items-center gap-1 px-2 py-1 text-xs font-medium text-aide-accent border border-aide-accent/30 rounded-sm hover:bg-aide-accent/10 transition-colors shrink-0"
        >
          <ClipboardPaste className="w-3 h-3" />
          Paste here
        </button>
      )}
      <button
        onClick={clear}
        className="p-0.5 rounded-sm text-aide-text-dim hover:text-aide-text hover:bg-aide-surface-hover transition-colors shrink-0"
        title="Clear clipboard"
      >
        <X className="w-3 h-3" />
      </button>
    </div>
  );
}
