"use client";

import { Loader2, RefreshCw, ShieldCheck, Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  AssuranceStrip,
  StageActions,
  StageHeader,
  StatusPill,
} from "../primitives";
import type { SetupFlow } from "../types";

export function WelcomeStage({ flow }: { flow: SetupFlow }) {
  const {
    checks,
    checksPass,
    moveTo,
    preflight,
    retrySetupVerification,
    setupVerified,
    verifyCode,
  } = flow;
  return (
    <>
      <StageHeader
        eyebrow="Welcome"
        title="Set up your Stealth instance"
        description="The browser handles the reviewed configuration. Your terminal stays responsible for Docker and the temporary setup connection."
      />
      <AssuranceStrip
        items={[
          {
            icon: Terminal,
            title: "Local control",
            description:
              "The setup API is reachable only through your temporary setup URL.",
          },
          {
            icon: ShieldCheck,
            title: "Server-side secrets",
            description:
              "Provider credentials stay encrypted and never enter the public state.",
          },
          {
            icon: RefreshCw,
            title: "Resumable",
            description:
              "Refresh or reconnect without restarting the installation from scratch.",
          },
        ]}
      />
      <div className="mt-7 rounded-lg border border-[var(--setup-border)] p-5">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-[13px] font-medium text-[var(--setup-text)]">
              System check
            </p>
            <p className="mt-1 text-[12px] text-[var(--setup-text-muted)]">
              Checks run automatically when the setup link opens this page.
            </p>
          </div>
          {preflight.isPending ? (
            <Loader2 className="size-[18px] animate-spin text-[var(--setup-text-muted)]" />
          ) : null}
        </div>
        <div className="mt-4 overflow-hidden rounded-lg border border-[var(--setup-border)]">
          {checks.length === 0 ? (
            <p className="px-3.5 py-3 text-[13px] text-[var(--setup-text-muted)]">
              System checks will appear here.
            </p>
          ) : (
            checks.map((check) => (
              <div
                key={check.name}
                className="flex items-center justify-between gap-4 border-b border-[var(--setup-border)] px-3.5 py-3 last:border-b-0"
              >
                <div className="min-w-0">
                  <p className="text-[13px] text-[var(--setup-text-secondary)]">
                    {check.name}
                  </p>
                  <p className="mt-0.5 text-[12px] text-[var(--setup-text-muted)]">
                    {check.detail}
                  </p>
                </div>
                <StatusPill
                  status={
                    check.status === "pass"
                      ? "ready"
                      : check.status === "warn"
                        ? "warning"
                        : "error"
                  }
                >
                  {check.status === "pass"
                    ? "Ready"
                    : check.status === "warn"
                      ? "Review"
                      : "Blocked"}
                </StatusPill>
              </div>
            ))
          )}
        </div>
      </div>
      {!setupVerified ? (
        <div className="mt-7 flex flex-col gap-3 rounded-lg border border-[var(--setup-border)] p-5 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-[13px] font-medium text-[var(--setup-text)]">
              Setup link not verified
            </p>
            <p className="mt-1 max-w-[52ch] text-[12px] leading-5 text-[var(--setup-text-muted)]">
              This page verifies the one-time code carried by the setup link the
              CLI printed. Open that link, or retry here while the code is still
              valid.
            </p>
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={() => void retrySetupVerification()}
            disabled={verifyCode.isPending}
            className="shrink-0"
          >
            {verifyCode.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <RefreshCw className="size-4" />
            )}
            Retry verification
          </Button>
        </div>
      ) : null}
      <StageActions
        next={() => moveTo("instance")}
        nextLabel="Start configuration"
        nextDisabled={!setupVerified || !checksPass}
      />
    </>
  );
}
