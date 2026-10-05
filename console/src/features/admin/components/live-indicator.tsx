"use client";

import { RefreshCw } from "lucide-react";
import { useElapsedSeconds } from "../hooks/use-live-updates";

/** Pulsing "Live" marker — pure CSS pulse so it never re-renders the page.
 * `live` reflects real query connectivity: green while data is flowing, amber
 * while a query is failing, so the badge never lies about the connection. */
export function LiveIndicator({
  label,
  live = true,
}: {
  label?: string;
  live?: boolean;
}) {
  const text = label ?? (live ? "Live" : "Reconnecting");
  const color = live
    ? "text-[var(--projects-accent)]"
    : "text-[var(--projects-warning)]";
  const dot = live
    ? "bg-[var(--projects-accent)]"
    : "bg-[var(--projects-warning)]";
  return (
    <span
      className={`inline-flex h-9 items-center gap-2 rounded-lg border border-[var(--projects-border)] bg-[#141416] px-3 text-[12.5px] font-medium ${color}`}
      role="status"
      aria-live="polite"
    >
      <span className="relative flex size-2">
        {live && (
          <span className="absolute inline-flex size-full animate-ping rounded-full bg-[var(--projects-accent)] opacity-50" />
        )}
        <span className={`relative inline-flex size-2 rounded-full ${dot}`} />
      </span>
      {text}
    </span>
  );
}

/**
 * "Last updated Xs ago" — owns its 1s ticker internally so the parent page
 * (and its charts) do not re-render every second. Reset alongside refreshes.
 */
export function UpdatedLabel({
  resetKey,
  className,
}: {
  resetKey?: unknown;
  className?: string;
}) {
  const { seconds } = useElapsedSeconds(resetKey);
  const label = seconds < 3 ? "just now" : `${seconds}s ago`;
  return (
    <span className={className} aria-live="off">
      Last updated {label}
    </span>
  );
}

/** Square icon button for manual refresh. */
export function RefreshButton({
  onClick,
  label = "Refresh data",
}: {
  onClick: () => void;
  label?: string;
}) {
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
