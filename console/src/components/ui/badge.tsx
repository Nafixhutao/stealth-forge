import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";
import {
  AnimatedBadge,
  type AnimatedBadgeStatus,
} from "@/components/motion/animated-badge";

const badgeVariants = cva(
  "inline-flex items-center gap-1.5 whitespace-nowrap rounded-xs border px-1.5 py-0.5 text-xs font-medium tracking-[-0.01em]",
  {
    variants: {
      variant: {
        default: "border-cyan-300/25 bg-cyan-300/10 text-cyan-200",
        neutral: "border-white/10 bg-white/[0.06] text-slate-300",
        success: "border-emerald-300/25 bg-emerald-300/10 text-emerald-200",
        warning: "border-amber-300/25 bg-amber-300/10 text-amber-200",
        error: "border-rose-300/25 bg-rose-300/10 text-rose-200",
        building: "border-violet-300/25 bg-violet-300/10 text-violet-200",
      },
    },
    defaultVariants: { variant: "neutral" },
  },
);

export function Badge({
  className,
  variant,
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & VariantProps<typeof badgeVariants>) {
  return (
    <span className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

const UNDERSCORE = /_/g;
const FIRST_CHAR = /^\w/;

/** Console status chip, rendered by the shared AnimatedBadge so every status
    in the app animates and sizes identically. The status vocabulary is mapped
    onto the badge's semantic tones. */
export function StatusBadge({ status }: { status: string | null | undefined }) {
  const normalized = status?.toLowerCase() ?? "unknown";
  const tone = getStatusTone(normalized);
  const label = status
    ? status
        .replace(UNDERSCORE, " ")
        .replace(FIRST_CHAR, (character) => character.toUpperCase())
    : "Unknown";
  return (
    <AnimatedBadge status={tone} size="sm" contentKey={label}>
      {label}
    </AnimatedBadge>
  );
}

function getStatusTone(status: string): AnimatedBadgeStatus {
  switch (status) {
    case "ready":
    case "active":
    case "available":
    case "succeeded":
    case "delivered":
    case "healthy":
    case "completed":
    case "converged":
      return "success";
    case "failed":
    case "error":
    case "blocked":
      return "danger";
    case "building":
    case "running":
    case "delivering":
    case "pending":
    case "queued":
    case "processing":
    case "accepted":
    case "deferred":
    case "reconciling":
    case "waiting_for_runtime":
    case "waiting_for_health":
      return "loading";
    case "warning":
    case "past_due":
    case "degraded":
    case "cancelled":
    case "canceled":
    case "unhealthy":
    case "unavailable":
      return "warning";
    default:
      return "neutral";
  }
}

export function HttpStatusBadge({
  status,
}: {
  status: number | null | undefined;
}) {
  const variant =
    status === null || status === undefined
      ? "neutral"
      : status >= 500
        ? "error"
        : status >= 400
          ? "warning"
          : status >= 300
            ? "default"
            : status >= 200
              ? "success"
              : "neutral";

  return (
    <Badge
      variant={variant}
      title={
        status !== null && status !== undefined
          ? `HTTP status ${status}`
          : undefined
      }
    >
      <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
      {status ?? "Unknown"}
    </Badge>
  );
}
