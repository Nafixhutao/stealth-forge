"use client";

import { useMemo, useState, type ReactNode } from "react";
import Link from "next/link";
import { CircleCheck, CircleX, LoaderCircle } from "lucide-react";
import { RUNS } from "../data/admin-mock-data";
import { AdminHeader, AdminPageBody, Mono } from "../components/admin-panel";
import { ToolbarSearch } from "../components/toolbar-search";
import { AdminSelect } from "../components/admin-select";
import { DetailDrawer, DetailField } from "../components/detail-drawer";
import { RunStatusBadge } from "../components/domain-badges";
import type { AgentRun, RunStep, RunStatus } from "../types/runs";

type StatusFilter = "all" | RunStatus;
type ProviderFilter = "all" | string;
type ModelFilter = "all" | string;

const PROVIDERS = ["OpenAI", "Anthropic"];
const MODELS = ["GPT-5.6", "GPT-5.6 mini", "Claude Sonnet 4.5", "Claude Haiku 4.5"];

/**
 * Agent Runs — platform-wide run history with filters and a per-run detail
 * drawer (steps, error, and mock cross-links to logs/traces/changes).
 */
export function RunsPage() {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<StatusFilter>("all");
  const [provider, setProvider] = useState<ProviderFilter>("all");
  const [model, setModel] = useState<ModelFilter>("all");
  const [selected, setSelected] = useState<AgentRun | null>(null);

  const visible = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return RUNS.filter((run) => {
      if (status !== "all" && run.status !== status) return false;
      if (provider !== "all" && run.provider !== provider) return false;
      if (model !== "all" && run.model !== model) return false;
      if (!normalizedQuery) return true;
      return [run.id, run.user, run.agent, run.repository].join(" ").toLowerCase().includes(normalizedQuery);
    });
  }, [query, status, provider, model]);

  return (
    <AdminPageBody>
      <AdminHeader title="Agent Runs" subtitle="Every agent run executed by the platform, with cost and outcome.">
        <Mono className="hidden h-9 items-center rounded-lg border border-[var(--projects-border)] bg-[#141416] px-3 text-[12px] text-[var(--projects-muted)] sm:inline-flex">
          {visible.length} / {RUNS.length} runs
        </Mono>
      </AdminHeader>

      <div className="flex flex-wrap items-center gap-2.5">
        <ToolbarSearch value={query} onChange={setQuery} placeholder="Search run, user, agent, repo..." label="Search runs" />
        <AdminSelect
          label="Filter by status"
          value={status}
          onChange={setStatus}
          options={[
            { value: "all", label: "All statuses" },
            { value: "running", label: "Running" },
            { value: "completed", label: "Completed" },
            { value: "failed", label: "Failed" },
            { value: "queued", label: "Queued" },
          ]}
        />
        <AdminSelect
          label="Filter by provider"
          value={provider}
          onChange={setProvider}
          options={[{ value: "all", label: "All providers" }, ...PROVIDERS.map((item) => ({ value: item, label: item }))]}
        />
        <AdminSelect
          label="Filter by model"
          value={model}
          onChange={setModel}
          search
          options={[{ value: "all", label: "All models" }, ...MODELS.map((item) => ({ value: item, label: item }))]}
        />
      </div>

      <div className="overflow-hidden rounded-lg border border-[var(--projects-border)] bg-[#141416]">
        <div
          aria-hidden="true"
          className="hidden grid-cols-[104px_70px_minmax(0,1.5fr)_88px_minmax(0,1.2fr)_86px_70px_84px_100px_76px] gap-3 border-b border-[var(--projects-divider)] px-3.5 py-2 xl:grid"
        >
          <ColumnLabel>Run</ColumnLabel>
          <ColumnLabel>User</ColumnLabel>
          <ColumnLabel>Agent</ColumnLabel>
          <ColumnLabel>Provider</ColumnLabel>
          <ColumnLabel>Model</ColumnLabel>
          <ColumnLabel>Tokens</ColumnLabel>
          <ColumnLabel>Cost</ColumnLabel>
          <ColumnLabel>Duration</ColumnLabel>
          <ColumnLabel>Status</ColumnLabel>
          <ColumnLabel>Started</ColumnLabel>
        </div>
        <ul className="m-0 list-none p-0">
          {visible.length === 0 ? (
            <li className="px-4 py-12 text-center text-[13px] text-[var(--projects-muted)]">
              No runs match the current filters.
            </li>
          ) : (
            visible.map((run) => (
              <li key={run.id} className="border-b border-[var(--projects-divider)] last:border-b-0">
                <button
                  type="button"
                  onClick={() => setSelected(run)}
                  aria-label={`Inspect run ${run.id}`}
                  className="block w-full px-3.5 py-2 text-left transition-colors hover:bg-white/[0.03]"
                >
                  <span className="hidden items-center gap-3 xl:grid xl:grid-cols-[104px_70px_minmax(0,1.5fr)_88px_minmax(0,1.2fr)_86px_70px_84px_100px_76px]">
                    <Mono className="text-[12px] font-medium text-[var(--projects-text)]">{run.id}</Mono>
                    <span className="truncate text-[12px] text-[var(--projects-muted)]">{run.user}</span>
                    <span className="truncate text-[12.5px] text-[var(--projects-text)]">{run.agent}</span>
                    <Mono className="text-[11.5px] text-[#b3b0ba]">{run.provider}</Mono>
                    <Mono className="truncate text-[11.5px] text-[#b3b0ba]">{run.model}</Mono>
                    <Mono className="text-[11px] text-[var(--projects-muted)]">
                      {run.tokensIn} / {run.tokensOut}
                    </Mono>
                    <Mono className="text-[11.5px] text-[var(--projects-text)]">{run.cost}</Mono>
                    <Mono className="text-[11.5px] text-[var(--projects-muted)]">{run.duration}</Mono>
                    <RunStatusBadge status={run.status} />
                    <Mono className="text-[11px] text-[var(--projects-muted)]">{run.startedAt}</Mono>
                  </span>
                  <span className="block xl:hidden">
                    <span className="flex items-center gap-2">
                      <Mono className="text-[12px] font-medium text-[var(--projects-text)]">{run.id}</Mono>
                      <span className="truncate text-[12px] text-[var(--projects-muted)]">{run.agent}</span>
                      <span className="ml-auto shrink-0">
                        <RunStatusBadge status={run.status} />
                      </span>
                    </span>
                    <Mono className="mt-1 block text-[11px] leading-4 text-[var(--projects-muted)]">
                      {run.user} · {run.model} · {run.tokensIn}/{run.tokensOut} tokens · {run.cost} ·{" "}
                      {run.duration === "—" ? "pending" : run.duration} · {run.startedAt}
                    </Mono>
                  </span>
                </button>
              </li>
            ))
          )}
        </ul>
      </div>

      <RunDetail run={selected} onClose={() => setSelected(null)} />
    </AdminPageBody>
  );
}

function RunDetail({ run, onClose }: { run: AgentRun | null; onClose: () => void }) {
  return (
    <DetailDrawer
      open={run !== null}
      onClose={onClose}
      title={
        <>
          <Mono className="truncate">{run?.id}</Mono>
          {run && <span className="ml-2"><RunStatusBadge status={run.status} /></span>}
        </>
      }
      subtitle={run ? `${run.user} · ${run.repository}` : undefined}
    >
      {run && (
        <div className="flex flex-col gap-4">
          <div className="grid grid-cols-2 gap-x-4">
            <DetailField label="User">{run.user}</DetailField>
            <DetailField label="Agent">{run.agent}</DetailField>
            <DetailField label="Model">{run.model}</DetailField>
            <DetailField label="Provider">{run.provider}</DetailField>
            <DetailField label="Duration">{run.duration}</DetailField>
            <DetailField label="Input tokens">{run.tokensIn}</DetailField>
            <DetailField label="Output tokens">{run.tokensOut}</DetailField>
            <DetailField label="Cost (mock)">{run.cost}</DetailField>
          </div>

          <div>
            <p className="m-0 mb-2 text-[11px] font-medium uppercase tracking-[0.06em] text-[var(--projects-muted)]">
              Steps
            </p>
            {run.steps.length === 0 ? (
              <p className="m-0 rounded-lg border border-[var(--projects-border)] bg-[#0f0f11] p-3 text-[12.5px] text-[var(--projects-muted)]">
                Run is queued — no steps executed yet.
              </p>
            ) : (
              <ul className="m-0 list-none p-0">
                {run.steps.map((step, index) => (
                  <StepRow key={`${step.label}-${index}`} step={step} last={index === run.steps.length - 1} />
                ))}
              </ul>
            )}
          </div>

          {run.error && (
            <div>
              <p className="m-0 mb-1.5 text-[11px] font-medium uppercase tracking-[0.06em] text-[var(--projects-muted)]">
                Error
              </p>
              <p className="m-0 rounded-lg border border-[color-mix(in_srgb,var(--projects-danger)_45%,var(--projects-border))] bg-[color-mix(in_srgb,var(--projects-danger)_8%,#0f0f11)] p-3 text-[12.5px] leading-5 text-[var(--projects-danger)]">
                {run.error}
              </p>
            </div>
          )}

          <div>
            <p className="m-0 mb-2 text-[11px] font-medium uppercase tracking-[0.06em] text-[var(--projects-muted)]">
              Explore
            </p>
            <div className="flex flex-wrap gap-2">
              <Link
                href="/admin/logs"
                className="inline-flex h-8 items-center rounded-md border border-[var(--projects-border)] px-3 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-white/[0.04]"
              >
                View Logs
              </Link>
              <Link
                href="/admin/traces"
                className="inline-flex h-8 items-center rounded-md border border-[var(--projects-border)] px-3 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-white/[0.04]"
              >
                View Trace
              </Link>
              <Link
                href="/agent"
                className="inline-flex h-8 items-center rounded-md border border-[var(--projects-border)] px-3 text-[12px] font-medium text-[var(--projects-text)] transition-colors hover:border-[var(--projects-border-hover)] hover:bg-white/[0.04]"
              >
                View Changes
              </Link>
            </div>
            <Mono className="mt-2 block text-[11px] text-[var(--projects-muted)]">
              trace {run.traceId} · repository {run.repository}
            </Mono>
          </div>
        </div>
      )}
    </DetailDrawer>
  );
}

function StepRow({ step, last }: { step: RunStep; last: boolean }) {
  const meta =
    step.state === "done"
      ? { Icon: CircleCheck, className: "text-[var(--projects-accent)]" }
      : step.state === "failed"
        ? { Icon: CircleX, className: "text-[var(--projects-danger)]" }
        : { Icon: LoaderCircle, className: "text-[var(--admin-info)]" };

  return (
    <li className="relative flex items-start gap-2.5 pb-3 last:pb-0">
      {!last && <span aria-hidden="true" className="absolute left-[7px] top-5 h-[calc(100%-12px)] w-px bg-[var(--projects-divider)]" />}
      <meta.Icon size={15} strokeWidth={2} className={`mt-0.5 shrink-0 ${meta.className} ${step.state === "running" ? "animate-spin" : ""}`} aria-hidden="true" />
      <span className="text-[12.5px] leading-5 text-[var(--projects-text)]">
        {step.label}
        {step.state === "running" && <span className="ml-2 text-[11px] text-[var(--admin-info)]">running…</span>}
        {step.state === "failed" && <span className="ml-2 text-[11px] text-[var(--projects-danger)]">failed</span>}
      </span>
    </li>
  );
}

function ColumnLabel({ children }: { children: ReactNode }) {
  return (
    <span className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--projects-muted)]">{children}</span>
  );
}
