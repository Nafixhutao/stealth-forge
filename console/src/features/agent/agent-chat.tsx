"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { Bot, Loader2 } from "lucide-react";
import { useAgentCatalog, useAgentRuns, useAgents } from "@/api/queries/agents";
import { useCreateAgentRun } from "@/api/mutations/agents";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { Button } from "@/components/ui/button";
import { SECONDARY_ACTION } from "@/features/user/action-styles";
import { AgentComposer } from "./agent-composer";
import { AgentHeader } from "./agent-header";
import { AgentThread } from "./agent-thread";

/**
 * The Agent surface: one conversation with one agent, drawn like a chat rather
 * than a dashboard — the agent on top, the thread in the middle, the reply box
 * pinned at the bottom.
 *
 * Runs are queued, not executed inline. When this installation has no
 * execution worker the page says so under the box instead of leaving a prompt
 * that looks answered; the run's own status is the source of that truth.
 */
export function AgentChat() {
  const agents = useAgents();
  const catalog = useAgentCatalog();
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const items = useMemo(() => agents.data?.agents ?? [], [agents.data]);
  const agent =
    items.find((item) => item.id === selectedId) ?? items[0] ?? null;

  const runs = useAgentRuns(agent?.id);
  const createRun = useCreateAgentRun(agent?.id);
  // The run list reports whether this member may enqueue work for the agent's
  // project; the cross-project agent list cannot answer that per agent.
  const canSend = runs.data?.can_manage ?? false;

  const queueOnly = catalog.data?.execution.ready === false;

  return (
    <div className="flex h-[100dvh] min-h-0 flex-col lg:h-full">
      <AgentHeader agents={items} agent={agent} onSelectAgent={setSelectedId} />

      {agents.isPending ? (
        <div className="flex flex-1 items-center justify-center gap-2 text-[13px] text-[var(--projects-muted)]">
          <Loader2
            size={15}
            strokeWidth={2}
            className="animate-spin"
            aria-hidden="true"
          />
          Loading agents…
        </div>
      ) : agents.isError ? (
        <div className="flex flex-1 items-center justify-center px-4">
          <div className="w-full max-w-[520px]">
            <ErrorState
              error={agents.error}
              retry={() => void agents.refetch()}
              title="Unable to load agents"
            />
          </div>
        </div>
      ) : items.length === 0 ? (
        <div className="flex flex-1 items-center justify-center px-4">
          <EmptyState
            className="max-w-[420px]"
            icon={Bot}
            title="No agent yet"
            description="An agent belongs to a project: it carries that project's name, role, and skills. Open a project to configure one, then come back here to talk to it."
            action={
              <Button asChild variant="outline" className={SECONDARY_ACTION}>
                <Link href="/user">Go to projects</Link>
              </Button>
            }
          />
        </div>
      ) : (
        <>
          <AgentThread agent={agent} runs={runs.data?.runs ?? []} />

          <div className="shrink-0 px-3 pb-2 sm:px-6">
            <div className="mx-auto w-full max-w-[1016px]">
              <AgentComposer
                agent={agent}
                canSend={canSend}
                onSend={async (prompt) => {
                  await createRun.mutateAsync(prompt);
                }}
              />
              {!canSend ? (
                <p className="m-0 mt-2 text-[12px] leading-5 text-[var(--projects-muted)]">
                  Only an owner or admin of this agent&apos;s project can send
                  prompts.
                </p>
              ) : queueOnly ? (
                <p className="m-0 mt-2 text-[12px] leading-5 text-[var(--projects-muted)]">
                  Prompts are accepted into the durable queue. This installation
                  has no execution worker yet, so replies stay pending.
                </p>
              ) : null}
            </div>
          </div>
        </>
      )}

      <footer className="flex shrink-0 items-center justify-end gap-1.5 px-4 pb-2 text-[12px] leading-5 text-[var(--projects-muted)]">
        <Bot size={13} strokeWidth={1.8} aria-hidden="true" />
        <span>Agent</span>
      </footer>
    </div>
  );
}
