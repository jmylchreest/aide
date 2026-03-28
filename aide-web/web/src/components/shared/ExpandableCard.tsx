import { useState, useRef, useEffect } from "react";
import { cn } from "@/lib/utils";
import { ChevronDown } from "lucide-react";

interface BadgeProps {
  label: string;
  variant?: "accent" | "muted" | "green" | "red" | "yellow";
}

const badgeVariants: Record<string, string> = {
  accent: "text-aide-accent bg-aide-accent/10",
  muted: "text-aide-text-dim bg-white/5 border border-white/8",
  green: "text-aide-green bg-aide-green/10",
  red: "text-aide-red bg-aide-red/10",
  yellow: "text-aide-yellow bg-aide-yellow/10",
};

export function Badge({ label, variant = "accent" }: BadgeProps) {
  return (
    <span
      className={cn(
        "text-[0.65rem] font-semibold uppercase tracking-wide px-1.5 py-0.5 rounded shrink-0",
        badgeVariants[variant] ?? badgeVariants.accent
      )}
    >
      {label}
    </span>
  );
}

export function Tag({ label }: { label: string }) {
  return (
    <span className="text-[0.65rem] text-aide-text-dim bg-white/5 border border-white/10 px-1.5 py-0.5 rounded">
      {label}
    </span>
  );
}

interface ExpandableCardProps {
  header: React.ReactNode;
  headerRight?: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  defaultExpanded?: boolean;
}

const COLLAPSED_HEIGHT = 60; // px — ~3 lines of text

export function ExpandableCard({
  header,
  headerRight,
  children,
  footer,
  defaultExpanded = false,
}: ExpandableCardProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [overflows, setOverflows] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = contentRef.current;
    if (el) {
      setOverflows(el.scrollHeight > COLLAPSED_HEIGHT);
    }
  }, [children]);

  return (
    <div
      className={cn(
        "bg-aide-surface border border-aide-border rounded p-3 transition-colors hover:border-aide-border-light",
        overflows && "cursor-pointer"
      )}
      onClick={overflows ? () => setExpanded((e) => !e) : undefined}
    >
      <div className="flex items-start justify-between gap-2 mb-1">
        <div className="flex items-center gap-2 min-w-0 flex-wrap">{header}</div>
        <div className="flex items-center gap-1.5 shrink-0">
          {headerRight}
          {overflows && (
            <ChevronDown
              className={cn(
                "w-3.5 h-3.5 text-aide-text-dim transition-transform",
                expanded && "rotate-180"
              )}
            />
          )}
        </div>
      </div>
      <div
        ref={contentRef}
        className={cn(
          "text-xs text-aide-text-muted leading-relaxed whitespace-pre-wrap break-words",
          !expanded && overflows && "overflow-hidden relative"
        )}
        style={!expanded && overflows ? { maxHeight: COLLAPSED_HEIGHT } : undefined}
      >
        {children}
        {!expanded && overflows && (
          <div className="absolute bottom-0 left-0 right-0 h-6 bg-gradient-to-t from-aide-surface to-transparent pointer-events-none" />
        )}
      </div>
      {footer && (
        <div className="mt-1.5 pt-1.5 border-t border-aide-border">{footer}</div>
      )}
    </div>
  );
}
