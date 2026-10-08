"use client";

import { KeyRound } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Field, StageActions, StageHeader } from "../primitives";
import type { SetupFlow } from "../types";

/**
 * Optional external sign-in. Both providers are skipped when their fields are
 * empty, and the operator can change them later from the Admin Console.
 */
export function SignInStage({ flow }: { flow: SetupFlow }) {
  const { configForm, moveTo, persistConfig, saveConfig, setActionError } =
    flow;
  const publicURL = configForm.watch("public_url");
  const callbackBase = publicURL?.replace(/\/+$/, "") || "";

  return (
    <>
      <StageHeader
        eyebrow="06 / Sign-in"
        title="Optional: external sign-in"
        description="Let people sign in with GitHub or Google. Leave a provider empty to disable it. You can change these later in the Admin Console without touching the server."
      />

      <div className="space-y-5">
        <ProviderFields
          title="GitHub"
          clientIdField="oauth_github_client_id"
          clientSecretField="oauth_github_client_secret"
          callbackURL={
            callbackBase
              ? `${callbackBase}/v1/oauth/github/callback`
              : "/v1/oauth/github/callback"
          }
          register={configForm.register}
        />
        <ProviderFields
          title="Google"
          clientIdField="oauth_google_client_id"
          clientSecretField="oauth_google_client_secret"
          callbackURL={
            callbackBase
              ? `${callbackBase}/v1/oauth/google/callback`
              : "/v1/oauth/google/callback"
          }
          register={configForm.register}
        />
      </div>

      <StageActions
        back={() => moveTo("storage")}
        next={async () => {
          try {
            await persistConfig(configForm.getValues());
            moveTo("review");
          } catch (error) {
            setActionError(error);
          }
        }}
        nextLabel="Review installation"
        pending={saveConfig.isPending}
      />
    </>
  );
}

function ProviderFields({
  title,
  clientIdField,
  clientSecretField,
  callbackURL,
  register,
}: {
  title: string;
  clientIdField: "oauth_github_client_id" | "oauth_google_client_id";
  clientSecretField:
    "oauth_github_client_secret" | "oauth_google_client_secret";
  callbackURL: string;
  register: SetupFlow["configForm"]["register"];
}) {
  return (
    <div className="rounded-xl border border-stealth-border p-5">
      <div className="flex items-center gap-2">
        <KeyRound className="size-4 text-slate-300" aria-hidden="true" />
        <p className="text-sm font-medium text-white">{title}</p>
      </div>
      <div className="mt-4 grid gap-5 sm:grid-cols-2">
        <Field id={`${clientIdField}-id`} label="Client ID">
          <Input
            id={`${clientIdField}-id`}
            autoComplete="off"
            spellCheck={false}
            {...register(clientIdField)}
          />
        </Field>
        <Field id={`${clientSecretField}-id`} label="Client secret">
          <Input
            id={`${clientSecretField}-id`}
            type="password"
            autoComplete="new-password"
            {...register(clientSecretField)}
          />
        </Field>
      </div>
      <p className="mt-3 text-xs leading-5 text-slate-500">
        Register this callback URL with the provider:
        <code className="mx-1 break-all text-slate-300">{callbackURL}</code>
      </p>
    </div>
  );
}
