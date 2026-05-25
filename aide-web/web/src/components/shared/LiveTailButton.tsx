import { Radio } from "lucide-react";
import type { EventStreamStatus } from "@/hooks/useEventStream";

interface LiveTailButtonProps {
  active: boolean;
  onToggle: () => void;
  status?: EventStreamStatus;
}

export function LiveTailButton({ active, onToggle, status }: LiveTailButtonProps) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={
        "inline-flex items-center gap-1.5 rounded px-2.5 py-1.5 text-xs border transition " +
        (active
          ? "bg-aide-accent text-white border-aide-accent"
          : "bg-aide-surface text-aide-text border-aide-border hover:bg-aide-accent/5")
      }
      title={active && status ? `stream: ${status}` : undefined}
    >
      <Radio className={"w-3.5 h-3.5 " + (active ? "animate-pulse" : "")} />
      {active ? "Live" : "Tail"}
    </button>
  );
}
