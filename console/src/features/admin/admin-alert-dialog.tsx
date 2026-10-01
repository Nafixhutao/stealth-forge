"use client";

import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { useCreateAdminAlert } from "@/api/mutations";
import {
  CreateAdminAlertRuleRequestKind,
  CreateAdminAlertRuleRequestSeverity,
  type components,
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
import { FormField } from "@/components/form-field";

type CreateAlertRequest = components["schemas"]["CreateAdminAlertRuleRequest"];

export function CreateAlertDialog({
  monitors,
  onCreated,
}: {
  monitors: components["schemas"]["AdminMonitor"][];
  onCreated: () => void;
}) {
  const mutation = useCreateAdminAlert();
  const [name, setName] = useState("");
  const [monitorId, setMonitorId] = useState("");
  const [kind, setKind] = useState<CreateAlertRequest["kind"]>(
    CreateAdminAlertRuleRequestKind.monitor_failure,
  );
  const [severity, setSeverity] = useState<CreateAlertRequest["severity"]>(
    CreateAdminAlertRuleRequestSeverity.warning,
  );
  const [forSeconds, setForSeconds] = useState(0);
  const [certificateDays, setCertificateDays] = useState(7);
  const [operator, setOperator] = useState("gt");
  const [threshold, setThreshold] = useState(0.05);
  const [windowSeconds, setWindowSeconds] = useState(300);
  const [metric, setMetric] = useState("system.cpu.utilization");
  const [service, setService] = useState("");
  const [aggregation, setAggregation] = useState("avg");
  const [percentile, setPercentile] = useState("p95");
  const [search, setSearch] = useState("");
  const [level, setLevel] = useState("ERROR");
  const requiresMonitor =
    kind === CreateAdminAlertRuleRequestKind.monitor_failure ||
    kind === CreateAdminAlertRuleRequestKind.heartbeat_failure ||
    kind === CreateAdminAlertRuleRequestKind.certificate_expiry;
  const availableMonitors = useMemo(
    () =>
      monitors.filter((monitor) =>
        kind === CreateAdminAlertRuleRequestKind.certificate_expiry
          ? monitor.kind === "tls"
          : kind === CreateAdminAlertRuleRequestKind.heartbeat_failure
            ? monitor.kind === "heartbeat"
            : true,
      ),
    [kind, monitors],
  );

  function resetKindFields(nextKind: CreateAlertRequest["kind"]) {
    setKind(nextKind);
    setMonitorId("");
    setThreshold(
      nextKind === CreateAdminAlertRuleRequestKind.disk_pressure ? 0.9 : 0.05,
    );
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const condition = requiresMonitor
      ? kind === CreateAdminAlertRuleRequestKind.certificate_expiry
        ? { monitor_id: monitorId, days: certificateDays }
        : { monitor_id: monitorId }
      : {
          operator,
          threshold,
          window_seconds: windowSeconds,
          ...(kind === CreateAdminAlertRuleRequestKind.metric_threshold ||
          kind === CreateAdminAlertRuleRequestKind.disk_pressure
            ? { metric }
            : {}),
          ...(service ? { service } : {}),
          ...(kind === CreateAdminAlertRuleRequestKind.metric_threshold
            ? { aggregation }
            : {}),
          ...(kind === CreateAdminAlertRuleRequestKind.latency
            ? { percentile }
            : {}),
          ...(kind === CreateAdminAlertRuleRequestKind.log_match
            ? { search, level }
            : {}),
        };
    mutation.mutate(
      {
        name,
        kind,
        condition,
        severity,
        for_seconds: forSeconds,
        enabled: true,
      },
      { onSuccess: onCreated },
    );
  }

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Add {alertKindLabel(kind).toLowerCase()} rule</DialogTitle>
        <DialogDescription>
          The rule is evaluated in the same transaction as the monitor result
          and becomes firing only after the optional for-duration.
        </DialogDescription>
      </DialogHeader>
      <form className="space-y-4" onSubmit={submit}>
        <FormField label="Name" htmlFor="alert-name">
          <Input
            id="alert-name"
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Public API unavailable"
          />
        </FormField>
        <FormField label="Rule type" htmlFor="alert-kind">
          <select
            id="alert-kind"
            value={kind}
            onChange={(event) => {
              resetKindFields(event.target.value as CreateAlertRequest["kind"]);
            }}
            className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3.5 text-sm text-mist outline-none focus:border-acid-lime/70 focus:ring-2 focus:ring-acid-lime/15"
          >
            <option value={CreateAdminAlertRuleRequestKind.metric_threshold}>
              Metric threshold
            </option>
            <option value={CreateAdminAlertRuleRequestKind.error_rate}>
              HTTP error rate
            </option>
            <option value={CreateAdminAlertRuleRequestKind.latency}>
              HTTP latency
            </option>
            <option value={CreateAdminAlertRuleRequestKind.log_match}>
              Log match
            </option>
            <option value={CreateAdminAlertRuleRequestKind.service_health}>
              Service health
            </option>
            <option value={CreateAdminAlertRuleRequestKind.disk_pressure}>
              Disk pressure
            </option>
            <option value={CreateAdminAlertRuleRequestKind.monitor_failure}>
              Monitor failure
            </option>
            <option value={CreateAdminAlertRuleRequestKind.heartbeat_failure}>
              Heartbeat missing
            </option>
            <option value={CreateAdminAlertRuleRequestKind.certificate_expiry}>
              Certificate expiry
            </option>
          </select>
        </FormField>
        {requiresMonitor ? (
          <FormField label="Monitor" htmlFor="alert-monitor">
            <select
              id="alert-monitor"
              required
              value={monitorId}
              onChange={(event) => setMonitorId(event.target.value)}
              className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3.5 text-sm text-mist outline-none focus:border-acid-lime/70 focus:ring-2 focus:ring-acid-lime/15"
            >
              <option value="">Select a monitor</option>
              {availableMonitors.map((monitor) => (
                <option key={monitor.id} value={monitor.id}>
                  {monitor.name} · {monitor.kind}
                </option>
              ))}
            </select>
            {!availableMonitors.length ? (
              <p className="mt-1 text-[11px] text-fog">
                {kind === CreateAdminAlertRuleRequestKind.certificate_expiry
                  ? "Create a TLS monitor first."
                  : kind === CreateAdminAlertRuleRequestKind.heartbeat_failure
                    ? "Create a heartbeat monitor first."
                    : "Create a monitor first."}
              </p>
            ) : null}
          </FormField>
        ) : null}
        {kind === CreateAdminAlertRuleRequestKind.certificate_expiry ? (
          <FormField label="Alert when days remain below" htmlFor="alert-days">
            <Input
              id="alert-days"
              type="number"
              min={1}
              max={3650}
              value={certificateDays}
              onChange={(event) =>
                setCertificateDays(Number(event.target.value))
              }
              required
            />
          </FormField>
        ) : null}
        {!requiresMonitor ? (
          <div className="space-y-4 rounded-md border border-graphite bg-void/40 p-3">
            {kind === CreateAdminAlertRuleRequestKind.metric_threshold ? (
              <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_160px]">
                <FormField
                  label="Metric name"
                  htmlFor="alert-metric"
                  hint="Exact OTel metric name from the instance."
                >
                  <Input
                    id="alert-metric"
                    required
                    value={metric}
                    onChange={(event) => setMetric(event.target.value)}
                  />
                </FormField>
                <FormField label="Aggregation" htmlFor="alert-aggregation">
                  <select
                    id="alert-aggregation"
                    value={aggregation}
                    onChange={(event) => setAggregation(event.target.value)}
                    className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3 text-sm text-mist outline-none focus:border-acid-lime/70"
                  >
                    <option value="avg">Average</option>
                    <option value="latest">Latest</option>
                    <option value="max">Maximum</option>
                    <option value="min">Minimum</option>
                    <option value="sum">Sum</option>
                  </select>
                </FormField>
              </div>
            ) : null}
            {kind === CreateAdminAlertRuleRequestKind.disk_pressure ? (
              <FormField
                label="Metric name"
                htmlFor="alert-disk-metric"
                hint="Defaults to system.filesystem.utilization."
              >
                <Input
                  id="alert-disk-metric"
                  value={metric}
                  onChange={(event) => setMetric(event.target.value)}
                />
              </FormField>
            ) : null}
            {kind === CreateAdminAlertRuleRequestKind.log_match ? (
              <div className="grid gap-4 sm:grid-cols-2">
                <FormField label="Search text" htmlFor="alert-search">
                  <Input
                    id="alert-search"
                    required
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    placeholder="database unavailable"
                  />
                </FormField>
                <FormField label="Level" htmlFor="alert-level">
                  <Input
                    id="alert-level"
                    value={level}
                    onChange={(event) => setLevel(event.target.value)}
                    placeholder="ERROR"
                  />
                </FormField>
              </div>
            ) : null}
            {kind === CreateAdminAlertRuleRequestKind.service_health ||
            kind === CreateAdminAlertRuleRequestKind.error_rate ||
            kind === CreateAdminAlertRuleRequestKind.latency ? (
              <FormField
                label={
                  kind === CreateAdminAlertRuleRequestKind.service_health
                    ? "Service"
                    : "Service filter"
                }
                htmlFor="alert-service"
              >
                <Input
                  id="alert-service"
                  required={
                    kind === CreateAdminAlertRuleRequestKind.service_health
                  }
                  value={service}
                  onChange={(event) => setService(event.target.value)}
                  placeholder="stealth-api"
                />
              </FormField>
            ) : null}
            {kind === CreateAdminAlertRuleRequestKind.latency ? (
              <FormField label="Percentile" htmlFor="alert-percentile">
                <select
                  id="alert-percentile"
                  value={percentile}
                  onChange={(event) => setPercentile(event.target.value)}
                  className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3 text-sm text-mist outline-none focus:border-acid-lime/70"
                >
                  <option value="p50">p50</option>
                  <option value="p95">p95</option>
                  <option value="p99">p99</option>
                </select>
              </FormField>
            ) : null}
            <div className="grid gap-4 sm:grid-cols-3">
              <FormField label="Operator" htmlFor="alert-operator">
                <select
                  id="alert-operator"
                  value={operator}
                  onChange={(event) => setOperator(event.target.value)}
                  className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3 text-sm text-mist outline-none focus:border-acid-lime/70"
                >
                  <option value="gt">Greater than</option>
                  <option value="gte">At least</option>
                  <option value="lt">Less than</option>
                  <option value="lte">At most</option>
                </select>
              </FormField>
              <FormField
                label={
                  kind === CreateAdminAlertRuleRequestKind.latency
                    ? "Threshold (ms)"
                    : "Threshold"
                }
                htmlFor="alert-threshold"
              >
                <Input
                  id="alert-threshold"
                  type="number"
                  step="any"
                  value={threshold}
                  onChange={(event) => setThreshold(Number(event.target.value))}
                  required
                />
              </FormField>
              <FormField label="Window (seconds)" htmlFor="alert-window">
                <Input
                  id="alert-window"
                  type="number"
                  min={30}
                  max={86400}
                  value={windowSeconds}
                  onChange={(event) =>
                    setWindowSeconds(Number(event.target.value))
                  }
                  required
                />
              </FormField>
            </div>
            <p className="text-[11px] leading-5 text-fog">
              Error-rate and health thresholds use a fraction from 0 to 1.
              Measurements are evaluated only when the selected telemetry signal
              has samples.
            </p>
          </div>
        ) : null}
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Severity" htmlFor="alert-severity">
            <select
              id="alert-severity"
              value={severity}
              onChange={(event) =>
                setSeverity(
                  event.target.value as CreateAlertRequest["severity"],
                )
              }
              className="min-h-11 w-full rounded-md border border-graphite bg-carbon px-3.5 text-sm text-mist outline-none focus:border-acid-lime/70 focus:ring-2 focus:ring-acid-lime/15"
            >
              <option value="info">Info</option>
              <option value="warning">Warning</option>
              <option value="critical">Critical</option>
            </select>
          </FormField>
          <FormField label="For (seconds)" htmlFor="alert-for">
            <Input
              id="alert-for"
              type="number"
              min={0}
              max={86400}
              value={forSeconds}
              onChange={(event) => setForSeconds(Number(event.target.value))}
            />
          </FormField>
        </div>
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
          <Button
            type="submit"
            disabled={
              mutation.isPending ||
              (requiresMonitor && (!availableMonitors.length || !monitorId))
            }
          >
            {mutation.isPending ? "Creating…" : "Create rule"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}

function alertKindLabel(kind: CreateAlertRequest["kind"]) {
  switch (kind) {
    case CreateAdminAlertRuleRequestKind.metric_threshold:
      return "metric threshold";
    case CreateAdminAlertRuleRequestKind.error_rate:
      return "HTTP error rate";
    case CreateAdminAlertRuleRequestKind.latency:
      return "HTTP latency";
    case CreateAdminAlertRuleRequestKind.log_match:
      return "log match";
    case CreateAdminAlertRuleRequestKind.service_health:
      return "service health";
    case CreateAdminAlertRuleRequestKind.disk_pressure:
      return "disk pressure";
    case CreateAdminAlertRuleRequestKind.heartbeat_failure:
      return "heartbeat missing";
    case CreateAdminAlertRuleRequestKind.certificate_expiry:
      return "certificate expiry";
    default:
      return "monitor failure";
  }
}
