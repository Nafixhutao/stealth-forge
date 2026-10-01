"use client";

import { useState } from "react";
import type { FormEvent } from "react";
import { useCreateAdminMonitor } from "@/api/mutations";
import {
  CreateAdminMonitorRequestMethod,
  CreateAdminMonitorRequestRecord_type,
} from "@/api/generated/schema";
import { errorMessage } from "@/components/feedback/error-state";
import { Button } from "@/components/ui/button";
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { FormField } from "@/components/form-field";
import {
  initialForm,
  monitorKinds,
  type CreateMonitorRequest,
  type MonitorKind,
} from "./admin-monitor-model";
import {
  HeartbeatTokenNotice,
  targetPlaceholder,
} from "./admin-monitor-details";

export function CreateMonitorDialog({
  mutation,
  heartbeatToken,
  heartbeatEndpoint,
  onHeartbeatToken,
  onCreated,
}: {
  mutation: ReturnType<typeof useCreateAdminMonitor>;
  heartbeatToken: string | undefined;
  heartbeatEndpoint: string | undefined;
  onHeartbeatToken: (token: string, endpoint: string) => void;
  onCreated: () => void;
}) {
  const [form, setForm] = useState<CreateMonitorRequest>(initialForm);
  const [headersJSON, setHeadersJSON] = useState("");
  const [parseError, setParseError] = useState<string>();
  const kind = form.kind;
  const set = <K extends keyof CreateMonitorRequest>(
    key: K,
    value: CreateMonitorRequest[K],
  ) => setForm((current) => ({ ...current, [key]: value }));

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setParseError(undefined);
    let headers: Record<string, string> | undefined;
    if (headersJSON.trim()) {
      try {
        const value: unknown = JSON.parse(headersJSON);
        if (!value || typeof value !== "object" || Array.isArray(value)) {
          throw new Error("Headers must be a JSON object.");
        }
        headers = Object.fromEntries(
          Object.entries(value).map(([key, value]) => {
            if (typeof value !== "string")
              throw new Error("Header values must be strings.");
            return [key, value];
          }),
        );
      } catch (error) {
        mutation.reset();
        // The form stays open so a secret header is not lost to a validation
        // error. Only the local parse message is shown.
        setParseError(
          error instanceof Error ? error.message : "Headers are invalid.",
        );
        return;
      }
    }
    const body: CreateMonitorRequest = {
      ...form,
      headers,
      method: kind === "http" ? form.method : undefined,
      expected_status: form.expected_status ?? 200,
      body: kind === "http" ? form.body : undefined,
      body_contains: kind === "http" ? form.body_contains : undefined,
      host: kind === "tcp" || kind === "tls" ? form.host : undefined,
      port: kind === "tcp" || kind === "tls" ? form.port : undefined,
      record_type: kind === "dns" ? form.record_type : undefined,
      expected_values: kind === "dns" ? form.expected_values : undefined,
      grace_seconds: kind === "heartbeat" ? form.grace_seconds : undefined,
    };
    mutation.mutate(body, {
      onSuccess: (response) => {
        if (response?.heartbeat_token) {
          onHeartbeatToken(
            response.heartbeat_token,
            response.heartbeat_endpoint ?? "",
          );
        } else {
          onCreated();
        }
      },
    });
  }

  return (
    <DialogContent className="max-w-2xl">
      <DialogHeader>
        <DialogTitle>Add monitor</DialogTitle>
        <DialogDescription>
          The worker executes checks on a bounded schedule. Network targets are
          restricted to public addresses to reduce SSRF risk.
        </DialogDescription>
      </DialogHeader>
      {heartbeatToken ? (
        <HeartbeatTokenNotice
          token={heartbeatToken}
          endpoint={heartbeatEndpoint}
        />
      ) : (
        <form className="space-y-4" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <FormField label="Name" htmlFor="monitor-name">
              <Input
                id="monitor-name"
                required
                value={form.name ?? ""}
                onChange={(event) => set("name", event.target.value)}
                placeholder="Public API"
              />
            </FormField>
            <FormField label="Type" htmlFor="monitor-kind">
              <select
                id="monitor-kind"
                value={form.kind}
                onChange={(event) => {
                  const nextKind = event.target.value as MonitorKind;
                  setForm((current) => ({ ...current, kind: nextKind }));
                }}
                className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3.5 text-sm text-mist outline-none focus:border-acid-lime/70 focus:ring-2 focus:ring-acid-lime/15"
              >
                {monitorKinds.map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </FormField>
          </div>
          <FormField
            label="Target"
            htmlFor="monitor-target"
            hint={
              kind === "heartbeat"
                ? "A label for the job; the generated endpoint is shown after creation."
                : undefined
            }
          >
            <Input
              id="monitor-target"
              required
              value={form.target ?? ""}
              onChange={(event) => set("target", event.target.value)}
              placeholder={targetPlaceholder(kind)}
            />
          </FormField>
          <div className="grid gap-4 sm:grid-cols-2">
            <FormField label="Interval (seconds)" htmlFor="monitor-interval">
              <Input
                id="monitor-interval"
                type="number"
                min={5}
                max={86400}
                value={form.interval_seconds ?? 60}
                onChange={(event) =>
                  set("interval_seconds", Number(event.target.value))
                }
              />
            </FormField>
            <FormField label="Timeout (milliseconds)" htmlFor="monitor-timeout">
              <Input
                id="monitor-timeout"
                type="number"
                min={100}
                max={120000}
                value={form.timeout_ms ?? 5000}
                onChange={(event) =>
                  set("timeout_ms", Number(event.target.value))
                }
              />
            </FormField>
          </div>
          {kind === "http" ? (
            <div className="space-y-4 rounded-md border border-graphite bg-void/40 p-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <FormField label="Method" htmlFor="monitor-method">
                  <select
                    id="monitor-method"
                    value={form.method ?? "GET"}
                    onChange={(event) =>
                      set(
                        "method",
                        event.target.value as CreateAdminMonitorRequestMethod,
                      )
                    }
                    className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3.5 text-sm text-mist outline-none focus:border-acid-lime/70 focus:ring-2 focus:ring-acid-lime/15"
                  >
                    {["GET", "HEAD", "POST", "PUT", "PATCH", "OPTIONS"].map(
                      (method) => (
                        <option key={method} value={method}>
                          {method}
                        </option>
                      ),
                    )}
                  </select>
                </FormField>
                <FormField label="Expected status" htmlFor="monitor-status">
                  <Input
                    id="monitor-status"
                    type="number"
                    min={100}
                    max={599}
                    value={form.expected_status ?? 200}
                    onChange={(event) =>
                      set("expected_status", Number(event.target.value))
                    }
                  />
                </FormField>
              </div>
              <FormField
                label="Secret headers (JSON)"
                htmlFor="monitor-headers"
                hint="Write-only. Values are encrypted before persistence; they are not shown again."
              >
                <Textarea
                  id="monitor-headers"
                  value={headersJSON}
                  onChange={(event) => setHeadersJSON(event.target.value)}
                  placeholder={'{"Authorization":"Bearer …"}'}
                  className="min-h-20 font-mono text-xs"
                  autoComplete="off"
                />
              </FormField>
              <FormField
                label="Request body"
                htmlFor="monitor-body"
                hint="Write-only; max 1 MiB."
              >
                <Textarea
                  id="monitor-body"
                  value={form.body ?? ""}
                  onChange={(event) => set("body", event.target.value)}
                  className="min-h-20 font-mono text-xs"
                  autoComplete="off"
                />
              </FormField>
              <FormField
                label="Body contains"
                htmlFor="monitor-body-contains"
                hint="Write-only assertion; exact substring match."
              >
                <Input
                  id="monitor-body-contains"
                  value={form.body_contains ?? ""}
                  onChange={(event) => set("body_contains", event.target.value)}
                  autoComplete="off"
                />
              </FormField>
            </div>
          ) : null}
          {kind === "tcp" || kind === "tls" ? (
            <div className="grid gap-4 rounded-md border border-graphite bg-void/40 p-4 sm:grid-cols-[1fr_160px]">
              <FormField
                label="Host (optional)"
                htmlFor="monitor-host"
                hint="If omitted, use host:port in Target."
              >
                <Input
                  id="monitor-host"
                  value={form.host ?? ""}
                  onChange={(event) => set("host", event.target.value)}
                />
              </FormField>
              <FormField label="Port (optional)" htmlFor="monitor-port">
                <Input
                  id="monitor-port"
                  type="number"
                  min={1}
                  max={65535}
                  value={form.port ?? ""}
                  onChange={(event) =>
                    set("port", Number(event.target.value) || undefined)
                  }
                />
              </FormField>
            </div>
          ) : null}
          {kind === "dns" ? (
            <div className="grid gap-4 rounded-md border border-graphite bg-void/40 p-4 sm:grid-cols-2">
              <FormField label="Record type" htmlFor="monitor-record-type">
                <select
                  id="monitor-record-type"
                  value={
                    form.record_type ?? CreateAdminMonitorRequestRecord_type.A
                  }
                  onChange={(event) =>
                    set(
                      "record_type",
                      event.target
                        .value as CreateAdminMonitorRequestRecord_type,
                    )
                  }
                  className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3.5 text-sm text-mist outline-none focus:border-acid-lime/70 focus:ring-2 focus:ring-acid-lime/15"
                >
                  {["A", "AAAA", "CNAME", "TXT"].map((recordType) => (
                    <option key={recordType} value={recordType}>
                      {recordType}
                    </option>
                  ))}
                </select>
              </FormField>
              <FormField
                label="Expected values"
                htmlFor="monitor-expected-values"
                hint="Comma-separated; every value must be present, extra DNS records are allowed. Leave blank to assert lookup success only."
              >
                <Input
                  id="monitor-expected-values"
                  value={(form.expected_values ?? []).join(", ")}
                  onChange={(event) =>
                    set(
                      "expected_values",
                      event.target.value
                        .split(",")
                        .map((value) => value.trim())
                        .filter(Boolean),
                    )
                  }
                />
              </FormField>
            </div>
          ) : null}
          {kind === "heartbeat" ? (
            <FormField
              label="Grace period (seconds)"
              htmlFor="monitor-grace"
              hint="The token is generated after creation and shown once."
            >
              <Input
                id="monitor-grace"
                type="number"
                min={0}
                max={604800}
                value={form.grace_seconds ?? 0}
                onChange={(event) =>
                  set("grace_seconds", Number(event.target.value))
                }
              />
            </FormField>
          ) : null}
          {parseError ? (
            <p className="text-sm text-coral-red" role="alert">
              {parseError}
            </p>
          ) : null}
          {mutation.error ? (
            <p className="text-sm text-coral-red" role="alert">
              {errorMessage(mutation.error)}
            </p>
          ) : null}
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="secondary">
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? "Creating…" : "Create monitor"}
            </Button>
          </DialogFooter>
        </form>
      )}
    </DialogContent>
  );
}
