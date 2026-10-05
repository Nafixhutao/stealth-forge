"use client";

import { useEffect, useState } from "react";
import { WORKERS, WORKER_CPU_HISTORY } from "../data/admin-mock-data";
import { AdminPanel, AdminPanelHeader, Mono } from "../components/admin-panel";
import { ResourceBar } from "../components/resource-bar";
import { Sparkline } from "../components/sparkline";
import { StatusBadge } from "../components/status-badge";
import { nudge, useLiveTick } from "../hooks/use-live-updates";
import type { Worker } from "../types/infrastructure";

/**
 * Agent worker fleet cards. Each card is selectable and reveals the
 * worker's current run, queue depth, and a live CPU history line.
 */
export function WorkersSection({ className }: { className?: string }) {
  const [selectedId, setSelectedId] = useState<string | null>("wk-01");
  const [cpuValues, setCpuValues] = useState<Record<string, number>>(() =>
    Object.fromEntries(WORKERS.map((worker) => [worker.id, worker.cpu])),
  );
  const tick = useLiveTick(5000);

  // Heartbeats keep arriving; CPU drifts a little on each pass.
  useEffect(() => {
    if (tick === 0) return;
    setCpuValues((prev) =>
      Object.fromEntries(
        Object.entries(prev).map(([id, value]) => {
          const base = WORKERS.find((worker) => worker.id === id);
          const [min, max] = base ? [Math.max(5, base.cpu - 18), Math.min(96, base.cpu + 18)] : [5, 96];
          return [id, Math.round(nudge(value, 2.4, min, max))];
        }),
      ),
    );
  }, [tick]);

  return (
    <AdminPanel className={className}>
      <AdminPanelHeader
        title="Agent Workers"
        subtitle="Queue consumers executing agent runs inside sandboxes."
        right={
          <Mono className="text-[11.5px] text-[var(--projects-muted)]">
            {WORKERS.length} / {WORKERS.length} online
          </Mono>
        }
      />
      <div className="grid gap-3 sm:grid-cols-2">
        {WORKERS.map((worker) => (
          <WorkerCard
            key={worker.id}
            worker={worker}
            cpu={cpuValues[worker.id] ?? worker.cpu}
            selected={worker.id === selectedId}
            onSelect={() => setSelectedId((prev) => (prev === worker.id ? null : worker.id))}
          />
        ))}
      </div>
    </AdminPanel>
  );
}

function WorkerCard({
  worker,
  cpu,
  selected,
  onSelect,
}: {
  worker: Worker;
  cpu: number;
  selected: boolean;
  onSelect: () => void;
}) {
  const statusLabel = worker.status === "busy" ? "Busy" : worker.status === "online" ? "Online" : "Offline";
  const memoryPct = (parseInt(worker.memoryUsed, 10) / 8) * 100;

  return (
    <article
      className={
        selected
          ? "rounded-lg border border-[color-mix(in_srgb,var(--projects-accent)_35%,var(--projects-border))] bg-white/[0.03] p-3.5"
          : "rounded-lg border border-[var(--projects-border)] bg-[var(--projects-control)] p-3.5 transition-colors hover:border-[var(--projects-border-hover)]"
      }
    >
      <button
        type="button"
        onClick={onSelect}
        aria-expanded={selected}
        className="flex w-full flex-wrap items-center gap-2 text-left"
      >
        <Mono className="text-[13px] font-medium text-[var(--projects-text)]">{worker.name}</Mono>
        <StatusBadge tone={worker.status === "busy" ? "warning" : "success"} label={statusLabel} pulse={worker.status !== "offline"} />
        <Mono className="ml-auto text-[11px] text-[var(--projects-muted)]">♥ {worker.heartbeat}</Mono>
      </button>

      <div className="mt-3 grid grid-cols-1 gap-x-5 gap-y-2 sm:grid-cols-2">
        <ResourceBar label="CPU" value={cpu} />
        <ResourceBar label="RAM" value={memoryPct} detail={worker.memoryUsed} />
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11.5px] text-[var(--projects-muted)]">
        <span>
          Jobs <Mono className="text-[var(--projects-text)]">{worker.jobs}</Mono>
        </span>
        <span>
          Queue <Mono className="text-[var(--projects-text)]">{worker.queue}</Mono>
        </span>
        {worker.currentRun && (
          <span>
            Run <Mono className="text-[var(--projects-text)]">{worker.currentRun}</Mono>
          </span>
        )}
      </div>

      {selected && (
        <div className="mt-3 border-t border-[var(--projects-divider)] pt-3">
          <p className="m-0 mb-1 text-[10.5px] font-medium uppercase tracking-[0.07em] text-[var(--projects-muted)]">
            CPU · last 30 min
          </p>
          <Sparkline data={WORKER_CPU_HISTORY[worker.id] ?? []} tone={worker.status === "busy" ? "warning" : "accent"} height={32} />
        </div>
      )}
    </article>
  );
}
