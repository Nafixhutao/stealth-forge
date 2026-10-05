"use client";

import { RefreshCw } from "lucide-react";
import { useElapsedSeconds } from "../hooks/use-live-updates";

/** Pulsing "Live" marker — pure CSS pulse so it never re-renders the page. */
export function LiveIndicator({ label = "Live" }: { label?: string }) {
  return (
    <span className="inline-flex h-9 items-center gap-2 rounded-lg border border-[var(--projects-border)] bg-[#141416] px-3 text-[12.5px] font-medium text-[var(--projects-accent)]">
      <span className="relative flex size-2">
        <span className="absolute inline-flex size-full animate-ping rounded-full bg-[var(--projects-accent)] opacity-50" />
        <span className="relative inline-flex size-2 rounded-full bg-[var(--projects-accent)]" />
      </span>
      {label}
    </span>
  );
}

/**
 * "Last updated Xs ago" — owns its 1s ticker internally so the parent page
 * (and its charts) do not re-render every second. Reset alongside refreshes.
 */
export function UpdatedLabel({ resetKey, className }: { resetKey?: unknown; className?: string }) {
  const { seconds } = useElapsedSeconds(resetKey);
  const label = seconds < 3 ? "just now" : `${seconds}s ago`;
  return (
    <span className={className} aria-live="off">
      Last updated {label}
    </span>
  );
}

/** Square icon button for manual refresh. */
export function RefreshButton({ onClick, label = "Refresh data" }: { onClick: () => void; label?: string }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      title={label}
      className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg border border-[var(--projects-border)] bg-[#141416] text-[var(--projects-muted)] transition-colors hover:border-[var(--projects-border-hover)] hover:text-[var(--projects-text)]"
    >
      <RefreshCw size={14} strokeWidth={1.8} aria-hidden="true" />
    </button>
  );
}
