"use client";

import { ChevronDown, Ellipsis, Star } from "lucide-react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import type { components } from "@/api/generated/schema";
import { useApplicationShell } from "@/components/application-shell";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { PanelToggleIcon } from "@/features/navigation/sidebar-shared";
import { useFavoriteAgents } from "./use-favorite-agents";

type Agent = components["schemas"]["Agent"];

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--projects-ring)]";

/**
 * The conversation header: which agent this thread belongs to, the personal
 * star, the agent's own menu, and the control that folds the navigation rail.
 *
 * The rail toggle is the same control the sidebar header draws, so the page
 * never invents a second way to hide the navigation. Below `lg` there is no
 * rail, and the same button opens the mobile sheet instead.
 */
export function AgentHeader({
  agents,
  agent,
  onSelectAgent,
}: {
  agents: Agent[];
  agent: Agent | null;
  onSelectAgent: (agentId: string) => void;
}) {
  const router = useRouter();
  const shell = useApplicationShell();
  const { favorites, toggle } = useFavoriteAgents();
  const favorite = agent ? favorites.includes(agent.id) : false;

  const toggleNavigation = () => {
    if (window.matchMedia("(min-width: 1024px)").matches) {
      window.dispatchEvent(new Event("toggle-desktop-sidebar"));
      return;
    }
    shell?.openSidebar();
  };

  const copyAgentId = async () => {
    if (!agent) return;
    try {
      await navigator.clipboard.writeText(agent.id);
      toast.success("Agent ID copied");
    } catch {
      toast.error("Could not copy the agent ID");
    }
  };

  return (
    <header className="flex h-14 shrink-0 items-center gap-1 border-b border-[var(--projects-divider)] px-2 sm:px-3">
      <DropdownMenu>
        <DropdownMenuTrigger
          className={cn(
            "flex min-w-0 max-w-[calc(100%-3rem)] items-center gap-1.5 rounded-full bg-[var(--projects-card-bg)] py-1.5 pl-3 pr-2 text-left transition-colors hover:bg-[var(--projects-control)] data-[state=open]:bg-[var(--projects-control)]",
            FOCUS,
          )}
          aria-label="Switch agent"
        >
          <span className="truncate text-[13px] font-semibold leading-5 text-[var(--projects-text)]">
            {agent ? agent.name : "No agent"}
          </span>
          <ChevronDown
            size={14}
            strokeWidth={2}
            className="shrink-0 text-[var(--projects-muted)]"
            aria-hidden="true"
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" data-user-surface="">
          <DropdownMenuLabel>Agents</DropdownMenuLabel>
          {agents.length === 0 ? (
            <DropdownMenuItem disabled>No agents yet</DropdownMenuItem>
          ) : (
            agents.map((item) => (
              <DropdownMenuItem
                key={item.id}
                onSelect={() => onSelectAgent(item.id)}
                className="flex-col items-start gap-0"
              >
                <span className="w-full truncate text-[13px] text-[var(--projects-text)]">
                  {item.name}
                </span>
                <span className="w-full truncate text-[12px] text-[var(--projects-muted)]">
                  {item.project_name}
                </span>
              </DropdownMenuItem>
            ))
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      {agent ? (
        <>
          <button
            type="button"
            onClick={() => toggle(agent.id)}
            aria-pressed={favorite}
            aria-label={favorite ? "Unfavorite agent" : "Favorite agent"}
            title={favorite ? "Unfavorite agent" : "Favorite agent"}
            className={cn(
              "inline-flex size-8 shrink-0 items-center justify-center rounded-md transition-colors hover:bg-[var(--projects-control)]",
              favorite
                ? "text-[#FFA057]"
                : "text-[var(--projects-muted)] hover:text-[var(--projects-text)]",
              FOCUS,
            )}
          >
            <Star
              size={16}
              strokeWidth={1.8}
              fill={favorite ? "currentColor" : "none"}
              aria-hidden="true"
            />
          </button>

          <DropdownMenu>
            <DropdownMenuTrigger
              aria-label="Agent options"
              title="Agent options"
              className={cn(
                "inline-flex size-8 shrink-0 items-center justify-center rounded-md text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]",
                FOCUS,
              )}
            >
              <Ellipsis size={16} strokeWidth={1.8} aria-hidden="true" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" data-user-surface="">
              <DropdownMenuLabel>This agent</DropdownMenuLabel>
              <DropdownMenuItem onSelect={() => void copyAgentId()}>
                Copy agent ID
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() => router.push(`/projects/${agent.project_id}`)}
              >
                Open project
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem disabled>
                {agent.role} · {agent.provider}/{agent.model}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </>
      ) : null}

      <button
        type="button"
        onClick={toggleNavigation}
        aria-label="Toggle navigation"
        title="Toggle navigation"
        className={cn(
          "ml-auto inline-flex size-8 shrink-0 items-center justify-center rounded-md text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]",
          FOCUS,
        )}
      >
        <PanelToggleIcon className="size-4" />
      </button>
    </header>
  );
}
