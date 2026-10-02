"use client";

import { CheckCircle2, RefreshCw, XCircle } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useBrowserSetupFlow } from "../browser-setup-flow";
import { setupSteps, type SetupStep } from "../browser-setup-model";
import { HandoffSubmission } from "./handoff-submission";
import { safeError } from "./helpers";
import {
  ActionError,
  LoadingPanel,
  ProductionLink,
  StatusPill,
} from "./primitives";
import { SetupShell } from "./setup-shell";
import { DataStage } from "./stages/data-stage";
import { GitHubStage } from "./stages/github-stage";
import { InstallStage } from "./stages/install-stage";
import { InstanceStage } from "./stages/instance-stage";
import { NetworkingStage } from "./stages/networking-stage";
import { ReviewStage } from "./stages/review-stage";
import { StorageStage } from "./stages/storage-stage";
import { WelcomeStage } from "./stages/welcome-stage";

export function BrowserSetupView() {
  const flow = useBrowserSetupFlow();
  const {
    actionError,
    activeStep,
    callbackError,
    callbackNotice,
    handoffReady,
    handoffToken,
    installPhase,
    installPublicURL,
    installViewState,
    moveTo,
    notice,
    setupStatus,
  } = flow;

  const currentIndex = setupSteps.findIndex((item) => item.id === activeStep);
  const canOpenStep = (target: SetupStep) => {
    const targetIndex = setupSteps.findIndex((item) => item.id === target);
    return targetIndex <= currentIndex && activeStep !== "install";
  };

  if (setupStatus.isPending) {
    return (
      <SetupShell currentStep="welcome">
        <LoadingPanel label="Checking setup service" />
      </SetupShell>
    );
  }

  if (setupStatus.error) {
    if (
      setupStatus.error instanceof ApiError &&
      setupStatus.error.status === 404
    ) {
      return (
        <SetupShell currentStep="welcome">
          <ProductionLink />
        </SetupShell>
      );
    }
    return (
      <SetupShell currentStep="welcome">
        <Card role="alert" className="border-rose-300/20 bg-rose-400/[0.04]">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-rose-200">
              <XCircle className="size-4" /> Setup service unavailable
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap items-center gap-3">
            <p className="text-sm text-slate-400">
              {safeError(setupStatus.error)}
            </p>
            <Button
              variant="outline"
              onClick={() => void setupStatus.refetch()}
            >
              <RefreshCw className="size-4" /> Retry
            </Button>
          </CardContent>
        </Card>
      </SetupShell>
    );
  }

  if (installPhase === "complete") {
    return (
      <SetupShell currentStep="install">
        {handoffReady ? (
          <HandoffSubmission
            publicURL={installPublicURL}
            persistedURL={installViewState?.draft.public_url}
            token={handoffToken}
          />
        ) : null}
        <ProductionLink publicURL={installPublicURL} />
      </SetupShell>
    );
  }

  return (
    <SetupShell currentStep={activeStep}>
      {handoffReady ? (
        <HandoffSubmission
          publicURL={installPublicURL}
          persistedURL={installViewState?.draft.public_url}
          token={handoffToken}
        />
      ) : null}
      <div className="grid gap-7 lg:grid-cols-[220px_minmax(0,1fr)]">
        <nav aria-label="Setup progress" className="lg:pt-1">
          <div className="mb-3 flex items-center justify-between lg:block">
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-slate-500">
              Setup progress
            </p>
            <StatusPill
              status={
                installViewState?.phase === "failed" ? "error" : "pending"
              }
            >
              {installViewState?.phase === "failed"
                ? "Needs attention"
                : "In progress"}
            </StatusPill>
          </div>
          <ol className="flex gap-2 overflow-x-auto pb-2 lg:block lg:space-y-1 lg:overflow-visible">
            {setupSteps.map((item, index) => {
              const active = item.id === activeStep;
              const available = canOpenStep(item.id) || active;
              return (
                <li key={item.id} className="shrink-0">
                  <button
                    type="button"
                    disabled={!available}
                    onClick={() => available && moveTo(item.id)}
                    className={`flex min-h-11 items-center gap-2 rounded-lg px-2.5 py-2 text-left text-xs transition-colors duration-150 focus-visible:ring-2 focus-visible:ring-acid-lime/40 lg:w-full ${active ? "bg-acid-lime/[0.08] text-mist" : available ? "text-fog hover:bg-white/[0.04] hover:text-paper" : "cursor-not-allowed text-slate-700"}`}
                  >
                    <span
                      className={`flex size-6 items-center justify-center rounded-full border font-mono text-[10px] ${active ? "border-cyan-300/60" : "border-stealth-border"}`}
                    >
                      {index === 0 ? "S" : item.short}
                    </span>
                    <span className="hidden lg:block">{item.label}</span>
                  </button>
                </li>
              );
            })}
          </ol>
        </nav>

        <section
          className="min-w-0 rounded-xl border border-stealth-border bg-stealth-panel/70 p-5 sm:p-7"
          aria-live="polite"
        >
          {notice || callbackNotice ? (
            <div
              className="mb-5 flex items-start gap-2 rounded-lg border border-cyan-300/20 bg-cyan-300/[0.06] px-3 py-2.5 text-xs leading-5 text-cyan-100"
              role="status"
            >
              <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-cyan-300" />
              <span>{notice || callbackNotice}</span>
            </div>
          ) : null}
          <ActionError error={actionError ?? callbackError} />

          {activeStep === "welcome" ? <WelcomeStage flow={flow} /> : null}

          {activeStep === "instance" ? <InstanceStage flow={flow} /> : null}

          {activeStep === "github" ? <GitHubStage flow={flow} /> : null}

          {activeStep === "networking" ? <NetworkingStage flow={flow} /> : null}

          {activeStep === "data" ? <DataStage flow={flow} /> : null}

          {activeStep === "storage" ? <StorageStage flow={flow} /> : null}

          {activeStep === "review" ? <ReviewStage flow={flow} /> : null}

          {activeStep === "install" ? <InstallStage flow={flow} /> : null}
        </section>
      </div>
    </SetupShell>
  );
}
