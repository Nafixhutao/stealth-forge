"use client";

import { Input } from "@/components/ui/input";
import { Field, StageActions, StageHeader } from "../primitives";
import type { SetupFlow } from "../types";

export function InstanceStage({ flow }: { flow: SetupFlow }) {
  const { configForm, isPending, moveTo, persistConfig, setActionError } = flow;
  return (
    <form
      onSubmit={configForm.handleSubmit(async (values) => {
        setActionError(undefined);
        try {
          await persistConfig(values);
          moveTo("github");
        } catch (error) {
          setActionError(error);
        }
      })}
    >
      <StageHeader
        eyebrow="01 / Instance"
        title="Name and locate the instance"
        description="Choose the name shown to operators and the URL the production Console will use."
      />
      <div className="space-y-5">
        <Field
          id="instance-name"
          label="Instance name"
          hint="This label stays inside your installation."
          error={configForm.formState.errors.instance_name?.message}
        >
          <Input id="instance-name" {...configForm.register("instance_name")} />
        </Field>
        <Field
          id="public-url"
          label="Public Console URL"
          hint="Use the final URL, without a path, query, or fragment."
          error={configForm.formState.errors.public_url?.message}
        >
          <Input
            id="public-url"
            type="url"
            placeholder="https://console.example.com"
            {...configForm.register("public_url")}
          />
        </Field>
      </div>
      <StageActions
        back={() => moveTo("welcome")}
        next={() =>
          void configForm.handleSubmit(async (values) => {
            try {
              await persistConfig(values);
              moveTo("github");
            } catch (error) {
              setActionError(error);
            }
          })()
        }
        nextLabel="Save and connect GitHub"
        pending={isPending}
      />
    </form>
  );
}
