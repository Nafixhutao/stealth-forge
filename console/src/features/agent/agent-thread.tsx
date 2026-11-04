"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, Loader2 } from "lucide-react";
import { toast } from "sonner";
import type { components } from "@/api/generated/schema";
import { formatConversationStamp } from "@/lib/format";
import { cn } from "@/lib/utils";

type Agent = components["schemas"]["Agent"];
type AgentRun = components["schemas"]["AgentRun"];

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--projects-ring)]";

/** What a run's own status means to the reader, in one line. */
function runStatusLine(run: AgentRun) {
  switch (run.status) {
    case "queued":
      return "Queued — this installation has no execution worker yet, so the run waits in the queue.";
    case "running":
      return "Running…";
    case "failed":
      return run.error_message ?? "The run failed.";
    case "cancelled":
      return "Cancelled before it produced a reply.";
    default:
      return "No reply yet.";
  }
}

function CopyReply({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(false), 1600);
    return () => window.clearTimeout(timer);
  }, [copied]);

  return (
    <button
      type="button"
      aria-label="Copy reply"
      title="Copy reply"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text);
          setCopied(true);
        } catch {
          toast.error("Could not copy the reply");
        }
      }}
      className={cn(
        "inline-flex size-7 items-center justify-center rounded-md text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]",
        FOCUS,
      )}
    >
      {copied ? (
        <Check size={14} strokeWidth={1.9} aria-hidden="true" />
      ) : (
        <Copy size={14} strokeWidth={1.9} aria-hidden="true" />
      )}
    </button>
  );
}

/**
 * The conversation: one turn per run — the prompt on the right, the run's
 * reply (or the reason there is none) on the left, grouped by day.
 *
 * The API returns runs newest first; a conversation reads oldest first, so the
 * list is reversed once here rather than at every use.
 */
export function AgentThread({
  agent,
  runs,
}: {
  agent: Agent | null;
  runs: AgentRun[];
}) {
  const scrollRef = useRef<HTMLDivElement>(null);

  // A conversation reads oldest first, and each day gets one separator above
  // its first turn. Both are derived here so the render stays a plain map.
  const turns = useMemo(() => {
    const ordered = [...runs].reverse();
    return ordered.map((run, index) => {
      const stamp = formatConversationStamp(run.queued_at);
      const runDay = stamp.split(" ").slice(0, -1).join(" ") || stamp;
      const previous = ordered[index - 1];
      const previousDay = previous
        ? formatConversationStamp(previous.queued_at)
            .split(" ")
            .slice(0, -1)
            .join(" ")
        : "";
      return { run, stamp, showDay: index === 0 || runDay !== previousDay };
    });
  }, [runs]);

  // A new run is the only thing that moves the conversation, so following it
  // is the one scroll the thread performs. Reading history above is untouched.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [turns.length]);

  return (
    <div
      ref={scrollRef}
      className="sidebar-scrollbar min-h-0 flex-1 overflow-y-auto [overscroll-behavior:contain]"
    >
      <div className="mx-auto flex w-full max-w-[1016px] flex-col gap-6 px-4 py-6 sm:px-6">
        {turns.length === 0 ? (
          <p className="m-0 mt-16 text-center text-[13px] leading-5 text-[var(--projects-muted)]">
            {agent
              ? `${agent.name} has no runs yet. Send the first prompt below.`
              : "Select an agent to open its conversation."}
          </p>
        ) : null}

        {turns.map(({ run, stamp, showDay }) => {
          const reply = run.output_text?.trim();

          return (
            <div key={run.id} className="flex flex-col gap-4">
              {showDay ? (
                <p className="m-0 text-center text-[12px] leading-5 text-[var(--projects-muted)]">
                  {stamp}
                </p>
              ) : null}

              <div className="flex justify-end">
                <p className="m-0 max-w-[85%] whitespace-pre-wrap rounded-[10px] bg-[var(--projects-control)] px-3 py-1.5 text-[14px] leading-5 text-[var(--projects-text)] [overflow-wrap:anywhere] sm:max-w-[70%]">
                  {run.prompt}
                </p>
              </div>

              <div className="flex max-w-[92%] flex-col items-start gap-1 sm:max-w-[80%]">
                {reply ? (
                  <p className="m-0 whitespace-pre-wrap text-[15px] leading-[26px] text-[var(--projects-text)] [overflow-wrap:anywhere]">
                    {reply}
                  </p>
                ) : (
                  <p
                    className={cn(
                      "m-0 flex items-center gap-2 text-[14px] leading-6",
                      run.status === "failed"
                        ? "text-[var(--projects-danger)]"
                        : "text-[var(--projects-muted)]",
                    )}
                  >
                    {run.status === "running" ? (
                      <Loader2
                        size={14}
                        strokeWidth={2}
                        className="animate-spin"
                        aria-hidden="true"
                      />
                    ) : null}
                    {runStatusLine(run)}
                  </p>
                )}
                {reply ? <CopyReply text={reply} /> : null}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
