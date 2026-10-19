"use client";

import Link from "next/link";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * One fact about a project on the Overview: a labelled figure, an optional
 * second line that gives it context, and an optional link into the section the
 * figure describes.
 *
 * A tile links only when it has somewhere real to go — an unbuilt section gets
 * a plain tile instead of a link to a 404.
 *
 * Distinct from the shared `StatTile`, which lays a figure out beside its icon
 * in one row for a summary strip: this one stacks a value over its context in a
 * grid cell and can carry the whole tile as a link.
 */

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--projects-bg)]";

const SHELL =
  "flex flex-col gap-1 rounded-[13px] border border-[var(--projects-border)] bg-[var(--projects-card-bg)] px-4 py-3.5";

export function OverviewTile({
  label,
  value,
  detail,
  href,
  icon: Icon,
  loading = false,
}: {
  label: string;
  value: string;
  detail?: string;
  href?: string;
  icon?: LucideIcon;
  loading?: boolean;
}) {
  const body = (
    <>
      <span className="flex items-center gap-1.5 text-[12px] leading-5 text-[var(--projects-muted)]">
        {Icon ? <Icon size={13} strokeWidth={1.8} aria-hidden="true" /> : null}
        {label}
      </span>
      {loading ? (
        <>
          <span className="h-5 w-16 animate-pulse rounded bg-[var(--projects-chip)] motion-reduce:animate-none" />
          <span className="h-3.5 w-28 max-w-full animate-pulse rounded bg-[var(--projects-control)] motion-reduce:animate-none" />
        </>
      ) : (
        <>
          <span className="text-[18px] font-semibold leading-6 tracking-[-0.01em] text-[var(--projects-text)]">
            {value}
          </span>
          {detail ? (
            <span className="text-[12px] leading-5 text-[var(--projects-muted)] [text-wrap:pretty]">
              {detail}
            </span>
          ) : null}
        </>
      )}
    </>
  );

  if (href && !loading) {
    return (
      <Link
        href={href}
        className={cn(
          SHELL,
          "transition-colors hover:border-[var(--projects-border-hover)]",
          FOCUS,
        )}
      >
        {body}
      </Link>
    );
  }

  return <div className={SHELL}>{body}</div>;
}
