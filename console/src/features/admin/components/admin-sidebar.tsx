"use client";

import { useEffect, useId, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { motion, useReducedMotion } from "motion/react";
import {
  Activity,
  ArrowLeft,
  Bug,
  ChartNoAxesCombined,
  BrainCircuit,
  Gauge,
  Logs,
  RadioTower,
  Server,
  Settings,
  ShieldCheck,
  TriangleAlert,
  Users,
  Waypoints,
  X,
  type LucideIcon,
} from "lucide-react";
import { PANEL_CLOSE_TRANSITION, PANEL_TRANSITION, SPRING_LAYOUT, SPRING_PRESS } from "@/lib/ease";
import { cn } from "@/lib/utils";

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

function AdminNavRow({
  item,
  active,
  layoutId,
  reduce,
  onNavigate,
}: {
  item: AdminNavItem;
  active: boolean;
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
        className={cn(
          "relative isolate flex h-[32px] w-full items-center gap-2.5 px-4 text-left text-[13px] transition-colors",
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
            className="absolute inset-0 -z-10 bg-[color-mix(in_srgb,var(--projects-accent)_10%,transparent)]"
          />
        )}
        {active && <span aria-hidden="true" className="absolute inset-y-[7px] left-0 w-[2px] rounded-full bg-[var(--projects-accent)]" />}
        <Icon size={15} strokeWidth={1.8} className={cn("shrink-0", active ? "text-[var(--projects-accent)]" : "text-[#AAA6AE]")} />
        <span className="truncate">{item.label}</span>
      </motion.button>
    </motion.div>
  );
}

function AdminSidebarBody({
  onNavigate,
  onMobileClose,
}: {
  /** Fires after a navigation link is chosen (mobile closes the sheet). */
  onNavigate?: (href: string) => void;
  onMobileClose?: () => void;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const layoutId = useId();
  const reduce = useReducedMotion() ?? false;

  const handleNavigate = (href: string) => {
    router.push(href);
    onNavigate?.(href);
  };

  return (
    <>
      <header className="flex h-[48px] shrink-0 items-center gap-2.5 px-4">
        <span className="flex size-6 shrink-0 items-center justify-center rounded-md border border-[color-mix(in_srgb,var(--projects-accent)_35%,var(--projects-border))] bg-[color-mix(in_srgb,var(--projects-accent)_10%,transparent)] text-[var(--projects-accent)]">
          <ShieldCheck size={13} strokeWidth={2} aria-hidden="true" />
        </span>
        <span className="min-w-0 flex-1 truncate text-[13.5px] font-semibold tracking-[-0.01em] text-[#EEEAF0]">
          Admin Console
        </span>
        <span className="admin-mono shrink-0 rounded-[5px] border border-[#322F37] px-[5px] py-[2px] text-[9.5px] font-medium leading-none text-[#AAA6AE]">
          PROD
        </span>
        <button
          type="button"
          onClick={onMobileClose}
          aria-label="Close sidebar"
          className="inline-flex size-6 shrink-0 items-center justify-center text-[#AAA6AE] transition-colors hover:text-[#EEEAF0] lg:hidden"
        >
          <X size={14} strokeWidth={1.8} />
        </button>
      </header>

      <nav aria-label="Admin navigation" className="sidebar-scrollbar min-h-0 flex-1 overflow-y-auto overflow-x-hidden pb-2">
        {ADMIN_NAV.map((group, groupIndex) => (
          <div key={group.label}>
            {groupIndex > 0 && <div aria-hidden="true" className="mx-4 my-2 h-px bg-[#26242b]" />}
            <p className="m-0 px-4 pb-1 pt-2.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-[#6d6a74]">
              {group.label}
            </p>
            {group.items.map((item) => (
              <AdminNavRow
                key={item.href}
                item={item}
                active={isActive(pathname, item)}
                layoutId={layoutId}
                reduce={reduce}
                onNavigate={() => handleNavigate(item.href)}
              />
            ))}
          </div>
        ))}
      </nav>

      <footer className="shrink-0 border-t border-[#26242b] px-4 py-3">
        <Link
          href="/admin/status"
          onClick={() => onNavigate?.("/admin/status")}
          className="flex items-center gap-2 text-[12px] text-[#AAA6AE] transition-colors hover:text-[#EEEAF0]"
        >
          <span className="relative flex size-2">
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-[var(--projects-accent)] opacity-50" />
            <span className="relative inline-flex size-2 rounded-full bg-[var(--projects-accent)]" />
          </span>
          All systems operational
        </Link>
        <Link
          href="/"
          className="mt-2.5 flex items-center gap-2 text-[12px] text-[#AAA6AE] transition-colors hover:text-[#EEEAF0]"
        >
          <ArrowLeft size={13} strokeWidth={1.8} aria-hidden="true" />
          Back to app
        </Link>
      </footer>
    </>
  );
}

/** Desktop rail (fixed panel) + mobile slide-in sheet, mirroring the
 * customer sidebar's behavior but with admin-only navigation. */
export function AdminSidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [mounted, setMounted] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const reduce = useReducedMotion() ?? false;

  useEffect(() => setMounted(true), []);

  // Mobile sheet effect: scroll lock, focus handoff, Escape, focus trap.
  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
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
      {/* Desktop panel — admin area owns its own chrome, the customer rail
          never mounts on /admin routes. */}
      <aside
        aria-label="Admin navigation"
        className="sticky top-0 hidden h-dvh w-[228px] shrink-0 flex-col overflow-hidden border-r border-[#322F37] bg-[#121014] lg:flex"
      >
        <AdminSidebarBody />
      </aside>

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
            className={cn("absolute inset-0 bg-black/50", open ? "pointer-events-auto" : "pointer-events-none")}
          />
          <motion.div
            ref={panelRef}
            role="dialog"
            aria-modal="true"
            aria-label="Admin navigation"
            aria-hidden={!open}
            tabIndex={-1}
            initial={false}
            animate={{ opacity: reduce ? (open ? 1 : 0) : 1, x: reduce ? 0 : open ? "0%" : "-108%" }}
            transition={open ? PANEL_TRANSITION : PANEL_CLOSE_TRANSITION}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                event.preventDefault();
                onClose();
                return;
              }
              if (event.key !== "Tab") return;
              const focusable = panelRef.current ? Array.from(panelRef.current.querySelectorAll<HTMLElement>(FOCUSABLE)) : [];
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
            <AdminSidebarBody onNavigate={() => onClose()} onMobileClose={onClose} />
          </motion.div>
        </div>
      )}
    </>
  );
}
