import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * Small labeled statistic used in a summary row.
 *
 * Mirrors the admin console's tile: a bordered icon square, a muted label, and
 * a semibold value. Colours come from the `--projects-*` tokens rather than the
 * admin hex values, because the user area ships a light theme and the admin
 * area does not.
 *
 * `value` is a pre-formatted string so the caller decides what an unknown value
 * looks like. Pass an em-free placeholder such as "—" while data is loading: a
 * zero would be a number the API never returned (R-17).
 */
export function StatTile({
  icon: Icon,
  label,
  value,
  hint,
  tone = "neutral",
  className,
}: {
  /** A glyph relevant to the metric it labels. */
  icon?: LucideIcon;
  label: string;
  value: string;
  hint?: string;
  tone?: "neutral" | "success" | "warning" | "danger";
  className?: string;
}) {
  const valueClass =
    tone === "danger"
      ? "text-[var(--projects-danger)]"
      : tone === "warning"
        ? "text-[var(--projects-warning)]"
        : tone === "success"
          ? "text-[var(--projects-accent)]"
          : "text-[var(--projects-text)]";

  return (
    <article
      className={cn(
        "flex items-center gap-3 rounded-lg border border-[var(--projects-border)] bg-[var(--projects-card-bg)] px-3.5 py-3",
        className,
      )}
    >
      {Icon ? (
        <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-[var(--projects-border)] bg-[var(--projects-control)] text-[var(--projects-muted)]">
          <Icon size={15} strokeWidth={1.8} aria-hidden="true" />
        </span>
      ) : null}
      <div className="min-w-0">
        <p className="m-0 truncate text-[11px] leading-4 text-[var(--projects-muted)]">
          {label}
        </p>
        <p
          className={cn(
            "m-0 truncate text-[17px] font-semibold leading-6 tracking-[-0.01em]",
            valueClass,
          )}
        >
          {value}
          {hint ? (
            <span className="ml-1.5 text-[11.5px] font-normal text-[var(--projects-muted)]">
              {hint}
            </span>
          ) : null}
        </p>
      </div>
    </article>
  );
}
