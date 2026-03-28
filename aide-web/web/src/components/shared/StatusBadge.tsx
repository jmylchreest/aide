import { cn } from "@/lib/utils";
import type { InstanceStatus } from "@/lib/types";

interface StatusBadgeProps {
  status: InstanceStatus;
  className?: string;
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  return (
    <span
      className={cn(
        "inline-block w-1.5 h-1.5 rounded-full shrink-0",
        status === "connected" && "bg-aide-green shadow-[0_0_6px_theme(colors.aide.green)]",
        status === "disconnected" && "bg-aide-text-dim",
        status === "connecting" && "bg-aide-yellow animate-pulse",
        className
      )}
    />
  );
}
