import { useState } from "react";
import { cn } from "@/lib/utils";
import { X, Plus } from "lucide-react";

interface TagInputProps {
  tags: string[];
  onChange: (tags: string[]) => void;
  suggestions?: string[];
  autoTags?: string[];
  placeholder?: string;
}

export function TagInput({
  tags,
  onChange,
  suggestions = [],
  autoTags = [],
  placeholder = "Add tag...",
}: TagInputProps) {
  const [input, setInput] = useState("");

  function addTag(tag: string) {
    const t = tag.trim();
    if (t && !tags.includes(t) && !autoTags.includes(t)) {
      onChange([...tags, t]);
    }
    setInput("");
  }

  function removeTag(tag: string) {
    onChange(tags.filter((t) => t !== tag));
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addTag(input);
    }
    if (e.key === "Backspace" && !input && tags.length > 0) {
      onChange(tags.slice(0, -1));
    }
  }

  const availableSuggestions = suggestions.filter(
    (s) => !tags.includes(s) && !autoTags.includes(s)
  );

  return (
    <div>
      {/* Current tags */}
      <div className="flex flex-wrap gap-1 mb-1.5">
        {autoTags.map((tag) => (
          <span
            key={tag}
            className="inline-flex items-center gap-1 text-[0.65rem] px-1.5 py-0.5 rounded bg-aide-accent/10 text-aide-accent/60 border border-aide-accent/15"
            title="Auto-added"
          >
            {tag}
          </span>
        ))}
        {tags.map((tag) => (
          <span
            key={tag}
            className="inline-flex items-center gap-1 text-[0.65rem] px-1.5 py-0.5 rounded bg-white/5 text-aide-text-muted border border-white/10"
          >
            {tag}
            <button
              onClick={() => removeTag(tag)}
              className="hover:text-aide-red transition-colors"
            >
              <X className="w-2.5 h-2.5" />
            </button>
          </span>
        ))}
      </div>

      {/* Input */}
      <input
        type="text"
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={() => input.trim() && addTag(input)}
        placeholder={placeholder}
        className="w-full bg-aide-bg border border-aide-border rounded px-2.5 py-1.5 text-xs text-aide-text placeholder:text-aide-text-dim focus:border-aide-accent focus:ring-2 focus:ring-aide-accent/20 outline-none transition"
      />

      {/* Suggestions */}
      {availableSuggestions.length > 0 && (
        <div className="flex flex-wrap gap-1 mt-1.5">
          {availableSuggestions.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => addTag(s)}
              className={cn(
                "inline-flex items-center gap-0.5 text-[0.6rem] px-1.5 py-0.5 rounded",
                "border border-dashed border-aide-border text-aide-text-dim",
                "hover:border-aide-accent/40 hover:text-aide-accent hover:bg-aide-accent/5 transition-colors"
              )}
            >
              <Plus className="w-2.5 h-2.5" />
              {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
