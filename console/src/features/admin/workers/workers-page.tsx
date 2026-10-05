"use client";

import { useEffect, useState, type ReactNode } from "react";
import { WORKERS, WORKER_CPU_HISTORY } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody, Mono } from "../components/admin-panel";
import { AdminPanel, AdminPanelHeader } from "../components/admin-panel";
import { ResourceBar } from "../components/resource-bar";
import { Sparkline } from "../components/sparkline";
import { StatusBadge } from "../components/status-badge";
import { nudge, useLiveTick } from "../hooks/use-live-updates";

/**
 * Workers — dedicated queue-consumer view: fleet tiles, a live CPU table,
 * and per-worker detail (selection) with CPU history.
 */
export function WorkersPage() {
  const [selectedId, setSelectedId] = useState<string | null>("wk-01");
  const [cpuValues, setCpuValues] = useState<Record<string, number>>(() =>
    Object.fromEntries(WORKERS.map((worker) => [worker.id, worker.cpu])),
  );
  const tick = useLiveTick(4000);

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

  const online = WORKERS.filter((worker) => worker.status !== "offline").length;
  const totalJobs = WORKERS.reduce((sum, worker) => sum + worker.jobs, 0);
  const queueDepth = WORKERS.reduce((sum, worker) => sum + worker.queue, 0);
  const selected = WORKERS.find((worker) => worker.id === selectedId) ?? null;

  return (
    <AdminPageBody>
      <AdminHeader title="Workers" subtitle="Queue consumers that execute agent runs inside sandboxes." />

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Tile label="Workers online" value={`${online} / ${WORKERS.length}`} tone="success" />
        <Tile label="Active jobs" value={String(totalJobs)} />
        <Tile label="Queue depth" value={String(queueDepth)} tone={queueDepth > 4 ? "warning" : "neutral"} />
        <Tile label="Heartbeat interval" value="5 s" />
      </div>

      <AdminPanel>
        <AdminPanelHeader title="Fleet" subtitle="Select a worker for detail." />
        <div
          aria-hidden="true"
          className="hidden grid-cols-[minmax(0,1fr)_110px_minmax(0,1.2fr)_90px_90px_110px] gap-3 border-b border-[var(--projects-divider)] px-3.5 pb-2 lg:grid"
        >
          <Col>Worker</Col>
          <Col>Status</Col>
          <Col>CPU</Col>
          <Col>Jobs</Col>
          <Col>Queue</Col>
          <Col>Heartbeat</Col>
        </div>
        <ul className="m-0 list-none p-0">
          {WORKERS.map((worker) => {
            const cpu = cpuValues[worker.id] ?? worker.cpu;
            const memoryPct = (Number.parseInt(worker.memoryUsed, 10) / 8) * 100;
            return (
              <li key={worker.id} className="border-b border-[var(--projects-divider)] last:border-b-0">
                <button
                  type="button"
                  onClick={() => setSelectedId((prev) => (prev === worker.id ? null : worker.id))}
                  aria-expanded={selectedId === worker.id}
                  className="block w-full px-3.5 py-2.5 text-left transition-colors hover:bg-white/[0.03]"
                >
                  <span className="grid grid-cols-1 gap-2 lg:grid-cols-[minmax(0,1fr)_110px_minmax(0,1.2fr)_90px_90px_110px] lg:items-center lg:gap-3">
                    <span className="flex items-center gap-2.5">
                      <Mono className="text-[13px] font-medium text-[var(--projects-text)]">{worker.name}</Mono>
                      <Mono className="text-[11px] text-[var(--projects-muted)]">{worker.host}</Mono>
                    </span>
                    <span>
                      <StatusBadge
                        tone={worker.status === "busy" ? "warning" : "success"}
                        label={worker.status === "busy" ? "Busy" : "Online"}
                        pulse
                      />
                    </span>
                    <span className="hidden lg:block">
                      <ResourceBar label="CPU" value={cpu} />
                    </span>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)] lg:text-[12px] lg:text-[var(--projects-text)]">
                      {worker.jobs}
                    </Mono>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)] lg:text-[12px]">
                      {worker.queue}
                    </Mono>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)]">♥ {worker.heartbeat}</Mono>
                    {/* compact CPU bar */}
                    <span className="lg:hidden">
                      <ResourceBar label="CPU" value={cpu} />
                      <span className="mt-2 block">
                        <ResourceBar label="RAM" value={memoryPct} detail={worker.memoryUsed} />
                      </span>
                    </span>
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      </AdminPanel>

      {selected && (
        <AdminPanel>
          <AdminPanelHeader
            title={`worker detail · ${selected.name}`}
            subtitle={`host ${selected.host} · uptime ${selected.uptime} · ${selected.jobs} active jobs`}
          />
          <div className="grid gap-4 lg:grid-cols-2">
            <div>
              <p className="m-0 mb-1 text-[10.5px] font-medium uppercase tracking-[0.07em] text-[var(--projects-muted)]">
                CPU · last 30 min
              </p>
              <Sparkline data={WORKER_CPU_HISTORY[selected.id] ?? []} tone={selected.status === "busy" ? "warning" : "accent"} height={64} />
            </div>
            <div className="grid grid-cols-2 gap-x-4 gap-y-2 self-center">
              <Meta label="CPU now" value={`${cpuValues[selected.id] ?? selected.cpu}%`} />
              <Meta label="RAM" value={selected.memoryUsed} />
              <Meta label="Current run" value={selected.currentRun ?? "—"} />
              <Meta label="Queue" value={String(selected.queue)} />
            </div>
          </div>
        </AdminPanel>
      )}
    </AdminPageBody>
  );
}

function Tile({ label, value, tone = "neutral" }: { label: string; value: string; tone?: "neutral" | "success" | "warning" }) {
  const valueClass =
    tone === "success" ? "text-[var(--projects-accent)]" : tone === "warning" ? "text-[var(--projects-warning)]" : "text-[var(--projects-text)]";
  return (
    <article className="rounded-lg border border-[var(--projects-border)] bg-[#141416] px-3.5 py-3">
      <p className="m-0 text-[11px] leading-4 text-[var(--projects-muted)]">{label}</p>
      <p className={`m-0 text-[17px] font-semibold leading-6 ${valueClass}`}>{value}</p>
    </article>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="m-0 text-[10.5px] uppercase tracking-[0.06em] text-[var(--projects-muted)]">{label}</p>
      <p className="m-0 truncate text-[13px] font-medium text-[var(--projects-text)]">
        <Mono>{value}</Mono>
      </p>
    </div>
  );
}

function Col({ children }: { children: ReactNode }) {
  return (
    <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">{children}</span>
  );
}
