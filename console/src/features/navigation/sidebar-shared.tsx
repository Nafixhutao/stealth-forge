import type { ReactNode } from "react";
import { motion } from "motion/react";
import { type LucideIcon } from "lucide-react";
import {
  LABEL_ENTER_TRANSITION,
  LABEL_EXIT_TRANSITION,
  SPRING_LAYOUT,
} from "@/lib/ease";
import { cn } from "@/lib/utils";
import type { BadgeKind } from "./types";

export const tap = (reduce: boolean) => (reduce ? undefined : { scale: 0.98 });

/** 16×16 panel glyph from the sidebar header toggle, shared with the mobile
 * nav openers so every entry point carries the same mark. */
export function PanelToggleIcon({
  className = "size-4",
}: {
  className?: string;
}) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      className={className}
      aria-hidden="true"
    >
      <rect
        x="2"
        y="3"
        width="12"
        height="10"
        rx="2"
        stroke="currentColor"
        strokeWidth="2"
      />
      <rect x="4" y="5" width="2" height="6" rx="1" fill="currentColor" />
    </svg>
  );
}

export const NavIcon = (Icon: LucideIcon) => (
  <Icon size={15} strokeWidth={1.8} className="shrink-0 text-[#AAA6AE]" />
);

// `relative isolate` turns the row into its own stacking context so the
// active pill (-z-10) paints above the row background but below the content.
export const rowClass =
  "relative isolate flex h-[34px] w-full items-center gap-[10px] px-4 text-left text-[13px] font-normal text-[#C5C1C9] transition-colors hover:bg-white/[0.035] lg:h-[38px] lg:text-[14px]";

/** Rail-only separator between nav groups: folds away while the panel is
 * expanded (reference rail shows the two dividers at 8px/12px gutters). */
export function RailDivider({ collapsed }: { collapsed: boolean }) {
  return (
    <motion.div
      aria-hidden="true"
      initial={false}
      animate={{
        opacity: collapsed ? 1 : 0,
        height: collapsed ? 1 : 0,
        marginTop: collapsed ? 5 : 0,
        marginBottom: collapsed ? 5 : 0,
      }}
      transition={collapsed ? LABEL_ENTER_TRANSITION : LABEL_EXIT_TRANSITION}
      className="pointer-events-none ml-2 mr-3 overflow-hidden bg-[#322F37]"
    />
  );
}

/** The active-row pill from the beui animated-sidebar: one motion.span per
 * sidebar sharing a layoutId, so switching rows morphs the box across. */
export function ActiveRowPill({
  layoutId,
  reduce,
}: {
  layoutId?: string;
  reduce: boolean;
}) {
  if (!layoutId) return null;
  return (
    <motion.span
      aria-hidden="true"
      layoutId={layoutId}
      transition={reduce ? { duration: 0 } : SPRING_LAYOUT}
      className="absolute inset-0 -z-10 bg-white/[0.06]"
    />
  );
}

/** Crossfades a label while the rail collapses/expands (beui label
 * transitions): leaves fast so the shrinking width never clips mid-word,
 * returns slightly delayed once the panel has opened around it. */
export function RailLabel({
  collapsed,
  className,
  children,
}: {
  collapsed: boolean;
  className?: string;
  children: ReactNode;
}) {
  return (
    <motion.span
      initial={false}
      animate={{ opacity: collapsed ? 0 : 1, x: collapsed ? -6 : 0 }}
      transition={collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION}
      aria-hidden={collapsed}
      className={cn("min-w-0", collapsed && "pointer-events-none", className)}
    >
      {children}
    </motion.span>
  );
}

export function Badge({ kind }: { kind: BadgeKind }) {
  return kind === "New" ? (
    <span className="ml-auto rounded-[6px] bg-[#331E0B] px-[7px] py-[2px] text-[12px] font-medium leading-[16px] text-[rgb(255,160,87)]">
      New
    </span>
  ) : (
    <span className="ml-auto mr-2 rounded-[5px] bg-[#201E22] px-[6px] py-[2px] text-[12px] font-medium leading-[16px] text-[#C9C5CD]">
      Beta
    </span>
  );
}

// Dual chevron that pinches toward its resting position while a submenu is
// open — the expandable affordance reacts to state instead of sitting still.
export function ChevronToggle({
  className = "",
  open = false,
  reduce = false,
}: {
  className?: string;
  open?: boolean;
  reduce?: boolean;
}) {
  const transition = reduce ? { duration: 0 } : SPRING_LAYOUT;
  return (
    <svg
      viewBox="0 0 16 16"
      fill="currentColor"
      className={className}
      aria-hidden="true"
    >
      <motion.path
        fillRule="evenodd"
        clipRule="evenodd"
        initial={false}
        animate={{ y: open ? 0 : -0.6 }}
        transition={transition}
        d="M10.78 5.78a.75.75 0 0 1-1.06 0L8 4.06 6.28 5.78a.75.75 0 0 1-1.06-1.06l2.25-2.25a.75.75 0 0 1 1.06 0l2.25 2.25a.75.75 0 0 1 0 1.06Z"
      />
      <motion.path
        fillRule="evenodd"
        clipRule="evenodd"
        initial={false}
        animate={{ y: open ? 0 : 0.6 }}
        transition={transition}
        d="M5.22 10.22a.75.75 0 0 1 1.06 0L8 11.94l1.72-1.72a.75.75 0 1 1 1.06 1.06l-2.25 2.25a.75.75 0 0 1-1.06 0l-2.25-2.25a.75.75 0 0 1 0-1.06Z"
      />
    </svg>
  );
}

export function BrandIcon({ brand }: { brand: "slack" | "discord" }) {
  if (brand === "slack") {
    return (
      <svg
        aria-hidden="true"
        viewBox="0 0 24 24"
        className="h-[14px] w-[14px] shrink-0 fill-current text-[#AAA6AE] lg:h-[15px] lg:w-[15px]"
      >
        <path d="M6.2 14.5a2.2 2.2 0 1 1-2.2-2.2h2.2v2.2ZM7.3 14.5a2.2 2.2 0 1 1 4.4 0V20a2.2 2.2 0 1 1-4.4 0v-5.5Z" />
        <path d="M9.5 6.2a2.2 2.2 0 1 1 2.2-2.2v2.2H9.5ZM9.5 7.3a2.2 2.2 0 1 1 0 4.4H4a2.2 2.2 0 1 1 0-4.4h5.5Z" />
        <path d="M17.8 9.5A2.2 2.2 0 1 1 20 11.7h-2.2V9.5ZM16.7 9.5a2.2 2.2 0 1 1-4.4 0V4a2.2 2.2 0 1 1 4.4 0v5.5Z" />
        <path d="M14.5 17.8a2.2 2.2 0 1 1-2.2 2.2v-2.2h2.2ZM14.5 16.7a2.2 2.2 0 1 1 0-4.4H20a2.2 2.2 0 1 1 0 4.4h-5.5Z" />
      </svg>
    );
  }

  return (
    <span className="flex size-4 shrink-0 items-center justify-center text-[#AAA6AE] [&>svg]:size-4">
      <svg
        viewBox="-32 -60.5 320 320"
        fill="currentColor"
        aria-hidden="true"
        preserveAspectRatio="xMidYMid"
      >
        <path
          fillRule="nonzero"
          d="M216.9,16.6 C200.3,8.8 182.6,3.2 164,0 C161.8,4.1 159.1,9.6 157.3,14 C137.6,11.1 118.1,11.1 98.7,14 C96.9,9.6 94.2,4.1 91.9,0 C73.4,3.2 55.6,8.9 39,16.6 C5.6,67.1 -3.4,116.4 1.1,165 C23.3,181.5 44.7,191.6 65.9,198.1 C71.1,191 75.7,183.3 79.7,175.3 C72.1,172.4 64.8,168.8 57.9,164.7 C59.7,163.3 61.5,161.9 63.2,160.4 C105.4,180.1 151.1,180.1 192.8,160.4 C194.5,161.9 196.3,163.3 198.1,164.7 C191.2,168.8 183.9,172.4 176.2,175.3 C180.2,183.3 184.9,191 190.1,198.2 C211.2,191.6 232.7,181.5 254.9,165 C260.2,108.7 245.8,59.9 216.9,16.6 ZM85.5,135.1 C72.8,135.1 62.5,123.3 62.5,108.9 C62.5,94.5 72.6,82.7 85.5,82.7 C98.3,82.7 108.7,94.5 108.5,108.9 C108.5,123.3 98.3,135.1 85.5,135.1 ZM170.5,135.1 C157.9,135.1 147.5,123.3 147.5,108.9 C147.5,94.5 157.7,82.7 170.5,82.7 C183.4,82.7 193.8,94.5 193.5,108.9 C193.5,123.3 183.4,135.1 170.5,135.1 Z"
        />
      </svg>
    </span>
  );
}

export const menuRowClass =
  "flex h-8 w-full items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] text-[#b3b0ba] transition-colors hover:bg-[#26262e] hover:text-[#edecf1] [&>svg]:shrink-0";

export function Divider() {
  return <div aria-hidden="true" className="mx-2 my-1 h-px bg-[#2d2d35]" />;
}
