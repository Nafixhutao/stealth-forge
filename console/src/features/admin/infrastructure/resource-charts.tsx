"use client";

import { useEffect, useState } from "react";
import { INFRA_SERIES } from "../data/admin-mock-data";
import { AdminPanel } from "../components/admin-panel";
import { TelemetryChart } from "../components/telemetry-chart";
import { Mono } from "../components/admin-panel";
import { useLiveTick } from "../hooks/use-live-updates";
import type { MetricPoint } from "../types/telemetry";

/**
 * The 2-column (1 on mobile) grid of CPU / Memory / Disk / Network charts.
 * The newest sample of each series drifts every 4s for a live feel.
 */
export function ResourceCharts() {
  const [series, setSeries] = useState(() => INFRA_SERIES);
  const tick = useLiveTick(4000);

  useEffect(() => {
    if (tick === 0) return;
    setSeries((prev) =>
      prev.map((item) => {
        const last = item.data[item.data.length - 1];
        if (!last) return item;
        const values = item.data.map((point) => point.value);
        const min = Math.min(...values);
        const max = Math.max(...values);
        const span = Math.max(4, (max - min) * 0.12);
        const next = clamp(last.value + (Math.random() - 0.5) * 2 * span, min - span / 2, max + span / 2);
        const data: MetricPoint[] = item.data.map((point, index) =>
          index === item.data.length - 1 ? { ...point, value: round(next) } : point,
        );
        return { ...item, data, current: formatCurrent(item.id, next) };
      }),
    );
  }, [tick]);

  return (
    <div className="grid gap-4 md:grid-cols-2">
      {series.map((item) => (
        <AdminPanel key={item.id}>
          <div className="flex flex-wrap items-start justify-between gap-2 px-1 pb-2 pt-1">
            <div>
              <h3 className="m-0 text-[13.5px] font-semibold leading-5 text-[var(--projects-text)]">{item.label}</h3>
              <Mono className="m-0 mt-0.5 block text-[11px] leading-4 text-[var(--projects-muted)]">
                peak {item.peak} · avg {item.average}
              </Mono>
            </div>
            <Mono className="text-[19px] font-semibold leading-6 tracking-[-0.01em] text-[var(--projects-text)]">
              {item.current}
            </Mono>
          </div>
          <TelemetryChart
            data={item.data}
            series={[{ key: "value", label: item.label, tone: item.tone }]}
            unit={item.unit}
            height={148}
          />
        </AdminPanel>
      ))}
    </div>
  );
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function round(value: number) {
  return Math.round(value * 10) / 10;
}

function formatCurrent(id: string, value: number): string {
  if (id === "memory") return `${value.toFixed(1)} GB`;
  if (id === "network") return `${Math.round(value)} MB/s`;
  return `${Math.round(value)}%`;
}
