"use client";

import { CheckCircle2, RefreshCw, XCircle } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button } from "@/components/ui/button";
import { useBrowserSetupFlow } from "../browser-setup-flow";
import { setupSteps, type SetupStep } from "../browser-setup-model";
import { HandoffSubmission } from "./handoff-submission";
import { safeError } from "./helpers";
import { ActionError, LoadingPanel, ProductionLink } from "./primitives";
import { SetupShell } from "./setup-shell";
import { SetupUnavailable } from "./setup-unavailable";
import { StepRail } from "./step-rail";
import { DataStage } from "./stages/data-stage";
import { GitHubStage } from "./stages/github-stage";
import { InstallStage } from "./stages/install-stage";
import { InstanceStage } from "./stages/instance-stage";
import { NetworkingStage } from "./stages/networking-stage";
import { ReviewStage } from "./stages/review-stage";
import { SignInStage } from "./stages/signin-stage";
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
      // Setup is not available on this instance (already installed, or this
      // deployment is not in setup mode). Never render the wizard here.
      return <SetupUnavailable />;
    }
    return (
      <SetupShell currentStep="welcome">
        <div
          role="alert"
          className="flex flex-col gap-4 rounded-lg border border-[#ea9198]/25 bg-[#ea9198]/[0.05] p-5"
        >
          <div className="flex items-center gap-2 text-[13px] font-medium text-[#f0b4b7]">
            <XCircle className="size-4" /> Setup service unavailable
          </div>
          <p className="text-[13px] text-[var(--setup-text-muted)]">
            {safeError(setupStatus.error)}
          </p>
          <Button
            variant="outline"
            className="w-full sm:w-auto"
            onClick={() => void setupStatus.refetch()}
          >
            <RefreshCw className="size-4" /> Retry
          </Button>
        </div>
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
    <SetupShell
      currentStep={activeStep}
      rail={
        <StepRail
          steps={setupSteps}
          activeStep={activeStep}
          canOpen={canOpenStep}
          onSelect={moveTo}
        />
      }
    >
      {handoffReady ? (
        <HandoffSubmission
          publicURL={installPublicURL}
          persistedURL={installViewState?.draft.public_url}
          token={handoffToken}
        />
      ) : null}

      {notice || callbackNotice ? (
        <div
          className="mb-5 flex items-start gap-2 rounded-lg border border-[var(--setup-border-strong)] bg-white/[0.04] px-3 py-2.5 text-[12px] leading-5 text-[var(--setup-text-secondary)]"
          role="status"
        >
          <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-[#76bc9a]" />
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

      {activeStep === "signin" ? <SignInStage flow={flow} /> : null}

      {activeStep === "review" ? <ReviewStage flow={flow} /> : null}

      {activeStep === "install" ? <InstallStage flow={flow} /> : null}
    </SetupShell>
  );
}
