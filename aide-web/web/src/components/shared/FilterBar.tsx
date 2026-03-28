import { Search } from "lucide-react";

interface FilterOption {
  value: string;
  label: string;
}

interface DropdownFilter {
  value: string;
  onChange: (value: string) => void;
  options: FilterOption[];
  placeholder: string;
}

interface FilterBarProps {
  query: string;
  onQueryChange: (query: string) => void;
  placeholder?: string;
  dropdowns?: DropdownFilter[];
  right?: React.ReactNode;
}

export function FilterBar({
  query,
  onQueryChange,
  placeholder = "Filter...",
  dropdowns,
  right,
}: FilterBarProps) {
  return (
    <div className="flex items-center gap-2 mb-3">
      <div className="relative flex-1">
        <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-aide-text-dim pointer-events-none" />
        <input
          type="search"
          placeholder={placeholder}
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          className="w-full bg-aide-surface border border-aide-border rounded pl-7 pr-2 py-1.5 text-xs text-aide-text placeholder:text-aide-text-dim focus:border-aide-accent focus:ring-2 focus:ring-aide-accent/20 outline-none transition"
        />
      </div>
      {dropdowns?.map((dd, i) => (
        <select
          key={i}
          value={dd.value}
          onChange={(e) => dd.onChange(e.target.value)}
          className="bg-aide-surface border border-aide-border rounded px-2 py-1.5 text-xs text-aide-text focus:border-aide-accent outline-none"
        >
          <option value="">{dd.placeholder}</option>
          {dd.options.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      ))}
      {right}
    </div>
  );
}
