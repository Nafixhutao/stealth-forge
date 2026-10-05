"use client";

import type { ReactNode } from "react";
import { PROVIDERS, MODEL_USAGE } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody, Mono } from "../components/admin-panel";
import { AdminPanel, AdminPanelHeader } from "../components/admin-panel";
import { ServiceStatusBadge } from "../components/domain-badges";

/**
 * Models & Providers — provider endpoints with health plus the per-model
 * usage/cost table.
 */
export function ProvidersPage() {
  return (
    <AdminPageBody>
      <AdminHeader title="Models & Providers" subtitle="Provider endpoints, model catalog, and usage economics." />

      <div className="grid gap-3 md:grid-cols-3">
        {PROVIDERS.map((provider) => (
          <AdminPanel key={provider.id}>
            <div className="flex items-center gap-2.5">
              <Mono className="text-[14px] font-semibold text-[var(--projects-text)]">{provider.name}</Mono>
              <span className="ml-auto">
                <ServiceStatusBadge status={provider.status} />
              </span>
            </div>
            <dl className="m-0 mt-3 grid grid-cols-3 gap-2">
              <Stat label="Latency" value={provider.latency} />
              <Stat label="Requests" value={provider.requestsToday} hint="today" />
              <Stat label="Uptime" value={provider.uptime} />
            </dl>
            <div className="mt-3 flex flex-wrap gap-1.5">
              {provider.models.map((model) => (
                <Mono
                  key={model}
                  className="rounded-md border border-[var(--projects-border)] px-1.5 py-0.5 text-[11px] text-[var(--projects-muted)]"
                >
                  {model}
                </Mono>
              ))}
            </div>
          </AdminPanel>
        ))}
      </div>

      <AdminPanel flush>
        <div className="px-4 pb-3 pt-4">
          <AdminPanelHeader title="Model usage · today" subtitle="Requests, tokens, latency, errors, and mock spend per model." />
        </div>
        <div className="admin-scrollbar overflow-x-auto">
          <table className="w-full min-w-[640px] border-collapse text-left">
            <thead>
              <tr className="border-y border-[var(--projects-divider)] bg-[var(--projects-control)]">
                <Th>Model</Th>
                <Th>Provider</Th>
                <Th>Requests</Th>
                <Th>Tokens</Th>
                <Th>Avg latency</Th>
                <Th>Error rate</Th>
                <Th>Cost today</Th>
              </tr>
            </thead>
            <tbody>
              {MODEL_USAGE.map((model) => (
                <tr key={model.id} className="border-b border-[var(--projects-divider)] last:border-b-0 hover:bg-white/[0.02]">
                  <Td>
                    <Mono className="text-[12px] font-medium text-[var(--projects-text)]">{model.model}</Mono>
                  </Td>
                  <Td>
                    <Mono className="text-[11.5px] text-[#b3b0ba]">{model.provider}</Mono>
                  </Td>
                  <Td>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)]">{model.requests}</Mono>
                  </Td>
                  <Td>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)]">{model.tokens}</Mono>
                  </Td>
                  <Td>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)]">{model.avgLatency}</Mono>
                  </Td>
                  <Td>
                    <Mono
                      className={
                        Number.parseFloat(model.errorRate) > 0.4
                          ? "text-[11.5px] text-[var(--projects-warning)]"
                          : "text-[11.5px] text-[var(--projects-muted)]"
                      }
                    >
                      {model.errorRate}
                    </Mono>
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

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="min-w-0">
      <dt className="m-0 text-[10.5px] uppercase tracking-[0.06em] text-[var(--projects-muted)]">{label}</dt>
      <dd className="m-0 truncate text-[13px] font-medium text-[var(--projects-text)]">
        <Mono>
          {value}
          {hint && <span className="ml-1 text-[10.5px] text-[var(--projects-muted)]">{hint}</span>}
        </Mono>
      </dd>
    </div>
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
