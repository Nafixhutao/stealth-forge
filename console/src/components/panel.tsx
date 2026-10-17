"use client";

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * Panel primitives for the signed-in console.
 *
 * They mirror the geometry and type system of the admin console panels so both
 * surfaces read as one product: an 8px-radius surface, an uppercase micro
 * heading, and a 1440px page measure.
 *
 * The one deliberate difference is colour. The admin panels hardcode the dark
 * surface `#141416`, and the admin area is dark-only. The user area supports the
 * light theme, so these use the `--projects-*` tokens instead, which resolve per
 * theme. Copying the admin hex values here would leave light mode with dark
 * panels.
 */

/** Bordered content surface — the base of every section. */
export function Panel({
  children,
  className,
  flush = false,
  labelledBy,
}: {
  children: ReactNode;
  className?: string;
  /** Remove the default inner padding (full-bleed lists). */
  flush?: boolean;
  /** id of the panel's heading, so the section is announced by its own name. */
  labelledBy?: string;
}) {
  return (
    <section
      aria-labelledby={labelledBy}
      className={cn(
        "overflow-hidden rounded-lg border border-[var(--projects-border)] bg-[var(--projects-card-bg)]",
        !flush && "p-4",
        className,
      )}
    >
      {children}
    </section>
  );
}

/** Section heading row: uppercase micro title, optional subtitle and right slot. */
export function PanelHeader({
  title,
  subtitle,
  right,
  className,
  id,
}: {
  title: string;
  subtitle?: string;
  right?: ReactNode;
  className?: string;
  /** Set when a parent uses `aria-labelledby` to name its region. */
  id?: string;
}) {
  return (
    <header
      className={cn(
        "flex flex-wrap items-center gap-x-3 gap-y-1.5",
        subtitle ? "mb-3" : "mb-2",
        className,
      )}
    >
      <h2
        id={id}
        className="m-0 text-[12px] font-semibold uppercase tracking-[0.08em] text-[var(--projects-muted)]"
      >
        {title}
      </h2>
      {right ? (
        <div className="ml-auto flex items-center gap-2">{right}</div>
      ) : null}
      {subtitle ? (
        // Full-strength muted, not an opacity step: at 80% the muted token drops
        // to 3.4:1 on the light theme, under the 4.5:1 WCAG AA floor (R-25).
        <p className="m-0 w-full text-[12px] leading-4 font-normal normal-case tracking-normal text-[var(--projects-muted)]">
          {subtitle}
        </p>
      ) : null}
    </header>
  );
}

/** Page header block: title, subtitle, and a right-hand control slot. */
export function PageHeader({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children?: ReactNode;
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-x-4 gap-y-3 border-b border-[var(--projects-border)] pb-4">
      <div className="min-w-0">
        <h1 className="m-0 text-[22px] font-semibold leading-7 tracking-[-0.03em] text-[var(--projects-text)]">
          {title}
        </h1>
        <p className="m-0 mt-1 text-[13px] leading-5 text-[var(--projects-muted)]">
          {subtitle}
        </p>
      </div>
      {children ? (
        <div className="flex w-full flex-wrap items-center gap-2 lg:w-auto">
          {children}
        </div>
      ) : null}
    </header>
  );
}

/** Page content wrapper: shared gutters and measure. */
export function PageBody({ children }: { children: ReactNode }) {
  return (
    // The user console's mobile top bar is a fixed 59px strip, so content
    // needs 72px of clearance until the desktop rail replaces it at `lg`; from
    // there the page returns to its normal top padding.
    <div className="mx-auto flex w-full max-w-full flex-col gap-[13px] pb-12 pt-[72px] sm:max-w-[1440px] sm:gap-4 sm:px-6 lg:px-7 lg:pt-5">
      {children}
    </div>
  );
}

/**
 * A read-only counter or label, styled as admin does: a quiet bordered chip.
 * Used for values that are facts about the data on screen, never for decoration.
 *
 * Height is 44px on small screens and 36px from `sm` up, so the chip matches the
 * admin control height on desktop while still meeting the minimum tap target
 * where the pointer is a finger.
 */
export function MetaChip({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex min-h-11 items-center rounded-lg border border-[var(--projects-border)] bg-[var(--projects-card-bg)] px-3 text-[12px] text-[var(--projects-muted)] sm:min-h-9",
        className,
      )}
    >
      {children}
    </span>
  );
}

/**
 * Technical values: slugs, ids, timestamps. Reuses the `admin-mono` class so
 * both surfaces share one monospace stack instead of declaring a second one.
 */
export function Mono({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <span className={cn("admin-mono", className)}>{children}</span>;
}
