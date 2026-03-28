import { useEffect, useRef, useCallback } from "react";
import { cn } from "@/lib/utils";
import { X } from "lucide-react";

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
  className?: string;
}

export function Modal({
  open,
  onClose,
  title,
  children,
  footer,
  className,
}: ModalProps) {
  const overlayRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      ref={overlayRef}
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={(e) => e.target === overlayRef.current && onClose()}
    >
      <div
        className={cn(
          "bg-aide-surface border border-aide-border rounded-lg shadow-2xl w-full max-w-lg mx-4 max-h-[85vh] flex flex-col",
          className
        )}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-aide-border shrink-0">
          <h3 className="text-sm font-semibold text-aide-text">{title}</h3>
          <button
            onClick={onClose}
            className="p-1 rounded-sm text-aide-text-dim hover:text-aide-text hover:bg-aide-surface-hover transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Body */}
        <div className="px-4 py-3 overflow-y-auto flex-1">{children}</div>

        {/* Footer */}
        {footer && (
          <div className="px-4 py-3 border-t border-aide-border flex items-center justify-end gap-2 shrink-0">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}

interface ConfirmDialogProps {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  message: string;
  confirmLabel?: string;
  loading?: boolean;
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  message,
  confirmLabel = "Delete",
  loading = false,
}: ConfirmDialogProps) {
  const handleKey = useCallback(
    (e: KeyboardEvent) => {
      if (!open || loading) return;
      if (e.key === "c" || e.key === "C") {
        e.preventDefault();
        onClose();
      } else if (e.key === "d" || e.key === "D") {
        e.preventDefault();
        onConfirm();
      }
    },
    [open, loading, onClose, onConfirm]
  );

  useEffect(() => {
    if (!open) return;
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [open, handleKey]);

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      className="max-w-sm"
      footer={
        <>
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-xs font-medium text-aide-text-muted border border-aide-border rounded-sm hover:bg-aide-surface-hover transition-colors"
          >
            <kbd className="text-[0.6rem] text-aide-text-dim bg-aide-bg border border-aide-border rounded px-1 py-0.5 mr-1.5">C</kbd>
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={loading}
            className="px-3 py-1.5 text-xs font-medium text-aide-red border border-aide-red/30 rounded-sm hover:bg-aide-red/10 transition-colors disabled:opacity-50"
          >
            <kbd className="text-[0.6rem] text-aide-red/60 bg-aide-bg border border-aide-red/20 rounded px-1 py-0.5 mr-1.5">D</kbd>
            {loading ? "..." : confirmLabel}
          </button>
        </>
      }
    >
      <p className="text-xs text-aide-text-muted">{message}</p>
    </Modal>
  );
}

/* Shared form field styling */
export function FormField({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="mb-3">
      <label className="block text-xs font-semibold uppercase tracking-wide text-aide-text-dim mb-1">
        {label}
      </label>
      {children}
    </div>
  );
}

export const inputClass =
  "w-full bg-aide-bg border border-aide-border rounded px-2.5 py-1.5 text-xs text-aide-text placeholder:text-aide-text-dim focus:border-aide-accent focus:ring-2 focus:ring-aide-accent/20 outline-none transition";

export const textareaClass = cn(inputClass, "resize-y min-h-[80px]");

export function ModalFooterButtons({
  onClose,
  onSubmit,
  submitLabel = "Save",
  loading = false,
}: {
  onClose: () => void;
  onSubmit: () => void;
  submitLabel?: string;
  loading?: boolean;
}) {
  return (
    <>
      <button
        onClick={onClose}
        className="px-3 py-1.5 text-xs font-medium text-aide-text-muted border border-aide-border rounded-sm hover:bg-aide-surface-hover transition-colors"
      >
        Cancel
      </button>
      <button
        onClick={onSubmit}
        disabled={loading}
        className="px-3 py-1.5 text-xs font-medium text-aide-accent border border-aide-accent/30 rounded-sm hover:bg-aide-accent/10 transition-colors disabled:opacity-50"
      >
        {loading ? "..." : submitLabel}
      </button>
    </>
  );
}
