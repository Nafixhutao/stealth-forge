"use client";

import { Check, CircleAlert, Loader2, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  installationStepState,
  installationSteps,
} from "../../browser-setup-model";
import { StageHeader, StatusPill } from "../primitives";
import type { SetupFlow } from "../types";

export function InstallStage({ flow }: { flow: SetupFlow }) {
  const { installViewState, isPending, retryProductionInstall, sseConnected } =
    flow;
  const phase = installViewState?.phase;
  const step = installViewState?.step;
  const errorMessage = installViewState?.error_message;
  const completed = installationSteps.filter(
    (label) => installationStepState(phase, step, label) === "complete",
  ).length;

  return (
    <>
      <StageHeader
        eyebrow="08 / Install"
        title={
          phase === "failed"
            ? "Installation needs attention"
            : "Installing Stealth"
        }
        description={
          phase === "failed"
            ? "The production stack is still repairable. Review the safe error below and retry after correcting the underlying issue."
            : phase === "install_requested"
              ? "Your configuration was received. The host installer is preparing the installation."
              : "The host installer is running the shared install engine. Keep this window open until the production session handoff completes."
        }
      />
      <div className="rounded-xl border border-[var(--setup-border)]">
        <div className="flex items-center justify-between gap-4 border-b border-[var(--setup-border)] px-4 py-3.5">
          <div className="min-w-0">
            <p className="truncate text-[13px] font-medium text-[var(--setup-text)]">
              {step ?? "Preparing installation"}
            </p>
            <p className="mt-0.5 text-[12px] text-[var(--setup-text-muted)]">
              {sseConnected
                ? "Live progress connected"
                : "Reconnecting to live progress"}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-3">
            <span className="hidden font-mono text-[11px] tabular-nums text-[var(--setup-text-muted)] sm:inline">
              {completed}/{installationSteps.length}
            </span>
            {phase === "failed" ? (
              <StatusPill status="error">Failed</StatusPill>
            ) : (
              <StatusPill status="pending">Working</StatusPill>
            )}
          </div>
        </div>
        <ol className="divide-y divide-[var(--setup-border)]">
          {installationSteps.map((label) => {
            const status = installationStepState(phase, step, label);
            const done = status === "complete";
            const current = status === "current";
            const failed = status === "failed";
            return (
              <li
                key={label}
                aria-current={current ? "step" : undefined}
                className="flex items-center gap-3 px-4 py-3 text-[13px]"
              >
                <span
                  className={cn(
                    "flex size-5 shrink-0 items-center justify-center rounded-full border",
                    done || current
                      ? "border-[var(--setup-border-strong)] text-[var(--setup-text)]"
                      : failed
                        ? "border-[#ea9198]/50 text-[#ea9198]"
                        : "border-[var(--setup-border)] text-[var(--setup-text-muted)]",
                  )}
                >
                  {done ? (
                    <Check className="size-3" strokeWidth={2.5} />
                  ) : current ? (
                    <Loader2 className="size-3 animate-spin" />
                  ) : failed ? (
                    <CircleAlert className="size-3" />
                  ) : null}
                </span>
                <span
                  className={cn(
                    done || current
                      ? "text-[var(--setup-text-secondary)]"
                      : failed
                        ? "text-[#f0b4b7]"
                        : "text-[var(--setup-text-muted)]",
                  )}
                >
                  {label}
                </span>
              </li>
            );
          })}
        </ol>
      </div>
      {phase === "failed" && errorMessage ? (
        <p
          className="mt-5 rounded-lg border border-[#ea9198]/20 bg-[#ea9198]/[0.06] px-3 py-2.5 text-[12px] leading-5 text-[#f0b4b7]"
          role="alert"
        >
          {errorMessage}
        </p>
      ) : null}
      {phase === "failed" ? (
        <Button
          className="mt-6"
          onClick={() => void retryProductionInstall()}
          disabled={isPending}
        >
          <RefreshCw className="size-4" /> Retry installation
        </Button>
      ) : null}
      <p className="mt-6 max-w-[62ch] text-[12px] leading-5 text-[var(--setup-text-muted)]">
        If the browser disconnects, return to this URL while the temporary setup
        service is available. The latest state is persisted server-side.
      </p>
    </>
  );
}
