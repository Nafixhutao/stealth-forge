"use client";

import {
  KeyRound,
  Loader2,
  RefreshCw,
  ShieldCheck,
  Terminal,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { InfoCard, StageActions, StageHeader, StatusPill } from "../primitives";
import type { SetupFlow } from "../types";

export function WelcomeStage({ flow }: { flow: SetupFlow }) {
  const {
    checks,
    checksPass,
    moveTo,
    preflight,
    setupCode,
    setupVerified,
    setSetupCode,
    verifyCode,
    verifySetupCode,
  } = flow;
  return (
    <>
      <StageHeader
        eyebrow="Welcome"
        title="Set up your Stealth instance"
        description="The browser handles the reviewed configuration. Your terminal stays responsible for Docker and the temporary setup connection."
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <InfoCard
          icon={Terminal}
          title="Local control"
          description="The setup API is reachable only through your temporary setup URL."
        />
        <InfoCard
          icon={ShieldCheck}
          title="Server-side secrets"
          description="Provider credentials stay encrypted and never enter the public state."
        />
        <InfoCard
          icon={RefreshCw}
          title="Resumable"
          description="Refresh or reconnect without restarting the installation from scratch."
        />
      </div>
      <div className="mt-7 rounded-xl border border-stealth-border bg-black/10 p-5">
        <div className="flex items-start gap-3">
          <KeyRound className="mt-0.5 size-5 shrink-0 text-cyan-300" />
          <div>
            <p className="text-sm font-medium text-white">
              Verify this setup session
            </p>
            <p className="mt-1 text-xs leading-5 text-slate-500">
              Enter the one-time code printed by `stealth install`. It expires
              after 15 minutes and is not placed in the URL.
            </p>
          </div>
        </div>
        <div className="mt-5 flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="flex-1 space-y-2">
            <Label htmlFor="browser-setup-code">Setup code</Label>
            <Input
              id="browser-setup-code"
              value={setupCode}
              onChange={(event) =>
                setSetupCode(event.target.value.toUpperCase())
              }
              placeholder="STEALTH-XXXX-XXXX-XXXX"
              autoComplete="one-time-code"
              spellCheck={false}
              translate="no"
              disabled={setupVerified}
            />
          </div>
          <Button
            type="button"
            onClick={() => void verifySetupCode()}
            disabled={setupVerified || verifyCode.isPending}
          >
            {verifyCode.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <ShieldCheck className="size-4" />
            )}
            {setupVerified ? "Verified" : "Verify code"}
          </Button>
        </div>
      </div>
      <div className="mt-7 rounded-xl border border-stealth-border p-5">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-white">System check</p>
            <p className="mt-1 text-xs text-slate-500">
              Unlock the live dependency checks after verifying the setup code.
            </p>
          </div>
          {preflight.isPending ? (
            <Loader2 className="size-5 animate-spin text-cyan-300" />
          ) : null}
        </div>
        <div className="mt-4 space-y-2">
          {checks.length === 0 ? (
            <p className="text-sm text-slate-600">
              System checks will appear here.
            </p>
          ) : (
            checks.map((check) => (
              <div
                key={check.name}
                className="flex items-start justify-between gap-4 rounded-lg bg-black/10 px-3 py-2.5"
              >
                <div>
                  <p className="text-sm text-slate-200">{check.name}</p>
                  <p className="mt-0.5 text-xs text-slate-600">
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
      <StageActions
        next={() => moveTo("instance")}
        nextLabel="Start configuration"
        nextDisabled={!setupVerified || !checksPass}
      />
    </>
  );
}
