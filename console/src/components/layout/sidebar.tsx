"use client";

import Link from "next/link";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { useConsoleRouteContext } from "@/components/navigation/console-route-context";
import { useCurrentAccount } from "@/api/queries";
import { cn } from "@/lib/utils";
import { sidebarNav, type NavSection } from "./sidebar-nav";

function NavGroup({
  section,
  collapsed,
  pathname,
  onNavigate,
}: {
  section: NavSection;
  collapsed: boolean;
  pathname: string;
  onNavigate?: () => void;
}) {
  return (
    <div className="mb-5">
      <p
        className={cn(
          "mb-2 px-3 text-[10px] font-medium uppercase tracking-[0.14em] text-fog",
          collapsed && "sr-only",
        )}
      >
        {section.label}
      </p>
      <nav className="space-y-0.5" aria-label={section.label}>
        {section.items.map((item) => {
          const active =
            pathname === item.href ||
            (!item.exact &&
              item.href !== "/" &&
              pathname.startsWith(`${item.href}/`));
          const Icon = item.icon;
          return (
            <Link
              key={item.href}
              href={item.href}
              onClick={onNavigate}
              title={collapsed ? item.label : undefined}
              aria-current={active ? "page" : undefined}
              className={cn(
                "group flex min-h-11 items-center gap-2.5 rounded-md px-3 py-2.5 text-[13px] text-fog transition-colors duration-150 hover:bg-white/[0.05] hover:text-mist",
                collapsed && "justify-center px-2",
                active && "bg-white/[0.06] text-mist",
              )}
            >
              <Icon
                className={cn(
                  "size-4 shrink-0",
                  active ? "text-acid-lime" : "text-ash group-hover:text-fog",
                )}
                aria-hidden="true"
              />
              {collapsed ? (
                <span className="sr-only">{item.label}</span>
              ) : (
                <span className="min-w-0 truncate">{item.label}</span>
              )}
              {active && !collapsed ? (
                <span className="ml-auto size-1.5 rounded-full bg-acid-lime" />
              ) : null}
            </Link>
          );
        })}
      </nav>
    </div>
  );
}

export function Sidebar({
  mobile = false,
  collapsed = false,
  onToggle,
  onNavigate,
}: {
  mobile?: boolean;
  collapsed?: boolean;
  onToggle?: () => void;
  onNavigate?: () => void;
}) {
  const { organizationId, projectId, pathname } = useConsoleRouteContext();
  const account = useCurrentAccount();
  const instanceAdmin =
    account.data?.account.instance_role === "instance_owner" ||
    account.data?.account.instance_role === "instance_admin";
  const nav = sidebarNav({ organizationId, projectId, instanceAdmin });
  const effectiveCollapsed = collapsed && !mobile;

  return (
    <aside
      aria-label="Primary navigation"
      className={cn(
        "scrollbar-thin shrink-0 flex-col overflow-y-auto border-r border-graphite bg-carbon py-5 transition-[width] duration-200",
        mobile
          ? "flex w-72 px-3"
          : cn("hidden lg:flex", collapsed ? "w-[4.5rem] px-2" : "w-60 px-3"),
      )}
    >
      <div
        className={cn(
          "mb-8 flex items-center gap-2.5",
          collapsed && !mobile ? "justify-center" : "px-3",
        )}
      >
        <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-acid-lime/40 bg-acid-lime text-sm font-semibold text-void">
          S
        </span>
        {!collapsed || mobile ? (
          <div className="min-w-0">
            <p className="text-sm font-semibold tracking-[-0.012em] text-paper">
              Stealth
            </p>
            <p className="text-[10px] uppercase tracking-[0.14em] text-fog">
              Control plane
            </p>
          </div>
        ) : null}
        {onToggle && !mobile ? (
          <button
            type="button"
            onClick={onToggle}
            className="ml-auto flex size-11 items-center justify-center rounded-md text-ash transition-colors duration-150 hover:bg-white/[0.06] hover:text-mist"
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            {collapsed ? (
              <PanelLeftOpen className="size-4" />
            ) : (
              <PanelLeftClose className="size-4" />
            )}
          </button>
        ) : null}
      </div>
      <NavGroup
        section={nav.organization}
        collapsed={effectiveCollapsed}
        pathname={pathname}
        onNavigate={onNavigate}
      />
      {nav.project ? (
        nav.project.map((section) => (
          <NavGroup
            key={section.label}
            section={section}
            collapsed={effectiveCollapsed}
            pathname={pathname}
            onNavigate={onNavigate}
          />
        ))
      ) : (
        <div
          className={cn(
            "rounded-lg border border-dashed border-graphite px-3 py-4 text-xs leading-5 text-fog",
            effectiveCollapsed && "border-0 px-0 text-center text-[0px]",
          )}
        >
          {effectiveCollapsed ? (
            <span className="sr-only">
              Select a project to open its service console.
            </span>
          ) : (
            "Select a project to open its service console."
          )}
        </div>
      )}
      {nav.admin ? (
        <NavGroup
          section={nav.admin}
          collapsed={effectiveCollapsed}
          pathname={pathname}
          onNavigate={onNavigate}
        />
      ) : null}
      <div
        className={cn(
          "mt-auto pt-8 text-[10px] leading-5 text-fog",
          effectiveCollapsed ? "px-1 text-center" : "px-3",
        )}
      >
        {effectiveCollapsed ? (
          <span className="text-sm text-fog">·</span>
        ) : (
          <>
            API is the platform.
            <br />
            Console is the interface.
          </>
        )}
      </div>
    </aside>
  );
}
