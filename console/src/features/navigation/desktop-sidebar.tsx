"use client";

import { useEffect, useState } from "react";
import { motion, useReducedMotion } from "motion/react";
import {
  REDUCED_TRANSITION,
  SIDEBAR_COLLAPSE_TRANSITION,
  SIDEBAR_EXPAND_TRANSITION,
} from "@/lib/ease";
import { cn } from "@/lib/utils";
import { SidebarContent } from "./sidebar-content";

/** Desktop rail width when collapsed to icons only — measured from the
 * reference rail capture: 51px content + 1px border. */
const RAIL_WIDTH = 52;
/** Expanded desktop sidebar width. */
const PANEL_WIDTH = 268;

// Module-level so the rail choice survives SPA route changes, where every
// page mounts its own DesktopSidebar instance and would reset to expanded.
let collapsedAcrossPages = false;

export function DesktopSidebar({ hasTopBar = false }: { hasTopBar?: boolean }) {
  const [collapsed, setCollapsed] = useState(collapsedAcrossPages);
  const reduce = useReducedMotion() ?? false;

  const toggleCollapse = () => {
    setCollapsed((value) => {
      collapsedAcrossPages = !value;
      return !value;
    });
  };

  useEffect(() => {
    window.addEventListener("toggle-desktop-sidebar", toggleCollapse);
    return () =>
      window.removeEventListener("toggle-desktop-sidebar", toggleCollapse);
    // The updater reads the current state via the functional form, so the
    // listener staying on the first render's closure is safe.
  }, []);

  return (
    // in-flow so the page content rides along while the width eases
    // between the icon rail and the full panel (beui collapsible="icon")
    <motion.aside
      aria-label="Main navigation"
      data-state={collapsed ? "collapsed" : "expanded"}
      initial={false}
      animate={{ width: collapsed ? RAIL_WIDTH : PANEL_WIDTH }}
      transition={
        reduce
          ? REDUCED_TRANSITION
          : collapsed
            ? SIDEBAR_COLLAPSE_TRANSITION
            : SIDEBAR_EXPAND_TRANSITION
      }
      className="relative z-50 hidden shrink-0 lg:block"
    >
      <div
        className={cn(
          "sticky flex w-full flex-col overflow-hidden border-r border-[#322F37] bg-[#121014]",
          hasTopBar ? "top-12 h-[calc(100dvh-48px)]" : "top-0 h-dvh",
        )}
      >
        <SidebarContent
          collapsed={collapsed}
          onToggleCollapse={toggleCollapse}
        />
      </div>
    </motion.aside>
  );
}
