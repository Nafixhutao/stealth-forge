"use client";

import type { ReactNode } from "react";
import { Copy } from "lucide-react";
import type { components } from "@/api/generated/schema";
import { StatusBadge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { formatDate, formatDuration } from "@/lib/format";
import type { MonitorKind } from "./admin-monitor-model";

export function HeartbeatTokenNotice({
  token,
  endpoint,
}: {
  token: string;
  endpoint: string | undefined;
}) {
  return (
    <div className="space-y-4 rounded-md border border-acid-lime/25 bg-acid-lime/[0.06] p-4">
      <div>
        <p className="text-sm text-paper">Heartbeat monitor created</p>
        <p className="mt-1 text-sm leading-6 text-fog">
          Copy this token now. It is shown once and cannot be recovered from
          Stealth.
        </p>
      </div>
      <div className="flex items-center gap-2 rounded-md border border-graphite bg-carbon p-3">
        <code className="min-w-0 flex-1 break-all font-mono text-xs text-mist">
          {token}
        </code>
        <Button
          type="button"
          variant="secondary"
          size="icon"
          aria-label="Copy heartbeat token"
          onClick={() => void navigator.clipboard?.writeText(token)}
        >
          <Copy className="size-4" aria-hidden="true" />
        </Button>
      </div>
      <p className="break-all font-mono text-xs leading-5 text-fog">
        POST {endpoint || "the generated heartbeat endpoint"} with
        X-Stealth-Heartbeat: &lt;token&gt;.
      </p>
    </div>
  );
}

export function MonitorDetails({
  data,
}: {
  data: components["schemas"]["AdminMonitorResponse"];
}) {
  const monitor = data.monitor;
  return (
    <div className="space-y-5">
      <div className="grid gap-3 sm:grid-cols-3">
        <Detail label="Status">
          <StatusBadge status={monitor.enabled ? monitor.status : "paused"} />
        </Detail>
        <Detail label="Target">
          <span className="break-all font-mono text-xs text-mist">
            {monitor.target}
          </span>
        </Detail>
        <Detail label="Schedule">
          <span className="font-mono text-xs text-mist">
            every {monitor.interval_seconds}s · {monitor.timeout_ms}ms timeout
          </span>
        </Detail>
      </div>
      {monitor.last_error ? (
        <p className="rounded-md border border-coral-red/20 bg-coral-red/[0.04] p-3 text-sm text-coral-red">
          {monitor.last_error}
        </p>
      ) : null}
      <div className="overflow-hidden rounded-md border border-graphite">
        <table className="w-full text-left text-xs">
          <thead className="border-b border-graphite bg-white/[0.02] text-fog">
            <tr>
              <th className="px-3 py-2 font-medium">Checked</th>
              <th className="px-3 py-2 font-medium">Result</th>
              <th className="px-3 py-2 font-medium">Latency</th>
              <th className="px-3 py-2 font-medium">Details</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-graphite">
            {(data.checks ?? []).map((check) => (
              <tr key={check.id}>
                <td className="px-3 py-2 text-fog">
                  {formatDate(check.checked_at)}
                </td>
                <td className="px-3 py-2">
                  <StatusBadge status={check.success ? "healthy" : "failing"} />
                </td>
                <td className="px-3 py-2 font-mono text-mist">
                  {formatDuration(check.latency_ms)}
                </td>
                <td className="max-w-[240px] break-words px-3 py-2 text-fog">
                  {check.error ?? JSON.stringify(check.details)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!data.checks?.length ? (
          <p className="p-4 text-sm text-fog">
            No worker checks have completed yet.
          </p>
        ) : null}
      </div>
    </div>
  );
}

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-graphite bg-void/40 p-3">
      <p className="text-[11px] uppercase tracking-[0.1em] text-fog">{label}</p>
      <div className="mt-2">{children}</div>
    </div>
  );
}
export function MonitorSummary({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "neutral" | "success" | "error";
}) {
  const color =
    tone === "success"
      ? "text-pulse-green"
      : tone === "error"
        ? "text-coral-red"
        : "text-paper";
  return (
    <div className="rounded-xl border border-graphite bg-carbon p-4">
      <p className="text-xs uppercase tracking-[0.1em] text-fog">{label}</p>
      <p className={`mt-2 font-mono text-2xl tabular-nums ${color}`}>{value}</p>
    </div>
  );
}

export function targetPlaceholder(kind: MonitorKind) {
  switch (kind) {
    case "http":
      return "https://status.example.com/health";
    case "tcp":
    case "tls":
      return "db.example.com:5432";
    case "dns":
      return "example.com";
    case "heartbeat":
      return "nightly-backup";
    default:
      return "Target";
  }
}
