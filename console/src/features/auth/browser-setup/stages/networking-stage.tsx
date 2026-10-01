"use client";

import {
  CheckCircle2,
  Cloud,
  Loader2,
  Network,
  ServerCog,
  Wifi,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  ChoiceCard,
  Field,
  StageActions,
  StageHeader,
  StatusPill,
} from "../primitives";
import type { SetupFlow } from "../types";

export function NetworkingStage({ flow }: { flow: SetupFlow }) {
  const {
    accounts,
    cloudflareConnected,
    cloudflareToken,
    configForm,
    createCloudflareTunnel,
    continueCloudflareSetup,
    moveTo,
    networkMode,
    saveCloudflareToken,
    saveConfig,
    selectedAccount,
    selectedZone,
    connectCloudflareToken,
    setCloudflareAccountID,
    setCloudflareToken,
    setCloudflareZoneID,
    tunnelReady,
    watchedConfig,
    zones,
  } = flow;
  return (
    <>
      <StageHeader
        eyebrow="03 / Networking"
        title="Choose production ingress"
        description="Quick Tunnel is only for this setup session. Production uses the reviewed mode below."
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <ChoiceCard
          active={networkMode === "cloudflare_tunnel"}
          icon={Cloud}
          title="Named Cloudflare Tunnel"
          description="Recommended. Creates a named tunnel and proxied DNS record for the final hostname."
          onClick={() =>
            configForm.setValue("network_mode", "cloudflare_tunnel", {
              shouldDirty: true,
            })
          }
        />
        <ChoiceCard
          active={networkMode === "public_ip"}
          icon={Wifi}
          title="Public IP"
          description="Binds the bundled proxy publicly. Put TLS termination in front of it."
          onClick={() =>
            configForm.setValue("network_mode", "public_ip", {
              shouldDirty: true,
            })
          }
        />
        <ChoiceCard
          active={networkMode === "reverse_proxy"}
          icon={Network}
          title="Existing reverse proxy"
          description="Keep proxy binding private and route the configured URL from your own ingress."
          onClick={() =>
            configForm.setValue("network_mode", "reverse_proxy", {
              shouldDirty: true,
            })
          }
        />
        <ChoiceCard
          active={networkMode === "local_only"}
          icon={ServerCog}
          title="Local only"
          description="Keep the proxy on loopback for a private operator installation."
          onClick={() =>
            configForm.setValue("network_mode", "local_only", {
              shouldDirty: true,
            })
          }
        />
      </div>
      {networkMode === "cloudflare_tunnel" ? (
        <div className="mt-6 space-y-5 rounded-xl border border-stealth-border p-5">
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium text-white">
                Cloudflare connection
              </p>
              <p className="mt-1 text-xs text-slate-500">
                Verify a scoped API token here. Stealth discovers the account
                and domain, then configures the named tunnel and DNS for you.
              </p>
            </div>
            {cloudflareConnected ? (
              <StatusPill status="ready">
                <CheckCircle2 className="size-3.5" />
                Cloudflare connected
              </StatusPill>
            ) : null}
          </div>
          {!cloudflareConnected ? (
            <div className="space-y-3">
              <Field
                id="cloudflare-api-token"
                label="API Token"
                hint="The token is sent only to the setup API and is never returned to browser state."
              >
                <div className="flex flex-col gap-2 sm:flex-row">
                  <Input
                    id="cloudflare-api-token"
                    type="password"
                    placeholder="Scoped Cloudflare API token"
                    value={cloudflareToken}
                    onChange={(event) => setCloudflareToken(event.target.value)}
                    autoComplete="new-password"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => void connectCloudflareToken()}
                    disabled={saveCloudflareToken.isPending}
                    className="sm:min-w-32"
                  >
                    {saveCloudflareToken.isPending ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : null}
                    Verify Token
                  </Button>
                </div>
              </Field>
              <p className="text-xs leading-5 text-slate-500">
                Create a custom token with Account: Cloudflare Tunnel Edit and
                Account Settings Read; Zone: Zone Read and DNS Edit for
                Console/workload zones, plus SSL and Certificates Read for
                workload edge TLS readiness. Scope it to the account and zones
                you use. Global API keys are not accepted.
              </p>
            </div>
          ) : (
            <>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field id="cloudflare-account" label="Account">
                  <select
                    id="cloudflare-account"
                    value={selectedAccount}
                    onChange={(event) => {
                      setCloudflareAccountID(event.target.value);
                      setCloudflareZoneID("");
                    }}
                    className="min-h-11 w-full rounded-lg border border-control-border bg-stealth-panel px-3 text-sm text-white focus-visible:border-cyan-300/60"
                  >
                    <option value="">
                      {accounts.isPending
                        ? "Discovering accounts…"
                        : "Select an account"}
                    </option>
                    {(accounts.data?.accounts ?? []).map((account) => (
                      <option key={account.id} value={account.id}>
                        {account.name}
                      </option>
                    ))}
                  </select>
                  {accounts.isError ? (
                    <p className="text-xs leading-5 text-rose-300">
                      Account discovery failed. Check the token scope and try
                      again.
                    </p>
                  ) : null}
                </Field>
                <Field id="cloudflare-zone" label="Domain">
                  <select
                    id="cloudflare-zone"
                    value={selectedZone}
                    onChange={(event) =>
                      setCloudflareZoneID(event.target.value)
                    }
                    disabled={!selectedAccount}
                    className="min-h-11 w-full rounded-lg border border-control-border bg-stealth-panel px-3 text-sm text-white focus-visible:border-cyan-300/60 disabled:opacity-50"
                  >
                    <option value="">
                      {zones.isPending
                        ? "Discovering domains…"
                        : "Select a domain"}
                    </option>
                    {(zones.data?.zones ?? []).map((zone) => (
                      <option key={zone.id} value={zone.id}>
                        {zone.name}
                      </option>
                    ))}
                  </select>
                  {zones.isError ? (
                    <p className="text-xs leading-5 text-rose-300">
                      Domain discovery failed. Check Zone Read access for the
                      selected account.
                    </p>
                  ) : null}
                </Field>
              </div>
              <Field
                id="cloudflare-hostname"
                label="Dashboard hostname"
                hint="The hostname must be inside the selected zone."
              >
                <Input
                  id="cloudflare-hostname"
                  placeholder="console.example.com"
                  {...configForm.register("hostname")}
                />
              </Field>
              {tunnelReady ? (
                <StatusPill status="ready">
                  <CheckCircle2 className="size-3.5" />
                  Tunnel and DNS ready
                </StatusPill>
              ) : (
                <p className="text-xs leading-5 text-slate-500">
                  Continue provisions the named tunnel, ingress, DNS, and
                  cloudflared configuration automatically.
                </p>
              )}
            </>
          )}
        </div>
      ) : (
        <div className="mt-6 rounded-xl border border-stealth-border bg-black/10 p-5">
          <p className="text-sm font-medium text-white">Public URL</p>
          <p className="mt-1 text-xs leading-5 text-slate-500">
            {watchedConfig.public_url}
          </p>
          <p className="mt-3 text-xs leading-5 text-amber-200/80">
            You are responsible for TLS termination and firewall policy for this
            mode.
          </p>
        </div>
      )}
      <StageActions
        back={() => moveTo("github")}
        next={() => void continueCloudflareSetup()}
        nextLabel="Continue"
        nextDisabled={
          networkMode === "cloudflare_tunnel" &&
          (!cloudflareConnected ||
            !selectedAccount ||
            !selectedZone ||
            !accounts.data?.accounts?.length ||
            !zones.data?.zones?.length ||
            accounts.isError ||
            zones.isError ||
            !watchedConfig.hostname?.trim() ||
            createCloudflareTunnel.isPending)
        }
        pending={saveConfig.isPending || createCloudflareTunnel.isPending}
      />
    </>
  );
}
