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
export function PanelToggleIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" className={className} aria-hidden="true">
      <rect x="2" y="3" width="12" height="10" rx="2" stroke="currentColor" strokeWidth="2" />
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
export function ActiveRowPill({ layoutId, reduce }: { layoutId?: string; reduce: boolean }) {
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
    <svg viewBox="0 0 16 16" fill="currentColor" className={className} aria-hidden="true">
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
      <svg aria-hidden="true" viewBox="0 0 24 24" className="h-[14px] w-[14px] shrink-0 fill-current text-[#AAA6AE] lg:h-[15px] lg:w-[15px]">
        <path d="M6.2 14.5a2.2 2.2 0 1 1-2.2-2.2h2.2v2.2ZM7.3 14.5a2.2 2.2 0 1 1 4.4 0V20a2.2 2.2 0 1 1-4.4 0v-5.5Z" />
        <path d="M9.5 6.2a2.2 2.2 0 1 1 2.2-2.2v2.2H9.5ZM9.5 7.3a2.2 2.2 0 1 1 0 4.4H4a2.2 2.2 0 1 1 0-4.4h5.5Z" />
        <path d="M17.8 9.5A2.2 2.2 0 1 1 20 11.7h-2.2V9.5ZM16.7 9.5a2.2 2.2 0 1 1-4.4 0V4a2.2 2.2 0 1 1 4.4 0v5.5Z" />
        <path d="M14.5 17.8a2.2 2.2 0 1 1-2.2 2.2v-2.2h2.2ZM14.5 16.7a2.2 2.2 0 1 1 0-4.4H20a2.2 2.2 0 1 1 0 4.4h-5.5Z" />
      </svg>
    );
  }

  return (
    <span className="flex size-4 shrink-0 items-center justify-center text-[#AAA6AE] [&>svg]:size-4">
      <svg viewBox="-32 -60.5 320 320" fill="currentColor" aria-hidden="true" preserveAspectRatio="xMidYMid">
        <path
          fillRule="nonzero"
          d="M216.856339,16.5966031 C200.285002,8.84328665 182.566144,3.2084988 164.041564,0 C161.766523,4.11318106 159.108624,9.64549908 157.276099,14.0464379 C137.583995,11.0849896 118.072967,11.0849896 98.7430163,14.0464379 C96.9108417,9.64549908 94.1925838,4.11318106 91.8971895,0 C73.3526068,3.2084988 55.6133949,8.86399117 39.0420583,16.6376612 C5.61752293,67.146514 -3.4433191,116.400813 1.08711069,164.955721 C23.2560196,181.510915 44.7403634,191.567697 65.8621325,198.148576 C71.0772151,190.971126 75.7283628,183.341335 79.7352139,175.300261 C72.104019,172.400575 64.7949724,168.822202 57.8887866,164.667963 C59.7209612,163.310589 61.5131304,161.891452 63.2445898,160.431257 C105.36741,180.133187 151.134928,180.133187 192.754523,160.431257 C194.506336,161.891452 196.298154,163.310589 198.110326,164.667963 C191.183787,168.842556 183.854737,172.420929 176.223542,175.320965 C180.230393,183.341335 184.861538,190.991831 190.096624,198.16893 C211.238746,191.588051 232.743023,181.531619 254.911949,164.955721 C260.227747,108.666201 245.831087,59.8662432 216.856339,16.5966031 ZM85.4738752,135.09489 C72.8290281,135.09489 62.4592217,123.290155 62.4592217,108.914901 C62.4592217,94.5396472 72.607595,82.7145857 85.4738752,82.7145857 C98.3405064,82.7145857 108.709962,94.5189427 108.488529,108.914901 C108.508531,123.290155 98.3405064,135.09489 85.4738752,135.09489 ZM170.525237,135.09489 C157.88039,135.09489 147.510584,123.290155 147.510584,108.914901 C147.510584,94.5396472 157.658606,82.7145857 170.525237,82.7145857 C183.391518,82.7145857 193.761324,94.5189427 193.539891,108.914901 C193.539891,123.290155 183.391518,135.09489 170.525237,135.09489 Z"
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
