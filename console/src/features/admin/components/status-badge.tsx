import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export type AdminStatusTone = "success" | "warning" | "danger" | "info" | "neutral";

/** Semantic tokens for the four allowed status colors (+ neutral). */
export const TONE_TOKENS: Record<AdminStatusTone, { dot: string; text: string; label: string }> = {
  success: { dot: "bg-[var(--projects-accent)]", text: "text-[var(--projects-accent)]", label: "text-[oklch(0.76_0.12_162)]" },
  warning: { dot: "bg-[var(--projects-warning)]", text: "text-[var(--projects-warning)]", label: "text-[oklch(0.8_0.13_75)]" },
  danger: { dot: "bg-[var(--projects-danger)]", text: "text-[var(--projects-danger)]", label: "text-[var(--projects-danger)]" },
  info: { dot: "bg-[var(--admin-info)]", text: "text-[var(--admin-info)]", label: "text-[var(--admin-info)]" },
  neutral: { dot: "bg-[var(--projects-ring)]", text: "text-[var(--projects-muted)]", label: "text-[var(--projects-muted)]" },
};

/**
 * Status indicator: dot + visible text label (color is never the only
 * signal). `pulse` marks live/attention states, mirroring AgentStatusDot.
 */
export function StatusBadge({
  tone,
  label,
  pulse = false,
  className,
}: {
  tone: AdminStatusTone;
  label: string;
  pulse?: boolean;
  className?: string;
}) {
  const token = TONE_TOKENS[tone];
  return (
    <span className={cn("inline-flex items-center gap-1.5 whitespace-nowrap", token.label, className)}>
      <span className="relative flex size-2 shrink-0">
        {pulse && (
          <span className={cn("absolute inline-flex size-full animate-ping rounded-full opacity-50", token.dot)} />
        )}
        <span className={cn("relative inline-flex size-2 rounded-full", token.dot)} />
      </span>
      {label}
    </span>
  );
}

/** Muted chip for counts/roles, matching the agent list role pill. */
export function CountPill({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex h-[20px] items-center rounded-md border border-[var(--projects-border)] px-1.5 text-[11px] leading-none text-[var(--projects-muted)]",
        className,
      )}
    >
      {children}
    </span>
  );
}
