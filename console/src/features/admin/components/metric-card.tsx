import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import type { AdminStatusTone } from "./status-badge";
import { Sparkline, type SparkTone } from "./sparkline";

/**
 * Compact metric card: icon, label, current value, delta, mini sparkline.
 * Kept intentionally small — a full row of these sits above the charts.
 */
export function MetricCard({
  icon: Icon,
  label,
  value,
  hint,
  change,
  changeLabel,
  changeTone = "neutral",
  history,
  sparkTone = "neutral",
}: {
  icon: LucideIcon;
  label: string;
  value: string;
  hint?: string;
  change?: string;
  changeLabel?: string;
  changeTone?: AdminStatusTone;
  history: number[];
  sparkTone?: SparkTone;
}) {
  const changeClass =
    changeTone === "danger"
      ? "text-[var(--projects-danger)]"
      : changeTone === "success"
        ? "text-[var(--projects-accent)]"
        : "text-[var(--projects-muted)]";

  return (
    <article className="flex flex-col gap-2.5 rounded-lg border border-[var(--projects-border)] bg-[#141416] p-3.5">
      <header className="flex items-center gap-2">
        <span className="flex size-6 shrink-0 items-center justify-center rounded-md border border-[var(--projects-border)] bg-[var(--projects-control)] text-[var(--projects-muted)]">
          <Icon size={13} strokeWidth={1.8} aria-hidden="true" />
        </span>
        <h3 className="m-0 truncate text-[11.5px] font-medium leading-4 text-[var(--projects-muted)]">{label}</h3>
        {hint && <span className="admin-mono ml-auto shrink-0 text-[11px] leading-4 text-[var(--projects-muted)]/80">{hint}</span>}
      </header>

      <div className="flex items-end justify-between gap-2">
        <p className="m-0 text-[22px] font-semibold leading-6 tracking-[-0.02em] text-[var(--projects-text)]">{value}</p>
        {change && (
          <p className={cn("m-0 text-right text-[11px] leading-4", changeClass)}>
            {change}
            {changeLabel && <span className="block text-[10.5px] text-[var(--projects-muted)]/70">{changeLabel}</span>}
          </p>
        )}
      </div>

      <Sparkline data={history} tone={sparkTone} height={26} />
    </article>
  );
}
