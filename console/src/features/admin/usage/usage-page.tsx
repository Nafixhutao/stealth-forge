"use client";

import type { ReactNode } from "react";
import { Bot, Banknote, Coins, Boxes } from "lucide-react";
import { USAGE_STATS, USAGE_RUNS_SERIES, USAGE_TOKENS_SERIES, MODEL_USAGE } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody } from "../components/admin-panel";
import { AdminPanel, AdminPanelHeader } from "../components/admin-panel";
import { StatTile } from "../components/stat-tile";
import { TelemetryChart } from "../components/telemetry-chart";
import { Mono } from "../components/admin-panel";

const STAT_ICONS = { "usage-runs": Bot, "usage-tokens": Coins, "usage-sandbox": Boxes, "usage-spend": Banknote } as const;

/**
 * Usage — platform consumption over the last 14 days: runs, tokens, and
 * mock spend, with a per-model breakdown table.
 */
export function UsagePage() {
  return (
    <AdminPageBody>
      <AdminHeader title="Usage" subtitle="Agent, token, and sandbox consumption across the platform." />

      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        {USAGE_STATS.map((stat) => (
          <StatTile key={stat.id} icon={STAT_ICONS[stat.id as keyof typeof STAT_ICONS]} label={stat.label} value={stat.value} />
        ))}
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <AdminPanel>
          <AdminPanelHeader title="Agent runs per day" subtitle="Completed + failed runs, last 14 days." />
          <TelemetryChart
            data={USAGE_RUNS_SERIES}
            series={[{ key: "value", label: "Runs", tone: "accent" }]}
            unit=""
            height={200}
          />
        </AdminPanel>
        <AdminPanel>
          <AdminPanelHeader title="Tokens per day" subtitle="Millions of tokens processed, last 14 days." />
          <TelemetryChart
            data={USAGE_TOKENS_SERIES}
            series={[{ key: "value", label: "Tokens", tone: "info" }]}
            unit=" M"
            height={200}
          />
        </AdminPanel>
      </div>

      <AdminPanel flush>
        <div className="px-4 pb-3 pt-4">
          <AdminPanelHeader title="By model · today" subtitle="Where consumption actually lands." />
        </div>
        <div className="admin-scrollbar overflow-x-auto">
          <table className="w-full min-w-[620px] border-collapse text-left">
            <thead>
              <tr className="border-y border-[var(--projects-divider)] bg-[var(--projects-control)]">
                <Th>Model</Th>
                <Th>Requests</Th>
                <Th>Tokens</Th>
                <Th>Cost today</Th>
              </tr>
            </thead>
            <tbody>
              {MODEL_USAGE.map((model) => (
                <tr key={model.id} className="border-b border-[var(--projects-divider)] last:border-b-0 hover:bg-white/[0.02]">
                  <Td>
                    <Mono className="text-[12px] font-medium text-[var(--projects-text)]">{model.model}</Mono>
                    <span className="ml-2 text-[11px] text-[var(--projects-muted)]">{model.provider}</span>
                  </Td>
                  <Td>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)]">{model.requests}</Mono>
                  </Td>
                  <Td>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)]">{model.tokens}</Mono>
                  </Td>
                  <Td>
                    <Mono className="text-[11.5px] text-[var(--projects-text)]">{model.costToday}</Mono>
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </AdminPanel>
    </AdminPageBody>
  );
}

function Th({ children }: { children: ReactNode }) {
  return (
    <th scope="col" className="px-3.5 py-2 text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">
      {children}
    </th>
  );
}

function Td({ children }: { children: ReactNode }) {
  return <td className="px-3.5 py-2.5">{children}</td>;
}
