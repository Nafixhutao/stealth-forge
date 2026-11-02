"use client";

import { useState } from "react";
import {
  ArrowUp,
  ChevronDown,
  Layers,
  Paperclip,
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
 * The value is controlled by the page so an example card can fill the box
 * before the reader types. Attachments are drawn but disabled on purpose — the
 * API has no upload path for an agent run, and a control that silently drops a
 * file is worse than one that says it cannot take it yet.
 */
export function AgentComposer({
  agent,
  canSend,
  value,
  onValueChange,
  onSend,
}: {
  agent: Agent | null;
  canSend: boolean;
  value: string;
  onValueChange: (value: string) => void;
  onSend: (prompt: string) => Promise<void>;
}) {
  const [sending, setSending] = useState(false);
  const prompt = value.trim();
  const ready = Boolean(agent) && canSend && prompt.length > 0 && !sending;

  const send = async () => {
    if (!ready) return;
    setSending(true);
    try {
      await onSend(prompt);
      onValueChange("");
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
        "flex h-[104px] min-h-[104px] w-full flex-col rounded-[16px] border border-white/[0.07] bg-[linear-gradient(180deg,#17171A_0%,#131316_100%)] p-3 shadow-[0_24px_60px_-28px_rgba(0,0,0,0.9)] transition-all sm:p-4",
        "focus-within:border-white/[0.14] focus-within:shadow-[0_0_0_1px_rgba(94,106,210,0.28),0_24px_60px_-28px_rgba(0,0,0,0.9)]",
      )}
    >
      <label htmlFor="agent-prompt" className="sr-only">
        Ask Agent
      </label>
      <textarea
        id="agent-prompt"
        value={value}
        onChange={(event) => onValueChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !event.shiftKey) {
            event.preventDefault();
            void send();
          }
        }}
        placeholder="Ask Agent..."
        rows={2}
        disabled={!agent || !canSend}
        className="min-h-0 flex-1 resize-none bg-transparent text-[15px] leading-[26px] text-[#EDEDF0] outline-none placeholder:text-[#6B6B72] disabled:cursor-not-allowed"
      />

      <div className="mt-2 flex items-center gap-1">
        <DropdownMenu>
          <DropdownMenuTrigger
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[12px] leading-5 text-[#8B8B90] transition-colors hover:bg-white/[0.05] hover:text-[#D6D6DC]",
              FOCUS,
            )}
            aria-label="Skills"
          >
            <Layers size={14} strokeWidth={1.8} aria-hidden="true" />
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
            className="inline-flex size-8 items-center justify-center rounded-md text-[#6F6F77] opacity-45"
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
              "inline-flex size-8 items-center justify-center rounded-full transition-colors",
              ready
                ? "cursor-pointer bg-[linear-gradient(180deg,#6B76DD_0%,#5E6AD2_100%)] text-white shadow-[0_6px_18px_rgba(94,106,210,0.45)] hover:brightness-110"
                : "cursor-not-allowed bg-white/[0.05] text-[#6F6F77] opacity-70",
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
