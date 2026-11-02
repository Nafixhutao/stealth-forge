"use client";

import { useMemo, useState } from "react";
import { Loader2, Send } from "lucide-react";
import { useCurrentAccount } from "@/api/queries/account";
import {
  useAgentArtifacts,
  useAgentCatalog,
  useAgentRuns,
  useAgents,
} from "@/api/queries/agents";
import { useCreateAgentRun } from "@/api/mutations/agents";
import { ErrorState } from "@/components/feedback/error-state";
import { AgentArtifactsPanel } from "./agent-artifacts-panel";
import { AgentBackdrop } from "./agent-backdrop";
import { AgentComposer } from "./agent-composer";
import { AgentHeader } from "./agent-header";
import { AgentLanding } from "./agent-landing";
import { AgentThread } from "./agent-thread";

/** The one honest sentence under the box for each blocked state. */
function ComposerNote({
  hasAgent,
  canSend,
  queueOnly,
}: {
  hasAgent: boolean;
  canSend: boolean;
  queueOnly: boolean;
}) {
  if (!hasAgent) {
    return (
      <p className="m-0 mt-2 text-[12px] leading-5 text-[var(--projects-muted)]">
        No agent is configured on this installation yet.
      </p>
    );
  }
  if (!canSend) {
    return (
      <p className="m-0 mt-2 text-[12px] leading-5 text-[var(--projects-muted)]">
        Only an owner or admin of this agent&apos;s project can send prompts.
      </p>
    );
  }
  if (queueOnly) {
    return (
      <p className="m-0 mt-2 text-[12px] leading-5 text-[var(--projects-muted)]">
        Prompts are accepted into the durable queue. This installation has no
        execution worker yet, so replies stay pending.
      </p>
    );
  }
  return null;
}

/**
 * The Agent surface: a project-free conversation surface.
 *
 * The page never asks the reader to open a project. It opens on a composer and
 * a few example prompts, and the moment a run exists it becomes the thread.
 * Runs are queued, not executed inline; when this installation has no execution
 * worker the page says so under the box instead of leaving a prompt that looks
 * answered.
 */
export function AgentChat() {
  const agents = useAgents();
  const catalog = useAgentCatalog();
  const account = useCurrentAccount();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  // "New chat" shows the composer again even when the selected agent already
  // has a history. Selecting an agent or sending a run returns to the thread.
  const [newChat, setNewChat] = useState(false);
  const [prompt, setPrompt] = useState("");
  const [panelOpen, setPanelOpen] = useState(false);

  const items = useMemo(() => agents.data?.agents ?? [], [agents.data]);
  const activeAgent =
    items.find((item) => item.id === selectedId) ?? items[0] ?? null;

  const runs = useAgentRuns(activeAgent?.id);
  const artifacts = useAgentArtifacts(activeAgent?.id);
  const artifactItems = artifacts.data?.artifacts ?? [];
  const createRun = useCreateAgentRun(activeAgent?.id);
  // The run list reports whether this member may enqueue work for the agent's
  // project; the cross-project agent list cannot answer that per agent.
  const canSend = runs.data?.can_manage ?? false;
  const queueOnly = catalog.data?.execution.ready === false;

  const runItems = runs.data?.runs ?? [];
  const showLanding = newChat || !activeAgent || runItems.length === 0;

  const displayName =
    account.data?.account.display_name?.trim() ||
    account.data?.account.email?.split("@")[0] ||
    "your account";

  const send = async (text: string) => {
    await createRun.mutateAsync(text);
    setNewChat(false);
  };

  const composer = (
    <AgentComposer
      agent={activeAgent}
      canSend={canSend}
      value={prompt}
      onValueChange={setPrompt}
      onSend={send}
    />
  );

  const note = (
    <ComposerNote
      hasAgent={Boolean(activeAgent)}
      canSend={canSend}
      queueOnly={queueOnly}
    />
  );

  return (
    <div className="relative isolate flex h-[100dvh] min-h-0 flex-col overflow-hidden bg-[#0D0E0E] [--projects-card-bg:#161617] [--projects-control:#1C1C1E] [--projects-border:#232323] [--projects-border-hover:#2E2E2E] [--projects-divider:#232323] [--projects-muted:#8B8B90] lg:h-full lg:rounded-2xl">
      <AgentBackdrop />
      <AgentHeader
        agents={items}
        agent={activeAgent}
        onSelectAgent={(agentId) => {
          setSelectedId(agentId);
          setNewChat(false);
        }}
        onNewChat={() => {
          setNewChat(true);
          setPrompt("");
        }}
        hasArtifacts={artifactItems.length > 0}
        onOpenArtifacts={() => setPanelOpen(true)}
      />

      <div className="flex min-h-0 flex-1 overflow-hidden">
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
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
      ) : showLanding ? (
        <AgentLanding
          displayName={displayName}
          composer={composer}
          note={note}
          onPickExample={(example) => setPrompt(example)}
        />
      ) : (
        <>
          <AgentThread agent={activeAgent} runs={runItems} />

          <div className="shrink-0 px-3 pb-2 sm:px-6">
            <div className="mx-auto w-full max-w-[1016px]">
              {composer}
              {note}
            </div>
          </div>
        </>
      )}
        </div>

        {artifactItems.length > 0 && activeAgent ? (
          <aside className="hidden w-[420px] shrink-0 border-l border-[#232323] lg:block">
            <AgentArtifactsPanel
              agentId={activeAgent.id}
              artifacts={artifactItems}
            />
          </aside>
        ) : null}
      </div>

      {panelOpen && artifactItems.length > 0 && activeAgent ? (
        <div className="fixed inset-0 z-50 flex flex-col bg-[#0D0E0E] lg:hidden">
          <AgentArtifactsPanel
            agentId={activeAgent.id}
            artifacts={artifactItems}
            onClose={() => setPanelOpen(false)}
          />
        </div>
      ) : null}

      <footer className="flex shrink-0 items-center justify-end gap-1.5 px-4 pb-2 text-[12px] leading-5 text-[var(--projects-muted)]">
        <Send size={13} strokeWidth={1.8} aria-hidden="true" />
        <span>Agent</span>
      </footer>
    </div>
  );
}
