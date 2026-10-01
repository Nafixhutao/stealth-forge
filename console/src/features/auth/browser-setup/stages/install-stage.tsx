"use client";

import { Check, CircleAlert, Loader2, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  installationStepState,
  installationSteps,
} from "../../browser-setup-model";
import { StageHeader, StatusPill } from "../primitives";
import type { SetupFlow } from "../types";

export function InstallStage({ flow }: { flow: SetupFlow }) {
  const { installViewState, isPending, retryProductionInstall, sseConnected } =
    flow;
  return (
    <>
      <StageHeader
        eyebrow="07 / Install"
        title={
          installViewState?.phase === "failed"
            ? "Installation needs attention"
            : "Installing Stealth"
        }
        description={
          installViewState?.phase === "failed"
            ? "The production stack is still repairable. Review the safe error below and retry after correcting the underlying issue."
            : installViewState?.phase === "install_requested"
              ? "Your configuration was received. The host installer is preparing the installation."
              : "The host installer is running the shared install engine. Keep this window open until the production session handoff completes."
        }
      />
      <div className="rounded-xl border border-stealth-border p-5">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-white">
              {installViewState?.step ?? "Preparing installation"}
            </p>
            <p className="mt-1 text-xs text-slate-500">
              {sseConnected
                ? "Live progress connected"
                : "Reconnecting to live progress"}
            </p>
          </div>
          {installViewState?.phase === "failed" ? (
            <StatusPill status="error">Failed</StatusPill>
          ) : (
            <StatusPill status="pending">Working</StatusPill>
          )}
        </div>
        <div className="mt-5 space-y-2">
          {installationSteps.map((label) => {
            const status = installationStepState(
              installViewState?.phase,
              installViewState?.step,
              label,
            );
            const done = status === "complete";
            const current = status === "current";
            const failed = status === "failed";
            return (
              <div
                key={label}
                aria-current={current ? "step" : undefined}
                className={`flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm ${done ? "bg-cyan-300/[0.06] text-cyan-100" : current ? "bg-white/[0.03] text-slate-200" : failed ? "bg-rose-400/[0.06] text-rose-200" : "text-slate-500"}`}
              >
                <span
                  className={`flex size-5 items-center justify-center rounded-full border ${done ? "border-cyan-300/50 text-cyan-300" : current ? "border-cyan-300/50 text-cyan-300" : failed ? "border-rose-300/50 text-rose-300" : "border-stealth-border"}`}
                >
                  {done ? (
                    <Check className="size-3" />
                  ) : current ? (
                    <Loader2 className="size-3 animate-spin" />
                  ) : failed ? (
                    <CircleAlert className="size-3" />
                  ) : null}
                </span>
                {label}
              </div>
            );
          })}
        </div>
        {installViewState?.phase === "failed" &&
        installViewState.error_message ? (
          <p
            className="mt-5 rounded-lg border border-rose-300/20 bg-rose-400/[0.08] px-3 py-2.5 text-xs leading-5 text-rose-200"
            role="alert"
          >
            {installViewState.error_message}
          </p>
        ) : null}
      </div>
      {installViewState?.phase === "failed" ? (
        <Button
          className="mt-6"
          onClick={() => void retryProductionInstall()}
          disabled={isPending}
        >
          <RefreshCw className="size-4" /> Retry installation
        </Button>
      ) : null}
      <p className="mt-6 text-xs leading-5 text-slate-600">
        If the browser disconnects, return to this URL while the temporary setup
        service is available. The latest state is persisted server-side.
      </p>
    </>
  );
}
