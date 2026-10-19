"use client";

import type { ReactNode } from "react";
import { pluralize } from "@/lib/format";
import { cn } from "@/lib/utils";

/**
 * The list vocabulary the project sections share: the box, its column headings,
 * its cells, and the closing line that says how much of the list is on screen.
 *
 * Sections own their columns and cells; the chrome lives here so Databases,
 * Storage, Functions, and Sites read as one surface. The box follows the
 * organization home's table box, which is the same shape one level up.
 */

/** A section's heading: what it lists, and one line about it. */
export function ResourceSectionHeader({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 sm:px-0">
      <div className="flex min-w-0 flex-col">
        <h2 className="m-0 text-[15px] font-semibold tracking-[-0.01em] text-[var(--projects-text)] sm:text-[16px]">
          {title}
        </h2>
        <p className="m-0 text-[12.5px] leading-5 text-[var(--projects-muted)] [text-wrap:pretty]">
          {description}
        </p>
      </div>
      {action}
    </div>
  );
}

/** The box that holds a section's rows. */
export const RESOURCE_BOX =
  "mx-4 overflow-hidden rounded-xl border border-[var(--projects-border)] bg-[var(--projects-card-bg)] sm:mx-0";

/** A column heading inside the box. */
export const RESOURCE_HEAD =
  "px-4 py-2.5 text-left text-[12px] font-normal text-[var(--projects-muted)] sm:px-5 sm:py-3 sm:text-[12.5px]";

/** A body cell. */
export const RESOURCE_CELL = "px-4 py-3 align-middle sm:px-5";

/**
 * The closing line: what is on screen, and the way to the rest of it.
 *
 * It sits outside the table on purpose. As a `<tr>` its `colSpan` had to name a
 * column count, and sections hide their widest column below `sm`; a span wider
 * than the visible columns makes the browser invent an empty one, which silently
 * steals width from the name column at 360px.
 */
export function ResourceListFooter({
  shown,
  noun,
  hasMore,
  isLoadingMore,
  onLoadMore,
}: {
  shown: number;
  noun: string;
  hasMore: boolean;
  isLoadingMore: boolean;
  onLoadMore: () => void;
}) {
  return (
    <div className="border-t border-[var(--projects-divider)] px-4 py-2.5 text-[12.5px] leading-5 text-[var(--projects-muted)] sm:px-5 sm:py-3">
      <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span>{`Showing ${pluralize(shown, noun)}`}</span>
        {hasMore ? (
          <button
            type="button"
            onClick={onLoadMore}
            disabled={isLoadingMore}
            className="rounded-md font-medium text-[var(--projects-text)] underline decoration-[var(--projects-border-hover)] underline-offset-2 transition-colors hover:decoration-[var(--projects-text)] disabled:opacity-60"
          >
            {isLoadingMore ? "Loading…" : "Load more"}
          </button>
        ) : null}
      </span>
    </div>
  );
}

/** The list's loading footprint, so the box does not jump when rows land. */
export function ResourceListSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className={RESOURCE_BOX}>
      {Array.from({ length: rows }).map((_, index) => (
        <div
          key={index}
          className="flex items-center gap-3 border-b border-[var(--projects-divider)] px-4 py-3 last:border-b-0 sm:px-5"
        >
          <span className="min-w-0 flex-1 space-y-2">
            <span className="block h-3.5 w-40 max-w-full animate-pulse rounded bg-[var(--projects-control)] motion-reduce:animate-none" />
            <span className="block h-3 w-24 max-w-full animate-pulse rounded bg-[var(--projects-control)] motion-reduce:animate-none" />
          </span>
          <span className="hidden h-3 w-24 animate-pulse rounded bg-[var(--projects-control)] motion-reduce:animate-none sm:block" />
        </div>
      ))}
    </div>
  );
}

/** A row's identity cell: the name over one line of context. */
export function ResourceIdentity({
  name,
  meta,
  mark,
}: {
  name: string;
  /** The line under the name. A node rather than a string so a row can put a
   * state chip on it instead of spending a whole column on one short word. */
  meta: ReactNode;
  mark?: ReactNode;
}) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      {mark}
      <span className="flex min-w-0 flex-col">
        <span
          title={name}
          className="block truncate text-[14px] font-semibold leading-5 text-[var(--projects-text)] sm:text-[15px]"
        >
          {name}
        </span>
        <span className="flex min-w-0 flex-wrap items-center gap-x-1.5 text-[12.5px] leading-5 text-[var(--projects-muted)] sm:flex-nowrap sm:text-[13px]">
          {meta}
        </span>
      </span>
    </div>
  );
}

/** A quiet tag for a state the API reports, such as a site's build status. */
export function ResourceStatusChip({
  label,
  tone = "neutral",
}: {
  label: string;
  tone?: "neutral" | "positive" | "warning";
}) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center whitespace-nowrap rounded-full border px-2 py-0.5 text-[11.5px] font-medium leading-4",
        tone === "positive" &&
          "border-[var(--projects-accent-border)]/40 text-[var(--projects-accent)]",
        tone === "warning" &&
          "border-[var(--projects-warning)]/40 text-[var(--projects-warning)]",
        tone === "neutral" &&
          "border-[var(--projects-border)] text-[var(--projects-muted)]",
      )}
    >
      {label}
    </span>
  );
}
