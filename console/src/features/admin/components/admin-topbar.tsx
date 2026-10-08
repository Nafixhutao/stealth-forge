"use client";

import { usePathname, useRouter } from "next/navigation";
import type { RefObject } from "react";
import { LogOut, PanelLeftClose, PanelLeftOpen, UserRound } from "lucide-react";
import { useCurrentAccount } from "@/api/queries/account";
import { useLogout } from "@/api/mutations/auth";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { PanelToggleIcon } from "@/features/navigation/sidebar-shared";
import { activeAdminNavItem } from "./admin-nav";

/** Initials fallback when the account has no avatar URL. */
function initialsFor(name: string, email: string): string {
  const source = name.trim() || email.trim();
  if (!source) return "?";
  const parts = source.split(/[\s@._-]+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

/** Round avatar. Uses a background image so an external provider URL needs no
    next/image remote-pattern configuration. */
function AdminAvatar({ url, initials }: { url?: string; initials: string }) {
  if (url) {
    return (
      <span
        aria-hidden="true"
        style={{ backgroundImage: `url(${url})` }}
        className="size-8 shrink-0 rounded-full bg-[#1B191D] bg-cover bg-center ring-1 ring-[#322F37]"
      />
    );
  }
  return (
    <span
      aria-hidden="true"
      className="admin-mono flex size-8 shrink-0 items-center justify-center rounded-full bg-[#1B191D] text-[11px] font-semibold text-[#C5C1C9] ring-1 ring-[#322F37]"
    >
      {initials}
    </span>
  );
}

/**
 * Admin top bar: the rail toggle, the current page title, and the account
 * menu. The rail control lives here rather than in the sidebar so it stays
 * reachable while the rail is collapsed.
 */
export function AdminTopBar({
  collapsed,
  onToggleCollapsed,
  onOpenMobile,
  onToggleRef,
}: {
  collapsed: boolean;
  onToggleCollapsed: () => void;
  onOpenMobile: () => void;
  onToggleRef?: RefObject<HTMLButtonElement | null>;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const account = useCurrentAccount();
  const logout = useLogout();

  const item = activeAdminNavItem(pathname);
  const Icon = item?.icon;
  const current = account.data?.account;
  const displayName = current?.display_name?.trim() || "";
  const email = current?.email ?? "";
  const initials = initialsFor(displayName, email);

  const toggleLabel = collapsed ? "Expand sidebar" : "Collapse sidebar";

  return (
    <header className="sticky top-0 z-30 flex h-14 shrink-0 items-center gap-2 border-b border-[#322F37] bg-[#121014]/95 px-3 pt-[env(safe-area-inset-top)] backdrop-blur supports-[backdrop-filter]:bg-[#121014]/80 sm:gap-3 sm:px-4">
      {/* Desktop rail control. */}
      <button
        ref={onToggleRef}
        type="button"
        onClick={onToggleCollapsed}
        aria-label={toggleLabel}
        title={toggleLabel}
        aria-expanded={!collapsed}
        className="hidden size-8 shrink-0 items-center justify-center rounded-md text-[#AAA6AE] transition-colors hover:bg-white/[0.05] hover:text-[#EEEAF0] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--projects-accent)] lg:inline-flex"
      >
        {collapsed ? (
          <PanelLeftOpen size={16} strokeWidth={1.8} aria-hidden="true" />
        ) : (
          <PanelLeftClose size={16} strokeWidth={1.8} aria-hidden="true" />
        )}
      </button>

      {/* Mobile sheet opener. */}
      <button
        type="button"
        onClick={onOpenMobile}
        aria-label="Open navigation"
        aria-haspopup="dialog"
        className="inline-flex size-8 shrink-0 items-center justify-center rounded-md text-[#AAA6AE] transition-colors hover:bg-white/[0.05] hover:text-[#EEEAF0] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--projects-accent)] lg:hidden"
      >
        <PanelToggleIcon className="size-[17px]" />
      </button>

      {/* Current page. */}
      <div className="flex min-w-0 items-center gap-2.5">
        {Icon ? (
          <span
            aria-hidden="true"
            className="hidden size-7 shrink-0 items-center justify-center rounded-md border border-[#322F37] bg-[#1B191D] text-[#AAA6AE] sm:flex"
          >
            <Icon size={14} strokeWidth={1.8} />
          </span>
        ) : null}
        <h1 className="m-0 truncate text-[15px] font-semibold tracking-[-0.01em] text-[#EEEAF0]">
          {item?.label ?? "Admin"}
        </h1>
      </div>

      {/* Account menu. */}
      <DropdownMenu>
        <DropdownMenuTrigger
          aria-label="Account menu"
          className="ml-auto inline-flex shrink-0 items-center rounded-full transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--projects-accent)]"
        >
          <AdminAvatar
            url={current?.avatar_url ?? undefined}
            initials={initials}
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-56">
          {/* The shared label style is an uppercase section heading; this one
              carries a name and address, so it opts out. */}
          <DropdownMenuLabel className="normal-case tracking-normal">
            <span className="block truncate text-[13px] font-medium text-[var(--projects-text)]">
              {displayName || current?.provider_login || "Signed in"}
            </span>
            {email ? (
              <span className="mt-0.5 block truncate text-[11.5px] font-normal text-[var(--projects-muted)]">
                {email}
              </span>
            ) : null}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => router.push("/account")}>
            <UserRound className="size-4" /> Account
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() =>
              logout.mutate(undefined, {
                onSuccess: () => router.replace("/login"),
              })
            }
          >
            <LogOut className="size-4" /> Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </header>
  );
}
