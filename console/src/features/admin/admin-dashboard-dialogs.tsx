"use client";

import { useState } from "react";
import type { FormEvent } from "react";
import { useCreateAdminDashboard } from "@/api/mutations";
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
import type { DashboardRequest, Panel } from "./admin-dashboard-model";

export function CreateDashboardDialog({
  onCreated,
}: {
  onCreated: (id: string) => void;
}) {
  const mutation = useCreateAdminDashboard();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [metric, setMetric] = useState("stealth_api_http_requests_total");
  const [service, setService] = useState("stealth-api");
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const body: DashboardRequest = {
      name,
      description,
      definition: {
        panels: [
          {
            id: "primary",
            type: "time_series",
            title: metric,
            metric,
            service,
          },
        ],
      },
    };
    mutation.mutate(body, {
      onSuccess: (response) => {
        if (response?.dashboard.id) onCreated(response.dashboard.id);
      },
    });
  }
  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>New dashboard</DialogTitle>
        <DialogDescription>
          Choose a real metric series for the first panel. Add more bounded
          metric, log, or monitor panels after the dashboard is created.
        </DialogDescription>
      </DialogHeader>
      <form className="space-y-4" onSubmit={submit}>
        <FormField label="Name" htmlFor="dashboard-name">
          <Input
            id="dashboard-name"
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Instance health"
          />
        </FormField>
        <FormField label="Description" htmlFor="dashboard-description">
          <Textarea
            id="dashboard-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </FormField>
        <FormField
          label="Metric name"
          htmlFor="dashboard-metric"
          hint="Use the exact OTel/Prometheus metric name emitted by this instance."
        >
          <Input
            id="dashboard-metric"
            required
            value={metric}
            onChange={(event) => setMetric(event.target.value)}
          />
        </FormField>
        <FormField label="Service filter" htmlFor="dashboard-service">
          <Input
            id="dashboard-service"
            value={service}
            onChange={(event) => setService(event.target.value)}
            placeholder="Optional"
          />
        </FormField>
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
            {mutation.isPending ? "Creating…" : "Create dashboard"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}

export function AddPanelDialog({ onAdd }: { onAdd: (panel: Panel) => void }) {
  const [type, setType] = useState<Panel["type"]>("time_series");
  const [title, setTitle] = useState("");
  const [metric, setMetric] = useState("system.cpu.utilization");
  const [service, setService] = useState("");
  const [level, setLevel] = useState("");
  const [query, setQuery] = useState("");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const panel: Panel = {
      id: `panel-${Date.now()}`,
      type,
      title: title.trim() || undefined,
      ...(type === "time_series" ? { metric, service } : {}),
      ...(type === "logs" ? { service, level, query } : {}),
    };
    onAdd(panel);
  }

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Add dashboard panel</DialogTitle>
        <DialogDescription>
          Panels use the authenticated, bounded admin queries. No raw ClickHouse
          SQL is accepted.
        </DialogDescription>
      </DialogHeader>
      <form className="space-y-4" onSubmit={submit}>
        <FormField label="Panel type" htmlFor="dashboard-panel-type">
          <select
            id="dashboard-panel-type"
            value={type}
            onChange={(event) => setType(event.target.value)}
            className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3.5 text-sm text-mist outline-none focus:border-acid-lime/70"
          >
            <option value="time_series">Metric time series</option>
            <option value="logs">Recent logs</option>
            <option value="monitor_status">Monitor status</option>
          </select>
        </FormField>
        <FormField label="Title" htmlFor="dashboard-panel-title">
          <Input
            id="dashboard-panel-title"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            placeholder="Optional"
          />
        </FormField>
        {type === "time_series" ? (
          <>
            <FormField
              label="Metric name"
              htmlFor="dashboard-panel-metric"
              hint="Use the exact OTel metric name emitted by this instance."
            >
              <Input
                id="dashboard-panel-metric"
                value={metric}
                onChange={(event) => setMetric(event.target.value)}
                required
              />
            </FormField>
            <FormField label="Service filter" htmlFor="dashboard-panel-service">
              <Input
                id="dashboard-panel-service"
                value={service}
                onChange={(event) => setService(event.target.value)}
                placeholder="Optional"
              />
            </FormField>
          </>
        ) : null}
        {type === "logs" ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <FormField label="Search text" htmlFor="dashboard-panel-query">
              <Input
                id="dashboard-panel-query"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Optional"
              />
            </FormField>
            <FormField label="Level" htmlFor="dashboard-panel-level">
              <Input
                id="dashboard-panel-level"
                value={level}
                onChange={(event) => setLevel(event.target.value)}
                placeholder="Optional"
              />
            </FormField>
            <FormField label="Service filter" htmlFor="dashboard-log-service">
              <Input
                id="dashboard-log-service"
                value={service}
                onChange={(event) => setService(event.target.value)}
                placeholder="Optional"
              />
            </FormField>
          </div>
        ) : null}
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="secondary">
              Cancel
            </Button>
          </DialogClose>
          <Button type="submit" disabled={type === "time_series" && !metric}>
            Add panel
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}
