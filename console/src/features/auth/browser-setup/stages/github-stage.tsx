"use client";

import { CheckCircle2, GitBranch, Loader2, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, StageActions, StageHeader } from "../primitives";
import type { SetupFlow } from "../types";

export function GitHubStage({ flow }: { flow: SetupFlow }) {
  const {
    authorizeGitHubOwner,
    manualForm,
    moveTo,
    ownerConfirmed,
    saveManual,
    saveManualGitHub,
    startAuthorization,
    state,
  } = flow;
  return (
    <>
      <StageHeader
        eyebrow="02 / GitHub"
        title="Connect the owner identity"
        description="Register a GitHub OAuth App, paste its client credentials, then verify the person who will become the first Instance Owner."
      />
      {state?.github.connected ? (
        <div className="flex items-start gap-3 rounded-xl border border-emerald-300/20 bg-emerald-300/[0.06] p-5">
          <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-emerald-300" />
          <div>
            <p className="text-sm font-medium text-emerald-100">
              GitHub OAuth App connected
            </p>
            <p className="mt-1 text-xs leading-5 text-emerald-200/70">
              The client credentials are held by the setup API. The Console only
              sees the connection status.
            </p>
          </div>
        </div>
      ) : (
        <form onSubmit={saveManualGitHub} className="space-y-5">
          <div className="rounded-xl border border-stealth-border p-5">
            <div className="flex items-start gap-3">
              <GitBranch className="mt-0.5 size-5 shrink-0 text-slate-200" />
              <div>
                <p className="text-sm font-medium text-paper">
                  Create a GitHub OAuth App
                </p>
                <p className="mt-1 text-xs leading-5 text-slate-500">
                  In GitHub → Settings → Developer settings → OAuth Apps, create
                  an app and set the callback URL to{" "}
                  <code className="text-slate-300">
                    /v1/setup/github/authorize/callback
                  </code>{" "}
                  on this host. Paste its Client ID and Client Secret below.
                </p>
              </div>
            </div>
          </div>
          <Field
            id="github-client-id"
            label="Client ID"
            error={manualForm.formState.errors.client_id?.message}
            className="max-w-[520px]"
          >
            <Input
              id="github-client-id"
              autoComplete="off"
              {...manualForm.register("client_id")}
            />
          </Field>
          <Field
            id="github-client-secret"
            label="Client secret"
            error={manualForm.formState.errors.client_secret?.message}
            className="max-w-[520px]"
          >
            <Input
              id="github-client-secret"
              type="password"
              autoComplete="new-password"
              {...manualForm.register("client_secret")}
            />
          </Field>
          <Button type="submit" disabled={saveManual.isPending}>
            {saveManual.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <ShieldCheck className="size-4" />
            )}
            {saveManual.isPending
              ? "Saving securely…"
              : "Save GitHub OAuth App"}
          </Button>
        </form>
      )}
      {state?.github.connected ? (
        <div className="mt-6 border-t border-stealth-border pt-6">
          {ownerConfirmed ? (
            <div className="flex items-start gap-3 rounded-xl border border-emerald-300/20 bg-emerald-300/[0.06] p-5">
              <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-emerald-300" />
              <div>
                <p className="text-sm font-medium text-emerald-100">
                  Instance Owner verified
                </p>
                <p className="mt-1 text-xs leading-5 text-emerald-200/70">
                  The first-owner bootstrap is sealed. The setup claim remains
                  scoped to this browser until production handoff.
                </p>
              </div>
            </div>
          ) : (
            <div className="rounded-xl border border-stealth-border p-5">
              <p className="text-sm font-medium text-paper">
                Authorize the first owner in GitHub
              </p>
              <p className="mt-1 text-xs leading-5 text-slate-500">
                Stealth opens GitHub&apos;s browser authorization page. After
                approval, the callback creates the first Instance Owner. No
                password or device code is required.
              </p>
              <Button
                className="mt-5"
                onClick={() => void authorizeGitHubOwner()}
                disabled={startAuthorization.isPending}
              >
                {startAuthorization.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <GitBranch className="size-4" />
                )}
                {startAuthorization.isPending
                  ? "Opening GitHub"
                  : "Authorize owner in GitHub"}
              </Button>
            </div>
          )}
        </div>
      ) : null}
      <StageActions
        back={() => moveTo("instance")}
        next={() => moveTo("networking")}
        nextLabel="Continue to networking"
        nextDisabled={!state?.github.connected || !ownerConfirmed}
      />
    </>
  );
}
