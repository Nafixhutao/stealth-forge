"use client";

import { CheckCircle2, GitBranch, Loader2, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { submitGitHubManifest } from "../helpers";
import { Field, StageActions, StageHeader } from "../primitives";
import type { SetupFlow } from "../types";

export function GitHubStage({ flow }: { flow: SetupFlow }) {
  const {
    authorizeGitHubOwner,
    manualForm,
    moveTo,
    ownerConfirmed,
    providerMode,
    saveManual,
    saveManualGitHub,
    setActionError,
    setProviderMode,
    startAuthorization,
    startManifest,
    state,
  } = flow;
  return (
    <>
      <StageHeader
        eyebrow="02 / GitHub"
        title="Connect the owner identity"
        description="Connect a GitHub App for installation metadata, then verify the person who will become the first Instance Owner."
      />
      {state?.github.connected ? (
        <div className="flex items-start gap-3 rounded-xl border border-emerald-300/20 bg-emerald-300/[0.06] p-5">
          <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-emerald-300" />
          <div>
            <p className="text-sm font-medium text-emerald-100">
              GitHub App connected
            </p>
            <p className="mt-1 text-xs leading-5 text-emerald-200/70">
              The App credentials are held by the setup API. The Console only
              sees the connection status.
            </p>
          </div>
        </div>
      ) : (
        <>
          <div className="mb-5 flex gap-1 border-b border-stealth-border">
            <button
              type="button"
              onClick={() => setProviderMode("manifest")}
              className={`min-h-11 border-b-2 px-3 text-xs font-medium ${providerMode === "manifest" ? "border-cyan-300 text-cyan-200" : "border-transparent text-slate-500"}`}
            >
              GitHub Manifest
            </button>
            <button
              type="button"
              onClick={() => setProviderMode("manual")}
              className={`min-h-11 border-b-2 px-3 text-xs font-medium ${providerMode === "manual" ? "border-cyan-300 text-cyan-200" : "border-transparent text-slate-500"}`}
            >
              Manual App
            </button>
          </div>
          {providerMode === "manifest" ? (
            <div className="rounded-xl border border-stealth-border p-5">
              <div className="flex items-start gap-3">
                <GitBranch className="mt-0.5 size-5 shrink-0 text-slate-200" />
                <div>
                  <p className="text-sm font-medium text-white">
                    Create a private GitHub App
                  </p>
                  <p className="mt-1 text-xs leading-5 text-slate-500">
                    GitHub opens its registration page, then returns the App
                    credentials and opens a second browser authorization step
                    for the first owner.
                  </p>
                </div>
              </div>
              <Button
                className="mt-5"
                onClick={() =>
                  void startManifest
                    .mutateAsync()
                    .then((result) => {
                      if (!result?.manifest_url || !result.manifest) {
                        throw new Error(
                          "GitHub App Manifest data was not returned.",
                        );
                      }
                      submitGitHubManifest(
                        result.manifest_url,
                        result.manifest,
                      );
                    })
                    .catch(setActionError)
                }
                disabled={startManifest.isPending}
              >
                {startManifest.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <GitBranch className="size-4" />
                )}
                {startManifest.isPending
                  ? "Opening GitHub"
                  : "Create App and authorize owner"}
              </Button>
            </div>
          ) : (
            <form onSubmit={saveManualGitHub} className="space-y-5">
              <Field
                id="github-client-id"
                label="Client ID"
                error={manualForm.formState.errors.client_id?.message}
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
              >
                <Input
                  id="github-client-secret"
                  type="password"
                  autoComplete="new-password"
                  {...manualForm.register("client_secret")}
                />
              </Field>
              <Field
                id="github-private-key"
                label="Private key"
                hint="Paste the complete PEM value. It is submitted only to the setup API."
                error={manualForm.formState.errors.private_key?.message}
              >
                <Textarea
                  id="github-private-key"
                  rows={8}
                  spellCheck={false}
                  {...manualForm.register("private_key")}
                />
              </Field>
              <Field
                id="github-webhook-secret"
                label="Webhook secret"
                hint="Optional for first-run setup."
              >
                <Input
                  id="github-webhook-secret"
                  type="password"
                  autoComplete="new-password"
                  {...manualForm.register("webhook_secret")}
                />
              </Field>
              <Button type="submit" disabled={saveManual.isPending}>
                {saveManual.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <ShieldCheck className="size-4" />
                )}
                {saveManual.isPending ? "Saving securely…" : "Save GitHub App"}
              </Button>
            </form>
          )}
        </>
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
              <p className="text-sm font-medium text-white">
                Authorize the first owner in GitHub
              </p>
              <p className="mt-1 text-xs leading-5 text-slate-500">
                Stealth will open GitHub&apos;s browser authorization page.
                After approval, the callback creates the first Instance Owner.
                No password or device code is required.
              </p>
              {state.github.mode === "manual" ? (
                <p className="mt-3 rounded-lg border border-amber-300/20 bg-amber-300/[0.06] px-3 py-2.5 text-xs leading-5 text-amber-100">
                  Manual Apps must have the exact HTTPS callback URL
                  <code className="mx-1 break-all text-amber-200">
                    /v1/setup/github/authorize/callback
                  </code>
                  registered in GitHub App settings.
                </p>
              ) : null}
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
