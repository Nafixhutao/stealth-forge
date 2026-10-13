import Link from "next/link";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { setupSteps, type SetupStep } from "../browser-setup-model";
import { BrandMark } from "./primitives";

/**
 * Full-screen chrome for the first-run wizard. Ported onto the redesigned
 * sidebar's visual language: the same warm near-black panel, hairline borders
 * and Geist type, with the step rail occupying the sidebar's slot. The setup
 * route renders outside the (auth) group so this shell owns the viewport.
 */
export function SetupShell({
  children,
  currentStep,
  rail,
}: {
  children: ReactNode;
  currentStep: SetupStep;
  rail?: ReactNode;
}) {
  const index = Math.max(
    0,
    setupSteps.findIndex((item) => item.id === currentStep),
  );
  const total = setupSteps.length;
  const progress = total > 1 ? (index / (total - 1)) * 100 : 0;
  const current = setupSteps[index];

  return (
    <div
      data-setup
      className="min-h-dvh bg-[var(--setup-bg)] text-[var(--setup-text)]"
    >
      <div className="mx-auto flex min-h-dvh w-full max-w-[1120px] flex-col px-4 sm:px-6 lg:px-10">
        <header className="flex h-[56px] shrink-0 items-center justify-between gap-4">
          <BrandMark />
          <div className="flex min-w-0 items-center gap-2 text-[13px]">
            <span className="hidden text-[var(--setup-text-muted)] sm:inline">
              First-run setup
            </span>
            <span
              aria-hidden="true"
              className="hidden size-1 rounded-full bg-[var(--setup-border-strong)] sm:block"
            />
            <span className="shrink-0 font-medium tabular-nums text-[var(--setup-text-muted)]">
              Step {index + 1} of {total}
            </span>
            <span
              aria-hidden="true"
              className="hidden size-1 rounded-full bg-[var(--setup-border-strong)] sm:block"
            />
            <span className="truncate font-medium text-[var(--setup-text)]">
              {current?.label ?? "Setup"}
            </span>
          </div>
        </header>

        <div
          role="progressbar"
          aria-valuemin={1}
          aria-valuemax={total}
          aria-valuenow={index + 1}
          aria-label="Setup progress"
          className="h-[2px] w-full shrink-0 overflow-hidden rounded-full bg-[var(--setup-border)]"
        >
          <div
            className="h-full rounded-full bg-[var(--setup-text-secondary)] transition-[width] duration-500 ease-out"
            style={{ width: `${progress}%` }}
          />
        </div>

        <div
          className={cn(
            "flex flex-1 flex-col gap-8 py-8 lg:flex-row lg:gap-12 lg:py-12",
            !rail && "items-center justify-center",
          )}
        >
          {rail}
          <main
            className={cn(
              "min-w-0 flex-1",
              !rail && "mx-auto w-full max-w-2xl",
            )}
          >
            <div
              key={currentStep}
              className="setup-stage max-w-[780px] rounded-xl border border-[var(--setup-border)] bg-[var(--setup-surface)] p-5 sm:p-7"
            >
              {children}
            </div>
          </main>
        </div>

        <footer className="flex shrink-0 flex-col gap-2 border-t border-[var(--setup-border)] py-4 text-[12px] text-[var(--setup-text-muted)] sm:flex-row sm:items-center sm:justify-between">
          <span>Setup state stays on the server.</span>
          <Link
            href="/login"
            className="text-[var(--setup-text-muted)] transition-colors duration-150 hover:text-[var(--setup-text)]"
          >
            Already have access? Sign in
          </Link>
        </footer>
      </div>
    </div>
  );
}
