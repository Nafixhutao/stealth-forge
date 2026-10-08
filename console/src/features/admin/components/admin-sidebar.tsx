"use client";

import { useEffect, useId, useRef, useState } from "react";
import Link from "next/link";
import Image from "next/image";
import { usePathname, useRouter } from "next/navigation";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import {
  Activity,
  Bug,
  ChartNoAxesCombined,
  BrainCircuit,
  Gauge,
  LogOut,
  Logs,
  PanelLeftClose,
  PanelLeftOpen,
  RadioTower,
  Server,
  Settings,
  TriangleAlert,
  Users,
  Waypoints,
  X,
  type LucideIcon,
} from "lucide-react";
import { useLogout } from "@/api/mutations/auth";
import {
  LABEL_ENTER_TRANSITION,
  LABEL_EXIT_TRANSITION,
  PANEL_CLOSE_TRANSITION,
  PANEL_TRANSITION,
  SIDEBAR_COLLAPSE_TRANSITION,
  SIDEBAR_EXPAND_TRANSITION,
  SPRING_LAYOUT,
  SPRING_PRESS,
} from "@/lib/ease";
import { cn } from "@/lib/utils";

/** Rail geometry. Collapsed matches the reference rail; expanded matches the
    previous fixed admin panel width. */
const COLLAPSED_WIDTH = 72;
const EXPANDED_WIDTH = 228;

/** The rail choice is a device preference, so it survives reloads. */
const COLLAPSE_STORAGE_KEY = "stealth.admin.sidebar.collapsed:v1";

function readCollapsedPreference(): boolean {
  try {
    return window.localStorage.getItem(COLLAPSE_STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}

function writeCollapsedPreference(collapsed: boolean) {
  try {
    window.localStorage.setItem(COLLAPSE_STORAGE_KEY, collapsed ? "1" : "0");
  } catch {
    // Ignored: the rail still works, it just forgets the preference.
  }
}

interface AdminNavItem {
  label: string;
  href: string;
  icon: LucideIcon;
  /** Exact match instead of prefix (Overview must not swallow /admin/*). */
  exact?: boolean;
}

interface AdminNavGroup {
  label: string;
  items: AdminNavItem[];
}

/** Admin navigation — kept conceptually separate from the customer rail. */
const ADMIN_NAV: AdminNavGroup[] = [
  {
    label: "Admin",
    items: [{ label: "Overview", href: "/admin", icon: Gauge, exact: true }],
  },
  {
    label: "Observability",
    items: [
      { label: "Infrastructure", href: "/admin/infrastructure", icon: Server },
      { label: "Logs", href: "/admin/logs", icon: Logs },
      { label: "Traces", href: "/admin/traces", icon: Waypoints },
      { label: "Errors", href: "/admin/errors", icon: Bug },
    ],
  },
  {
    label: "Operations",
    items: [
      { label: "Agent Runs", href: "/admin/runs", icon: Activity },
      { label: "Workers", href: "/admin/workers", icon: BrainCircuit },
      { label: "Incidents", href: "/admin/incidents", icon: TriangleAlert },
    ],
  },
  {
    label: "Platform",
    items: [
      { label: "Users", href: "/admin/users", icon: Users },
      { label: "Models & Providers", href: "/admin/providers", icon: Server },
      { label: "Usage", href: "/admin/usage", icon: ChartNoAxesCombined },
    ],
  },
  {
    label: "Configuration",
    items: [
      { label: "Status Page", href: "/admin/status", icon: RadioTower },
      { label: "Settings", href: "/admin/settings", icon: Settings },
    ],
  },
];

function isActive(pathname: string, item: AdminNavItem) {
  return item.exact ? pathname === item.href : pathname.startsWith(item.href);
}

/** The project mark, shown in a bordered tile like the reference rail. When
    the rail is collapsed the mark is the only brand cue, so it carries the
    accessible name; expanded, the adjacent title does. */
function AdminBrandMark({
  collapsed,
  className,
}: {
  collapsed: boolean;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "flex shrink-0 items-center justify-center rounded-lg border border-[#322F37] bg-[#1B191D]",
        className,
      )}
    >
      <Image
        src="/stealth-cat.png"
        alt={collapsed ? "Stealth Admin" : ""}
        width={20}
        height={20}
        className="size-5"
      />
    </span>
  );
}

function AdminNavRow({
  item,
  active,
  collapsed,
  layoutId,
  reduce,
  onNavigate,
}: {
  item: AdminNavItem;
  active: boolean;
  collapsed: boolean;
  layoutId: string;
  reduce: boolean;
  onNavigate?: () => void;
}) {
  const Icon = item.icon;
  return (
    <motion.div layout="position" transition={SPRING_LAYOUT}>
      <motion.button
        type="button"
        onClick={onNavigate}
        whileTap={reduce ? undefined : { scale: 0.98 }}
        transition={SPRING_PRESS}
        aria-current={active ? "page" : undefined}
        title={collapsed ? item.label : undefined}
        className={cn(
          "relative isolate flex h-9 w-full items-center rounded-md text-left text-[13px] transition-colors",
          collapsed ? "justify-center px-0" : "gap-2.5 px-2.5",
          active
            ? "text-[oklch(0.83_0.11_162)]"
            : "text-[#C5C1C9] hover:bg-white/[0.035] hover:text-[#EEEAF0]",
        )}
      >
        {active && (
          <motion.span
            aria-hidden="true"
            layoutId={layoutId}
            transition={reduce ? { duration: 0 } : SPRING_LAYOUT}
            className="absolute inset-0 -z-10 rounded-md bg-[color-mix(in_srgb,var(--projects-accent)_10%,transparent)]"
          />
        )}
        <Icon
          size={16}
          strokeWidth={1.8}
          className={cn(
            "shrink-0",
            active ? "text-[var(--projects-accent)]" : "text-[#AAA6AE]",
          )}
        />
        <AnimatePresence initial={false}>
          {!collapsed && (
            <motion.span
              key="label"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1, transition: LABEL_ENTER_TRANSITION }}
              exit={{ opacity: 0, transition: LABEL_EXIT_TRANSITION }}
              className="truncate"
            >
              {item.label}
            </motion.span>
          )}
        </AnimatePresence>
      </motion.button>
    </motion.div>
  );
}

/** Collapse/expand control. Mirrors the customer rail's affordance. */
function RailToggle({
  collapsed,
  onToggle,
  className,
}: {
  collapsed: boolean;
  onToggle: () => void;
  className?: string;
}) {
  const Icon = collapsed ? PanelLeftOpen : PanelLeftClose;
  const label = collapsed ? "Expand sidebar" : "Collapse sidebar";
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-label={label}
      title={label}
      aria-expanded={!collapsed}
      className={cn(
        "inline-flex size-7 shrink-0 items-center justify-center rounded-md text-[#AAA6AE] transition-colors hover:bg-white/[0.05] hover:text-[#EEEAF0] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--projects-accent)]",
        className,
      )}
    >
      <Icon size={15} strokeWidth={1.8} aria-hidden="true" />
    </button>
  );
}

function AdminSidebarBody({
  collapsed = false,
  onToggle,
  onNavigate,
  onMobileClose,
}: {
  collapsed?: boolean;
  onToggle?: () => void;
  /** Fires after a navigation link is chosen (mobile closes the sheet). */
  onNavigate?: (href: string) => void;
  onMobileClose?: () => void;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const logout = useLogout();
  const layoutId = useId();
  const reduce = useReducedMotion() ?? false;

  const handleNavigate = (href: string) => {
    router.push(href);
    onNavigate?.(href);
  };

  const handleSignOut = () => {
    logout.mutate(undefined, {
      onSuccess: () => router.replace("/login"),
    });
  };

  return (
    <>
      <header
        className={cn(
          "flex h-[56px] shrink-0 items-center",
          collapsed ? "justify-center" : "gap-2.5 px-4",
        )}
      >
        <AdminBrandMark collapsed={collapsed} className="size-8" />
        <AnimatePresence initial={false}>
          {!collapsed && (
            <motion.div
              key="brand"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1, transition: LABEL_ENTER_TRANSITION }}
              exit={{ opacity: 0, transition: LABEL_EXIT_TRANSITION }}
              className="flex min-w-0 flex-1 items-center gap-2"
            >
              <span className="min-w-0 flex-1 truncate text-[13.5px] font-semibold tracking-[-0.01em] text-[#EEEAF0]">
                Admin Console
              </span>
              <span className="admin-mono shrink-0 rounded-[5px] border border-[#322F37] px-[5px] py-[2px] text-[9.5px] font-medium leading-none text-[#AAA6AE]">
                PROD
              </span>
            </motion.div>
          )}
        </AnimatePresence>
        {!collapsed && onToggle ? (
          <RailToggle collapsed={false} onToggle={onToggle} />
        ) : null}
        <button
          type="button"
          onClick={onMobileClose}
          aria-label="Close sidebar"
          className="inline-flex size-6 shrink-0 items-center justify-center text-[#AAA6AE] transition-colors hover:text-[#EEEAF0] lg:hidden"
        >
          <X size={14} strokeWidth={1.8} />
        </button>
      </header>

      {/* Collapsed rail keeps the toggle on its own row so the 72px column
          never has to fit the logo and the control side by side. */}
      {collapsed && onToggle ? (
        <div className="flex shrink-0 justify-center pb-2">
          <RailToggle collapsed onToggle={onToggle} />
        </div>
      ) : null}

      <nav
        aria-label="Admin navigation"
        className={cn(
          "sidebar-scrollbar min-h-0 flex-1 overflow-y-auto overflow-x-hidden pb-2",
          collapsed ? "px-2" : "px-3",
        )}
      >
        {ADMIN_NAV.map((group, groupIndex) => (
          <div key={group.label}>
            {groupIndex > 0 && (
              <div
                aria-hidden="true"
                className={cn(
                  "my-2 h-px bg-[#26242b]",
                  collapsed ? "mx-2" : "mx-1",
                )}
              />
            )}
            {!collapsed && (
              <p className="m-0 px-2.5 pb-1 pt-2.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-[#6d6a74]">
                {group.label}
              </p>
            )}
            {group.items.map((item) => (
              <AdminNavRow
                key={item.href}
                item={item}
                active={isActive(pathname, item)}
                collapsed={collapsed}
                layoutId={layoutId}
                reduce={reduce}
                onNavigate={() => handleNavigate(item.href)}
              />
            ))}
          </div>
        ))}
      </nav>

      <footer
        className={cn(
          "shrink-0 border-t border-[#26242b] py-3",
          collapsed ? "px-2" : "px-4",
        )}
      >
        <Link
          href="/admin/status"
          onClick={() => onNavigate?.("/admin/status")}
          title={collapsed ? "All systems operational" : undefined}
          className={cn(
            "flex items-center text-[12px] text-[#AAA6AE] transition-colors hover:text-[#EEEAF0]",
            collapsed ? "justify-center" : "gap-2",
          )}
        >
          <span className="relative flex size-2 shrink-0">
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-[var(--projects-accent)] opacity-50" />
            <span className="relative inline-flex size-2 rounded-full bg-[var(--projects-accent)]" />
          </span>
          {!collapsed && (
            <span className="truncate">All systems operational</span>
          )}
        </Link>
        <button
          type="button"
          onClick={handleSignOut}
          disabled={logout.isPending}
          title={collapsed ? "Sign out" : undefined}
          className={cn(
            "mt-2.5 flex w-full items-center text-[12px] text-[#AAA6AE] transition-colors hover:text-[#EEEAF0] disabled:opacity-60",
            collapsed ? "justify-center" : "gap-2",
          )}
        >
          <LogOut size={13} strokeWidth={1.8} aria-hidden="true" />
          {!collapsed && (
            <span className="truncate">
              {logout.isPending ? "Signing out…" : "Sign out"}
            </span>
          )}
        </button>
      </footer>
    </>
  );
}

/** Desktop rail (fixed panel) + mobile slide-in sheet, mirroring the
 * customer sidebar's behavior but with admin-only navigation. */
export function AdminSidebar({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const [mounted, setMounted] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const reduce = useReducedMotion() ?? false;

  useEffect(() => {
    setMounted(true);
    setCollapsed(readCollapsedPreference());
  }, []);

  const toggleCollapsed = () => {
    setCollapsed((value) => {
      const next = !value;
      writeCollapsedPreference(next);
      return next;
    });
  };

  // Mobile sheet effect: scroll lock, focus handoff, Escape, focus trap.
  useEffect(() => {
    if (!open) return;
    const opener =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const body = document.body;
    const scrollY = window.scrollY;
    const previous = {
      left: body.style.left,
      overflow: body.style.overflow,
      position: body.style.position,
      right: body.style.right,
      top: body.style.top,
    };
    body.style.position = "fixed";
    body.style.top = `-${scrollY}px`;
    body.style.left = "0";
    body.style.right = "0";
    body.style.overflow = "hidden";

    const frame = requestAnimationFrame(() => {
      const selector = "a[href], button:not([disabled])";
      const first = panelRef.current?.querySelector<HTMLElement>(selector);
      (first ?? panelRef.current)?.focus({ preventScroll: true });
    });

    return () => {
      cancelAnimationFrame(frame);
      body.style.position = previous.position;
      body.style.top = previous.top;
      body.style.left = previous.left;
      body.style.right = previous.right;
      body.style.overflow = previous.overflow;
      window.scrollTo(0, scrollY);
      opener?.focus({ preventScroll: true });
    };
  }, [open]);

  const FOCUSABLE = "a[href], button:not([disabled])";

  return (
    <>
      {/* Desktop rail — admin area owns its own chrome, the customer rail
          never mounts on /admin routes. Width is animated so the content
          column reflows with the rail instead of snapping. */}
      <motion.aside
        aria-label="Admin navigation"
        initial={false}
        animate={{ width: collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH }}
        transition={
          collapsed ? SIDEBAR_COLLAPSE_TRANSITION : SIDEBAR_EXPAND_TRANSITION
        }
        className="sticky top-0 hidden h-dvh shrink-0 flex-col overflow-hidden border-r border-[#322F37] bg-[#121014] lg:flex"
      >
        <AdminSidebarBody collapsed={collapsed} onToggle={toggleCollapsed} />
      </motion.aside>

      {mounted && (
        <div
          className={cn(
            "pointer-events-none fixed inset-0 z-[70] lg:hidden",
            open ? "visible" : "invisible",
          )}
        >
          <motion.div
            initial={false}
            animate={{ opacity: open ? 1 : 0 }}
            transition={open ? PANEL_TRANSITION : PANEL_CLOSE_TRANSITION}
            onClick={onClose}
            aria-hidden="true"
            className={cn(
              "absolute inset-0 bg-black/50",
              open ? "pointer-events-auto" : "pointer-events-none",
            )}
          />
          <motion.div
            ref={panelRef}
            role="dialog"
            aria-modal="true"
            aria-label="Admin navigation"
            aria-hidden={!open}
            tabIndex={-1}
            initial={false}
            animate={{
              opacity: reduce ? (open ? 1 : 0) : 1,
              x: reduce ? 0 : open ? "0%" : "-108%",
            }}
            transition={open ? PANEL_TRANSITION : PANEL_CLOSE_TRANSITION}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                event.preventDefault();
                onClose();
                return;
              }
              if (event.key !== "Tab") return;
              const focusable = panelRef.current
                ? Array.from(
                    panelRef.current.querySelectorAll<HTMLElement>(FOCUSABLE),
                  )
                : [];
              if (focusable.length === 0) {
                event.preventDefault();
                panelRef.current?.focus();
                return;
              }
              const first = focusable[0];
              const last = focusable[focusable.length - 1];
              if (event.shiftKey && document.activeElement === first) {
                event.preventDefault();
                last.focus();
              } else if (!event.shiftKey && document.activeElement === last) {
                event.preventDefault();
                first.focus();
              }
            }}
            className={cn(
              "pointer-events-auto absolute bottom-0 left-0 top-0 flex w-[84vw] max-w-[320px] flex-col overflow-hidden border-r border-[#302E34] bg-[#121014] shadow-[12px_0_32px_rgba(0,0,0,0.45)]",
              !open && "pointer-events-none",
            )}
          >
            <AdminSidebarBody
              onNavigate={() => onClose()}
              onMobileClose={onClose}
            />
          </motion.div>
        </div>
      )}
    </>
  );
}
