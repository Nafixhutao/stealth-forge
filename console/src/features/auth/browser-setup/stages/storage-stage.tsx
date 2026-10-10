"use client";

import { Cloud, HardDrive, Loader2, Wifi } from "lucide-react";
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
import { Switch } from "@/components/motion/switch";

export function StorageStage({ flow }: { flow: SetupFlow }) {
  const {
    configForm,
    moveTo,
    persistConfig,
    saveConfig,
    setActionError,
    state,
    storageMode,
    storageReady,
    testStorage,
    testStorageConnection,
  } = flow;
  return (
    <>
      <StageHeader
        eyebrow="05 / Storage"
        title="Choose artifact storage"
        description="Local storage is persistent on this host. S3-compatible storage is tested before it is written into the final production environment."
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <ChoiceCard
          active={storageMode === "local"}
          icon={HardDrive}
          title="Local volume"
          description="Use the persistent stealth_storage volume on this host."
          onClick={() =>
            configForm.setValue("storage_mode", "local", {
              shouldDirty: true,
            })
          }
        />
        <ChoiceCard
          active={storageMode === "s3"}
          icon={Cloud}
          title="S3-compatible"
          description="Use an external object store for durable artifacts and files."
          onClick={() =>
            configForm.setValue("storage_mode", "s3", {
              shouldDirty: true,
            })
          }
        />
      </div>
      {storageMode === "s3" ? (
        <div className="mt-6 space-y-5 rounded-xl border border-stealth-border p-5">
          <div className="grid gap-5 sm:grid-cols-2">
            <Field id="storage-endpoint" label="Endpoint">
              <Input
                id="storage-endpoint"
                type="url"
                placeholder="https://s3.example.com"
                {...configForm.register("storage_s3_endpoint")}
              />
            </Field>
            <Field id="storage-region" label="Region">
              <Input
                id="storage-region"
                placeholder="us-east-1"
                {...configForm.register("storage_s3_region")}
              />
            </Field>
            <Field id="storage-bucket" label="Bucket">
              <Input
                id="storage-bucket"
                placeholder="stealth-production"
                {...configForm.register("storage_s3_bucket")}
              />
            </Field>
            <Field
              id="storage-prefix"
              label="Prefix"
              hint="Optional path prefix."
            >
              <Input
                id="storage-prefix"
                placeholder="stealth"
                {...configForm.register("storage_s3_prefix")}
              />
            </Field>
            <Field id="storage-access-key" label="Access key">
              <Input
                id="storage-access-key"
                autoComplete="off"
                {...configForm.register("storage_s3_access_key")}
              />
            </Field>
            <Field id="storage-secret-key" label="Secret key">
              <Input
                id="storage-secret-key"
                type="password"
                autoComplete="new-password"
                {...configForm.register("storage_s3_secret_key")}
              />
            </Field>
          </div>
          <div className="flex flex-wrap gap-5 text-sm text-slate-300">
            <Switch
              checked={configForm.watch("storage_s3_use_ssl") === true}
              onCheckedChange={(value) =>
                configForm.setValue("storage_s3_use_ssl", value, {
                  shouldDirty: true,
                })
              }
              label="Use TLS"
              activeClassName="bg-cyan-400"
              className="min-h-11"
            />
            <Switch
              checked={configForm.watch("storage_s3_path_style") === true}
              onCheckedChange={(value) =>
                configForm.setValue("storage_s3_path_style", value, {
                  shouldDirty: true,
                })
              }
              label="Force path style"
              activeClassName="bg-cyan-400"
              className="min-h-11"
            />
          </div>
          <div className="flex flex-wrap items-center gap-3 border-t border-stealth-border pt-5">
            <Button
              type="button"
              onClick={() => void testStorageConnection()}
              disabled={testStorage.isPending}
            >
              {testStorage.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Wifi className="size-4" />
              )}
              Test storage
            </Button>
            {state?.draft.storage_tested ? (
              <StatusPill status="ready">Storage tested</StatusPill>
            ) : (
              <StatusPill status="warning">Test required</StatusPill>
            )}
          </div>
        </div>
      ) : (
        <div className="mt-6 rounded-xl border border-stealth-border bg-black/10 p-5 text-sm leading-6 text-slate-400">
          The setup API will verify that the local storage volume is writable
          during the system check.
        </div>
      )}
      <StageActions
        back={() => moveTo("data")}
        next={async () => {
          try {
            await persistConfig(configForm.getValues());
            moveTo("review");
          } catch (error) {
            setActionError(error);
          }
        }}
        nextLabel="Review installation"
        nextDisabled={!storageReady}
        pending={saveConfig.isPending}
      />
    </>
  );
}
