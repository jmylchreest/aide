import { useState, useCallback, useRef, useEffect } from "react";
import { ChevronLeft, ChevronRight, Calendar } from "lucide-react";

// ── Helpers ────────────────────────────────────────────────────────────

function daysInMonth(year: number, month: number) {
  return new Date(year, month + 1, 0).getDate();
}

function startOfDay(d: Date) {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

function sameDay(a: Date, b: Date) {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

function fmtShort(d: Date | null) {
  if (!d) return "—";
  const mon = d.toLocaleString("default", { month: "short" });
  return `${mon} ${d.getDate()}, ${d.getFullYear()}`;
}

function toISO(d: Date | null, endOfDay: boolean): string {
  if (!d) return "";
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}` + (endOfDay ? "T23:59:59Z" : "T00:00:00Z");
}

// Build 42-cell grid (6 rows × 7 cols) for a given month
function buildGrid(year: number, month: number): Date[] {
  const firstDay = new Date(year, month, 1).getDay(); // 0=Sun
  const total = daysInMonth(year, month);
  const cells: Date[] = [];

  // Previous month padding
  for (let i = firstDay - 1; i >= 0; i--) {
    cells.push(new Date(year, month, -i));
  }
  // Current month
  for (let d = 1; d <= total; d++) {
    cells.push(new Date(year, month, d));
  }
  // Next month padding to fill to 42 (or 35 if fits in 5 rows)
  const target = cells.length > 35 ? 42 : 35;
  let next = 1;
  while (cells.length < target) {
    cells.push(new Date(year, month + 1, next++));
  }
  return cells;
}

// ── Types ──────────────────────────────────────────────────────────────

export type RelativePreset = "7d" | "30d" | "90d" | "all";

export interface DateRangeValue {
  preset: RelativePreset | "custom";
  since: string; // RFC3339 or ""
  until: string; // RFC3339 or ""
}

const PRESETS: { value: RelativePreset; label: string; days: number | null }[] = [
  { value: "7d", label: "Last 7 days", days: 7 },
  { value: "30d", label: "Last 30 days", days: 30 },
  { value: "90d", label: "Last 90 days", days: 90 },
  { value: "all", label: "All time", days: null },
];

export function presetToRange(preset: RelativePreset): { since: string; until: string } {
  const p = PRESETS.find((x) => x.value === preset);
  if (!p?.days) return { since: "", until: "" };
  const d = new Date();
  d.setDate(d.getDate() - p.days);
  d.setHours(0, 0, 0, 0);
  return { since: d.toISOString(), until: "" };
}

// ── Component ──────────────────────────────────────────────────────────

const WEEKDAYS = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];

export function DateRangePicker({
  value,
  onChange,
}: {
  value: DateRangeValue;
  onChange: (v: DateRangeValue) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // Parse value into Date objects for calendar state
  const fromDate = value.since ? startOfDay(new Date(value.since)) : null;
  const toDate = value.until ? startOfDay(new Date(value.until)) : null;

  // Selection state: picking start vs end
  const [picking, setPicking] = useState<"start" | "end">("start");
  const [tempFrom, setTempFrom] = useState<Date | null>(fromDate);
  const [tempTo, setTempTo] = useState<Date | null>(toDate);
  const [hovered, setHovered] = useState<Date | null>(null);

  // Calendar month
  const [viewDate, setViewDate] = useState(() => {
    if (fromDate) return new Date(fromDate.getFullYear(), fromDate.getMonth(), 1);
    return new Date(new Date().getFullYear(), new Date().getMonth(), 1);
  });

  const viewYear = viewDate.getFullYear();
  const viewMonth = viewDate.getMonth();
  const grid = buildGrid(viewYear, viewMonth);
  const today = startOfDay(new Date());

  const prevMonth = useCallback(
    () => setViewDate(new Date(viewYear, viewMonth - 1, 1)),
    [viewYear, viewMonth],
  );
  const nextMonth = useCallback(
    () => setViewDate(new Date(viewYear, viewMonth + 1, 1)),
    [viewYear, viewMonth],
  );

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  // Sync temp state when opening
  useEffect(() => {
    if (open) {
      setTempFrom(fromDate);
      setTempTo(toDate);
      setPicking(fromDate && !toDate ? "end" : "start");
      if (fromDate) setViewDate(new Date(fromDate.getFullYear(), fromDate.getMonth(), 1));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const handleDayClick = useCallback(
    (d: Date) => {
      if (picking === "start" || (tempFrom && tempTo)) {
        // Starting new selection
        setTempFrom(d);
        setTempTo(null);
        setPicking("end");
      } else {
        // Completing selection
        if (d < tempFrom!) {
          setTempFrom(d);
          setTempTo(tempFrom);
        } else if (sameDay(d, tempFrom!)) {
          // Same day = single-day range
          setTempTo(d);
        } else {
          setTempTo(d);
        }
        setPicking("start");
      }
    },
    [picking, tempFrom, tempTo],
  );

  const apply = useCallback(() => {
    onChange({
      preset: "custom",
      since: toISO(tempFrom, false),
      until: toISO(tempTo, true),
    });
    setOpen(false);
  }, [onChange, tempFrom, tempTo]);

  const handlePreset = useCallback(
    (preset: RelativePreset) => {
      const { since, until } = presetToRange(preset);
      onChange({ preset, since, until });
      setOpen(false);
    },
    [onChange],
  );

  // Classify a day cell
  const classify = (d: Date) => {
    const isCurrentMonth = d.getMonth() === viewMonth;
    const isToday = sameDay(d, today);
    const effectiveTo = tempTo ?? (picking === "end" ? hovered : null);
    const rangeFrom = tempFrom;
    const rangeTo = effectiveTo;
    let inRange = false;
    let isStart = false;
    let isEnd = false;

    if (rangeFrom && rangeTo) {
      const lo = rangeFrom < rangeTo ? rangeFrom : rangeTo;
      const hi = rangeFrom < rangeTo ? rangeTo : rangeFrom;
      inRange = d >= lo && d <= hi;
      isStart = sameDay(d, lo);
      isEnd = sameDay(d, hi);
    } else if (rangeFrom && sameDay(d, rangeFrom)) {
      isStart = true;
      isEnd = true;
      inRange = true;
    }

    return { isCurrentMonth, isToday, inRange, isStart, isEnd };
  };

  // Trigger button label
  const presetObj = PRESETS.find((p) => p.value === value.preset);
  const triggerLabel =
    value.preset !== "custom"
      ? presetObj?.label ?? "Select"
      : fromDate
        ? `${fmtShort(fromDate)} — ${fmtShort(toDate)}`
        : "Select dates";

  const monthLabel = viewDate.toLocaleString("default", { month: "long", year: "numeric" });

  return (
    <div className="relative inline-block" ref={ref}>
      {/* Trigger */}
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded border text-[11px] transition ${
          open
            ? "border-aide-accent bg-aide-surface text-aide-accent"
            : "border-aide-border bg-aide-surface text-aide-text-muted hover:border-aide-border-light"
        }`}
      >
        <Calendar className="w-3 h-3" />
        {triggerLabel}
      </button>

      {/* Popover */}
      {open && (
        <div className="absolute left-0 top-full mt-1 z-50 flex rounded border border-aide-border bg-aide-surface shadow-lg shadow-black/40">
          {/* Presets sidebar */}
          <div className="flex flex-col gap-0.5 p-2 border-r border-aide-border min-w-[120px]">
            <span className="text-[9px] uppercase tracking-wider text-aide-text-dim mb-1 px-2">Quick select</span>
            {PRESETS.map((p) => (
              <button
                key={p.value}
                type="button"
                onClick={() => handlePreset(p.value)}
                className={`text-left px-2 py-1 rounded text-[11px] transition ${
                  value.preset === p.value
                    ? "bg-aide-accent/15 text-aide-accent"
                    : "text-aide-text-muted hover:bg-aide-surface-hover hover:text-aide-text"
                }`}
              >
                {p.label}
              </button>
            ))}
          </div>

          {/* Calendar */}
          <div className="p-2.5 w-[260px]">
            {/* Month header */}
            <div className="flex items-center justify-between mb-2">
              <button
                type="button"
                onClick={prevMonth}
                className="p-0.5 rounded hover:bg-aide-surface-hover text-aide-text-dim hover:text-aide-text transition"
              >
                <ChevronLeft className="w-3.5 h-3.5" />
              </button>
              <span className="text-[11px] font-medium text-aide-text">{monthLabel}</span>
              <button
                type="button"
                onClick={nextMonth}
                className="p-0.5 rounded hover:bg-aide-surface-hover text-aide-text-dim hover:text-aide-text transition"
              >
                <ChevronRight className="w-3.5 h-3.5" />
              </button>
            </div>

            {/* Weekday headers */}
            <div className="grid grid-cols-7 mb-1">
              {WEEKDAYS.map((wd) => (
                <div key={wd} className="text-center text-[9px] text-aide-text-dim font-medium py-0.5">
                  {wd}
                </div>
              ))}
            </div>

            {/* Day grid */}
            <div className="grid grid-cols-7">
              {grid.map((d, i) => {
                const { isCurrentMonth, isToday, inRange, isStart, isEnd } = classify(d);
                const isEndpoint = isStart || isEnd;

                // Background strip behind the cell (the range bar)
                let stripCls = "";
                if (inRange && !isStart && !isEnd) stripCls = "bg-aide-accent/10";
                else if (inRange && isStart && !isEnd) stripCls = "bg-gradient-to-r from-transparent to-aide-accent/10 via-aide-accent/10 via-50%";
                else if (inRange && isEnd && !isStart) stripCls = "bg-gradient-to-l from-transparent to-aide-accent/10 via-aide-accent/10 via-50%";

                return (
                  <div key={i} className={`relative flex items-center justify-center h-[28px] ${stripCls}`}>
                    <button
                      type="button"
                      onClick={() => handleDayClick(d)}
                      onMouseEnter={() => picking === "end" && setHovered(d)}
                      onMouseLeave={() => setHovered(null)}
                      className={`
                        relative z-10 w-[26px] h-[26px] rounded-full text-[11px] transition-colors
                        ${!isCurrentMonth ? "text-aide-text-dim/40" : ""}
                        ${isCurrentMonth && !isEndpoint && !inRange ? "text-aide-text-muted hover:bg-aide-surface-hover hover:text-aide-text" : ""}
                        ${isCurrentMonth && inRange && !isEndpoint ? "text-aide-accent" : ""}
                        ${isEndpoint ? "bg-aide-accent text-white font-medium" : ""}
                        ${isToday && !isEndpoint ? "ring-1 ring-aide-border-light" : ""}
                      `}
                    >
                      {d.getDate()}
                    </button>
                  </div>
                );
              })}
            </div>

            {/* Footer: selection hint + apply */}
            <div className="flex items-center justify-between mt-2 pt-2 border-t border-aide-border">
              <span className="text-[10px] text-aide-text-dim">
                {picking === "end" && tempFrom
                  ? `Select end date`
                  : tempFrom && tempTo
                    ? `${fmtShort(tempFrom)} — ${fmtShort(tempTo)}`
                    : "Select start date"}
              </span>
              <button
                type="button"
                onClick={apply}
                disabled={!tempFrom}
                className="px-2.5 py-1 rounded text-[11px] font-medium transition bg-aide-accent text-white hover:bg-aide-accent-dark disabled:opacity-40 disabled:cursor-not-allowed"
              >
                Apply
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
