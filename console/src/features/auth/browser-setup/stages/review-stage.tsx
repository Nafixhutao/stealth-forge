"use client";

import { defaultConfig } from "../../browser-setup-model";
import { ReviewRow, StageActions, StageHeader } from "../primitives";
import type { SetupFlow } from "../types";

export function ReviewStage({ flow }: { flow: SetupFlow }) {
  const {
    dataReady,
    databaseMode,
    isPending,
    moveTo,
    networkMode,
    ownerConfirmed,
    startProductionInstall,
    state,
    redisMode,
    storageMode,
    storageReady,
    watchedConfig,
  } = flow;
  return (
    <>
      <StageHeader
        eyebrow="06 / Review"
        title="Review the production handoff"
        description="This is the final configuration the setup service will validate and persist before the host installer starts the production Compose project."
      />
      <dl className="rounded-xl border border-stealth-border px-4">
        <ReviewRow
          label="Instance"
          value={watchedConfig.instance_name ?? defaultConfig.instance_name}
        />
        <ReviewRow
          label="Public URL"
          value={watchedConfig.public_url ?? defaultConfig.public_url}
        />
        <ReviewRow
          label="Networking"
          value={networkMode.replaceAll("_", " ")}
        />
        <ReviewRow
          label="GitHub"
          value={
            state?.github.client_id
              ? `Connected (${state.github.client_id})`
              : "Not connected"
          }
        />
        <ReviewRow
          label="PostgreSQL"
          value={databaseMode === "bundled" ? "Bundled" : "External URL tested"}
        />
        <ReviewRow
          label="Redis"
          value={redisMode === "bundled" ? "Bundled" : "External URL tested"}
        />
        <ReviewRow
          label="Storage"
          value={
            storageMode === "local"
              ? "Local volume"
              : "S3-compatible settings tested"
          }
        />
        {networkMode === "cloudflare_tunnel" ? (
          <ReviewRow
            label="Tunnel"
            value={watchedConfig.hostname || "Not configured"}
          />
        ) : null}
      </dl>
      <div className="mt-6 rounded-xl border border-amber-300/20 bg-amber-300/[0.06] p-4 text-xs leading-5 text-amber-100/80">
        Installation starts production services and may create provider-side
        resources. The operation is resumable, but provider side effects are not
        rolled back automatically.
      </div>
      <StageActions
        back={() => moveTo("storage")}
        next={() => void startProductionInstall()}
        nextLabel="Install Stealth"
        nextDisabled={!ownerConfirmed || !dataReady || !storageReady}
        pending={isPending}
      />
    </>
  );
}
