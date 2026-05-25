interface SessionFilterInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  widthClass?: string;
}

export function SessionFilterInput({
  value,
  onChange,
  placeholder = "session id",
  widthClass = "w-32",
}: SessionFilterInputProps) {
  return (
    <input
      type="text"
      placeholder={placeholder}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={
        "bg-aide-surface border border-aide-border rounded px-2 py-1.5 text-xs text-aide-text placeholder:text-aide-text-dim focus:border-aide-accent outline-none " +
        widthClass
      }
    />
  );
}
