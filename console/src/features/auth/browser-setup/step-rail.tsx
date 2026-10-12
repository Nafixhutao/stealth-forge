"use client";

import { motion, useReducedMotion } from "motion/react";
import { Check } from "lucide-react";
import { SPRING_LAYOUT } from "@/lib/ease";
import { cn } from "@/lib/utils";
import type { SetupStep } from "../browser-setup-model";

// Adapted from beUI's "Shared Layout Background" (beui.dev/components/motion/
// shared-layout-bg): one pill that travels between rows through motion's shared
// `layoutId` instead of a background painted on each row. Reason (R-31): the rail
// is the wizard's navigational spine, and a single travelling pill reads as "one
// progress track" where a per-row background reads as unrelated highlights. Hover
// stays a thin wash (as in the Admin Console sidebar) so the pill never claims a
// step the wizard has not reached.
//
// Markers are circles numbered from their index (1..9, no padded zeros), so the
// rail, the header's "Step X of Y" and the step order can never disagree. One
// continuous spine runs behind them with a lit overlay up to the active step, so
// progress reads off the line instead of off a second progress bar.
export function StepRail({
  steps,
  activeStep,
  canOpen,
  onSelect,
}: {
  steps: Array<{ id: SetupStep; label: string }>;
  activeStep: SetupStep;
  canOpen: (step: SetupStep) => boolean;
  onSelect: (step: SetupStep) => void;
}) {
  const reduce = useReducedMotion() ?? false;
  // Clamped like SetupShell's lookup: an unknown activeStep must not push the
  // progress overlay above the track or leave every marker unlit.
  const currentIndex = Math.max(
    0,
    steps.findIndex((item) => item.id === activeStep),
  );
  const progress = steps.length > 1 ? currentIndex / (steps.length - 1) : 0;

  return (
    <nav
      aria-label="Setup steps"
      className="min-w-0 lg:sticky lg:top-12 lg:w-[236px] lg:shrink-0 lg:self-start"
    >
      <p className="hidden px-2.5 pb-3 font-mono text-[11px] font-medium uppercase tracking-[0.12em] text-[var(--setup-text-muted)] lg:block">
        Setup progress
      </p>
      {/* The spine is drawn by this wrapper rather than the list itself: an
          `ol` may only contain `li` children, so the track lives one level up
          and keeps the same box as the list. */}
      <div className="relative">
        {/* One spine behind the markers, with a lit overlay up to the active
            step. The markers carry an opaque fill, so they mask the line where
            they sit. */}
        <span
          aria-hidden="true"
          className="pointer-events-none absolute bottom-[18px] left-5 top-[18px] hidden w-px bg-[var(--setup-border)] lg:block"
        />
        <span
          aria-hidden="true"
          className="pointer-events-none absolute left-5 top-[18px] hidden w-px bg-[var(--setup-text-muted)]/45 transition-[height] duration-500 ease-out lg:block"
          style={{ height: `calc((100% - 36px) * ${progress})` }}
        />
        <ol className="setup-scrollbar flex gap-1.5 overflow-x-auto pb-2 lg:block lg:overflow-visible lg:pb-0">
          {steps.map((item, index) => {
            const active = item.id === activeStep;
            const done = index < currentIndex;
            const available = canOpen(item.id) || active;
            return (
              <li key={item.id} className="relative shrink-0 lg:h-9">
                <button
                  type="button"
                  disabled={!available}
                  aria-current={active ? "step" : undefined}
                  onClick={() => available && onSelect(item.id)}
                  className={cn(
                    "relative isolate flex h-11 w-full items-center gap-3 rounded-md px-2.5 text-left text-[13px] transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/15 lg:h-9",
                    active
                      ? "text-[var(--setup-text)]"
                      : available
                        ? "text-[var(--setup-text-secondary)] hover:bg-[var(--setup-row-hover)] hover:text-[var(--setup-text)]"
                        : "cursor-not-allowed text-[var(--setup-text-faint)]/60",
                  )}
                >
                  {/* The travelling pill. It only ever marks the active step, so
                    the rail cannot suggest progress the wizard has not made. */}
                  {active ? (
                    <motion.span
                      layoutId="setup-step-pill"
                      transition={reduce ? { duration: 0 } : SPRING_LAYOUT}
                      className="pointer-events-none absolute inset-0 -z-10 rounded-md bg-[var(--setup-row-active)]"
                    />
                  ) : null}
                  <span
                    className={cn(
                      "flex size-5 shrink-0 items-center justify-center rounded-full border font-mono text-[10px] tabular-nums leading-none",
                      active
                        ? "border-transparent bg-[var(--setup-text)] font-medium text-[var(--setup-bg)]"
                        : done
                          ? "border-[var(--setup-border-strong)] bg-[var(--setup-bg)] text-[var(--setup-text)]"
                          : "border-[var(--setup-border)] bg-[var(--setup-bg)] text-[var(--setup-text-muted)]",
                    )}
                  >
                    {done ? (
                      <Check className="size-3" strokeWidth={2.75} />
                    ) : (
                      index + 1
                    )}
                  </span>
                  <span className="truncate">{item.label}</span>
                </button>
              </li>
            );
          })}
        </ol>
      </div>
    </nav>
  );
}
