"use client";

import { Database, Wifi } from "lucide-react";
import { Input } from "@/components/ui/input";
import {
  DependencySection,
  Field,
  StageActions,
  StageHeader,
} from "../primitives";
import type { SetupFlow } from "../types";

export function DataStage({ flow }: { flow: SetupFlow }) {
  const {
    configForm,
    dataReady,
    databaseMode,
    moveTo,
    persistConfig,
    saveConfig,
    setActionError,
    state,
    redisMode,
    testDatabase,
    testDatabaseConnection,
    testRedis,
    testRedisConnection,
  } = flow;
  return (
    <>
      <StageHeader
        eyebrow="04 / Database & Redis"
        title="Choose durable dependencies"
        description="Bundled services are the portable default. External endpoints are never started by the setup Compose project and must pass a live test."
      />
      <div className="space-y-7">
        <DependencySection
          icon={Database}
          title="PostgreSQL"
          mode={databaseMode}
          onModeChange={(value) =>
            configForm.setValue("database_mode", value, {
              shouldDirty: true,
            })
          }
          tested={Boolean(state?.draft.database_tested)}
          testing={testDatabase.isPending}
          onTest={() => void testDatabaseConnection()}
        >
          <Field
            id="database-url"
            label="PostgreSQL URL"
            hint="The URL is kept server-side and is not returned in setup status."
          >
            <Input
              id="database-url"
              type="text"
              placeholder="postgresql://user:password@db.example.com/stealth"
              autoComplete="off"
              {...configForm.register("database_url")}
            />
          </Field>
        </DependencySection>
        <DependencySection
          icon={Wifi}
          title="Redis"
          mode={redisMode}
          onModeChange={(value) =>
            configForm.setValue("redis_mode", value, {
              shouldDirty: true,
            })
          }
          tested={Boolean(state?.draft.redis_tested)}
          testing={testRedis.isPending}
          onTest={() => void testRedisConnection()}
        >
          <Field
            id="redis-url"
            label="Redis URL"
            hint="Use rediss:// when the provider requires TLS."
          >
            <Input
              id="redis-url"
              type="text"
              placeholder="rediss://:password@redis.example.com:6379/0"
              autoComplete="off"
              {...configForm.register("redis_url")}
            />
          </Field>
        </DependencySection>
      </div>
      <StageActions
        back={() => moveTo("networking")}
        next={async () => {
          try {
            await persistConfig(configForm.getValues());
            moveTo("storage");
          } catch (error) {
            setActionError(error);
          }
        }}
        nextLabel="Save and configure storage"
        nextDisabled={!dataReady}
        pending={saveConfig.isPending}
      />
    </>
  );
}
