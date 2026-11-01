"use client";

import { useState } from "react";
import {
  ArrowUp,
  ChevronDown,
  Paperclip,
  Sparkles,
  Wrench,
} from "lucide-react";
import { toast } from "sonner";
import type { components } from "@/api/generated/schema";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

type Agent = components["schemas"]["Agent"];

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--projects-ring)]";

/**
 * The reply box: the prompt field, the skills this agent can use, and the send
 * control. Enter sends, Shift+Enter starts a new line.
 *
 * Attachments are drawn but disabled on purpose — the API has no upload path
 * for an agent run, and a control that silently drops a file is worse than one
 * that says it cannot take it yet.
 */
export function AgentComposer({
  agent,
  canSend,
  onSend,
}: {
  agent: Agent | null;
  canSend: boolean;
  onSend: (prompt: string) => Promise<void>;
}) {
  const [value, setValue] = useState("");
  const [sending, setSending] = useState(false);
  const prompt = value.trim();
  const ready = Boolean(agent) && canSend && prompt.length > 0 && !sending;

  const send = async () => {
    if (!ready) return;
    setSending(true);
    try {
      await onSend(prompt);
      setValue("");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Could not queue the prompt",
      );
    } finally {
      setSending(false);
    }
  };

  return (
    <div
      className={cn(
        "flex min-h-[148px] w-full flex-col rounded-[12px] border border-[var(--projects-border)] bg-[var(--projects-card-bg)] p-3 transition-colors sm:min-h-[166px] sm:p-4",
        "focus-within:border-[var(--projects-border-hover)]",
      )}
    >
      <label htmlFor="agent-prompt" className="sr-only">
        Reply to {agent ? agent.name : "the agent"}
      </label>
      <textarea
        id="agent-prompt"
        value={value}
        onChange={(event) => setValue(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !event.shiftKey) {
            event.preventDefault();
            void send();
          }
        }}
        placeholder="Reply..."
        rows={3}
        disabled={!agent || !canSend}
        className="min-h-0 flex-1 resize-none bg-transparent text-[15px] leading-[26px] text-[var(--projects-text)] outline-none placeholder:text-[var(--projects-muted)] disabled:cursor-not-allowed"
      />

      <div className="mt-2 flex items-center gap-1">
        <DropdownMenu>
          <DropdownMenuTrigger
            className={cn(
              "inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 text-[13px] leading-5 text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]",
              FOCUS,
            )}
            aria-label="Skills"
          >
            <Sparkles size={14} strokeWidth={1.8} aria-hidden="true" />
            Skills
            <ChevronDown size={12} strokeWidth={2} aria-hidden="true" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" side="top" data-user-surface="">
            <DropdownMenuLabel>Skills this agent uses</DropdownMenuLabel>
            {agent && agent.tools.length > 0 ? (
              agent.tools.map((tool) => (
                <DropdownMenuItem key={tool} disabled>
                  <Wrench size={13} strokeWidth={1.8} aria-hidden="true" />
                  {tool}
                </DropdownMenuItem>
              ))
            ) : (
              <DropdownMenuItem disabled>No skills configured</DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>

        <div className="ml-auto flex items-center gap-1">
          <button
            type="button"
            disabled
            aria-label="Attach a file"
            title="Attachments are not supported yet"
            className="inline-flex size-8 items-center justify-center rounded-md text-[var(--projects-muted)] opacity-45"
          >
            <Paperclip size={16} strokeWidth={1.8} aria-hidden="true" />
          </button>
          <button
            type="button"
            onClick={() => void send()}
            disabled={!ready}
            aria-label="Send prompt"
            title="Send prompt"
            className={cn(
              "inline-flex size-8 items-center justify-center rounded-full bg-[var(--projects-control)] text-[var(--projects-text)] transition-colors hover:bg-[var(--projects-border-hover)] disabled:cursor-not-allowed disabled:opacity-45",
              FOCUS,
            )}
          >
            <ArrowUp size={16} strokeWidth={2} aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  );
}
