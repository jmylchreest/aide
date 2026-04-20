import { useState, useEffect, useRef } from "react";
import { cn } from "@/lib/utils";
import { Modal } from "./Modal";
import { api } from "@/lib/api";
import { Badge } from "./ExpandableCard";
import { Copy, Check } from "lucide-react";

interface CodeViewerProps {
  open: boolean;
  onClose: () => void;
  project: string;
  filePath: string;
  /** 1-based first line to highlight. */
  line?: number;
  /** 1-based last line to highlight; defaults to `line` when omitted (single-line). */
  endLine?: number;
  title?: string;
}

export function CodeViewer({
  open,
  onClose,
  project,
  filePath,
  line,
  endLine,
  title,
}: CodeViewerProps) {
  const [content, setContent] = useState<string | null>(null);
  const [language, setLanguage] = useState("text");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const targetRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open || !filePath) return;
    setLoading(true);
    setError(null);
    setContent(null);
    api
      .readFile(project, filePath)
      .then((r) => {
        setContent(r.content);
        setLanguage(r.language);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [open, project, filePath]);

  // Scroll to target line after content loads
  useEffect(() => {
    if (content && line && targetRef.current) {
      // Small delay to let the DOM render
      requestAnimationFrame(() => {
        targetRef.current?.scrollIntoView({ block: "center" });
      });
    }
  }, [content, line]);

  const lines = content?.split("\n") ?? [];

  function handleCopyPath() {
    navigator.clipboard.writeText(filePath);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title || filePath}
      className="max-w-5xl max-h-[90vh]"
      footer={
        <div className="flex items-center justify-between w-full text-xs text-aide-text-dim">
          <div className="flex items-center gap-2">
            <Badge label={language} variant="accent" />
            <span className="tabular-nums">{lines.length} lines</span>
            {line && (
              <span>
                &middot; line{" "}
                <span className="text-aide-accent">
                  {endLine && endLine > line ? `${line}–${endLine}` : line}
                </span>
              </span>
            )}
          </div>
          <button
            onClick={handleCopyPath}
            className="flex items-center gap-1 px-2 py-1 rounded-sm text-aide-text-dim hover:text-aide-text hover:bg-aide-surface-hover transition-colors"
          >
            {copied ? (
              <Check className="w-3 h-3 text-aide-green" />
            ) : (
              <Copy className="w-3 h-3" />
            )}
            {filePath}
          </button>
        </div>
      }
    >
      {loading && (
        <p className="text-aide-text-dim text-xs py-8 text-center">
          Loading file...
        </p>
      )}
      {error && (
        <p className="text-aide-red text-xs py-8 text-center">{error}</p>
      )}
      {content !== null && (
        <div className="overflow-auto max-h-[calc(90vh-10rem)] bg-aide-bg rounded border border-aide-border">
          <pre className="text-[0.7rem] leading-5 p-0 m-0">
            <code>
              {lines.map((lineContent, i) => {
                const lineNum = i + 1;
                const rangeEnd = endLine && line ? endLine : line;
                const inRange =
                  line !== undefined &&
                  lineNum >= line &&
                  lineNum <= (rangeEnd ?? line);
                const isFirstInRange = lineNum === line;
                return (
                  <div
                    key={lineNum}
                    ref={isFirstInRange ? targetRef : undefined}
                    className={cn(
                      "flex hover:bg-aide-surface-hover/50",
                      inRange && "bg-aide-accent/10 border-l-2 border-aide-accent"
                    )}
                  >
                    <span
                      className={cn(
                        "select-none text-right pr-3 pl-2 min-w-[3.5rem] shrink-0",
                        inRange
                          ? "text-aide-accent font-medium"
                          : "text-aide-text-dim/40"
                      )}
                    >
                      {lineNum}
                    </span>
                    <span className="flex-1 pr-4 whitespace-pre text-aide-text-muted">
                      {lineContent}
                    </span>
                  </div>
                );
              })}
            </code>
          </pre>
        </div>
      )}
    </Modal>
  );
}
