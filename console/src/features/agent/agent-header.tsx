"use client";

import { ChevronDown, FileText } from "lucide-react";
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

type Agent = components["schemas"]["Agent"];

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--projects-ring)]";

/**
 * The Agent surface header: a single "New chat" control that also switches the
 * conversation to another agent, and the control that folds the navigation
 * rail. It deliberately has no project language — the surface is the account's
 * agent, not a project page.
 *
 * The rail toggle is the same control the sidebar header draws, so the page
 * never invents a second way to hide the navigation. Below `lg` there is no
 * rail, and the same button opens the mobile sheet instead.
 */
export function AgentHeader({
  agents,
  agent,
  onSelectAgent,
  onNewChat,
  hasArtifacts = false,
  onOpenArtifacts,
}: {
  agents: Agent[];
  agent: Agent | null;
  onSelectAgent: (agentId: string) => void;
  onNewChat: () => void;
  hasArtifacts?: boolean;
  onOpenArtifacts?: () => void;
}) {
  const shell = useApplicationShell();

  const toggleNavigation = () => {
    if (window.matchMedia("(min-width: 1024px)").matches) {
      window.dispatchEvent(new Event("toggle-desktop-sidebar"));
      return;
    }
    shell?.openSidebar();
  };

  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b border-[#232326] px-3 sm:px-4">
      <DropdownMenu>
        <DropdownMenuTrigger
          className={cn(
            "flex min-w-0 items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-[14px] font-semibold leading-5 text-[var(--projects-text)] transition-colors hover:bg-[var(--projects-control)] data-[state=open]:bg-[var(--projects-control)]",
            FOCUS,
          )}
          aria-label="Agent chat menu"
        >
          <span className="truncate">New chat</span>
          <ChevronDown
            size={14}
            strokeWidth={2}
            className="shrink-0 text-[var(--projects-muted)]"
            aria-hidden="true"
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" data-user-surface="">
          <DropdownMenuItem onSelect={onNewChat}>New chat</DropdownMenuItem>
          {agents.length > 0 ? (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuLabel>Agents</DropdownMenuLabel>
              {agents.map((item) => (
                <DropdownMenuItem
                  key={item.id}
                  onSelect={() => onSelectAgent(item.id)}
                >
                  <span className="truncate">{item.name}</span>
                  {item.id === agent?.id ? (
                    <span className="ml-auto text-[11px] uppercase tracking-[0.08em] text-[var(--projects-muted)]">
                      Current
                    </span>
                  ) : null}
                </DropdownMenuItem>
              ))}
            </>
          ) : (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem disabled>No agents yet</DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <div className="ml-auto flex items-center gap-1">
        {hasArtifacts ? (
          <button
            type="button"
            onClick={onOpenArtifacts}
            aria-label="Open artifacts"
            title="Open artifacts"
            className={cn(
              "inline-flex size-8 shrink-0 items-center justify-center rounded-md text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)] lg:hidden",
              FOCUS,
            )}
          >
            <FileText size={16} strokeWidth={1.8} aria-hidden="true" />
          </button>
        ) : null}
        <button
          type="button"
          onClick={toggleNavigation}
          aria-label="Toggle navigation"
          title="Toggle navigation"
          className={cn(
            "inline-flex size-8 shrink-0 items-center justify-center rounded-md text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]",
            FOCUS,
          )}
        >
          <PanelToggleIcon className="size-4" />
        </button>
      </div>
    </header>
  );
}
