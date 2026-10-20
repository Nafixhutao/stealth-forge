import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * The zero-data state for a surface that would otherwise show a list.
 *
 * It follows the admin console's empty state exactly: a bordered icon square, a
 * semibold title, and a muted sentence. That matters for R-27 as well as
 * consistency: the copy names the cause and the next step, so the reader can
 * tell first-run from filtered-to-nothing instead of reading "No data".
 *
 * Colours come from the `--projects-*` tokens rather than the admin hex values,
 * because the user area ships a light theme and the admin area does not.
 */
export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
}: {
  /** A glyph that is literally relevant to the missing content. */
  icon?: LucideIcon;
  title: string;
  description: string;
  /** A real control that fills the empty state. Omit rather than ship a dead one. */
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-center text-center", className)}>
      {Icon ? (
        <span className="flex size-10 items-center justify-center rounded-lg border border-[var(--projects-border)] bg-[var(--projects-control)] text-[var(--projects-muted)]">
          <Icon size={18} strokeWidth={1.8} aria-hidden="true" />
        </span>
      ) : null}
      <h3 className="m-0 mt-3 text-[15px] font-semibold tracking-[-0.01em] text-[var(--projects-text)] [text-wrap:balance]">
        {title}
      </h3>
      <p className="m-0 mt-1 max-w-[42ch] text-[13px] leading-5 text-[var(--projects-muted)] [text-wrap:pretty]">
        {description}
      </p>
      {action ? (
        <div className="mt-4 flex flex-wrap justify-center gap-2">{action}</div>
      ) : null}
    </div>
  );
}
