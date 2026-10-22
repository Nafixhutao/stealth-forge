"use client";

import { memo, useCallback, useEffect, useId, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import dynamic from "next/dynamic";
import { motion, useReducedMotion, type Variants } from "motion/react";
import {
  LABEL_ENTER_TRANSITION,
  LABEL_EXIT_TRANSITION,
  POWER2_OUT,
  SPRING_LAYOUT,
  SPRING_PRESS,
} from "@/lib/ease";
import { cn } from "@/lib/utils";
import { useCurrentAccount } from "@/api/queries/account";
import { useOrganizationPlan } from "@/api/queries/organizations";
import { planLabel, planUsageText } from "@/lib/plans";
import { useCurrentOrganizationId } from "./use-current-organization";
import { FolderKanban, Search, X } from "lucide-react";
import { BottomProfile } from "./profile-menu";
import {
  ActiveRowPill,
  Badge,
  ChevronToggle,
  NavIcon,
  RailLabel,
  rowClass,
  tap,
} from "./sidebar-shared";
import type { NavItem, NavRowProps } from "./types";

// Original entrance: nav rows stagger in from y:8.
const NAV_VARIANTS: Variants = {
  hidden: {},
  visible: { transition: { staggerChildren: 0.04 } },
};

const NAV_ITEM_VARIANTS: Variants = {
  hidden: { y: 8, opacity: 0 },
  visible: {
    y: 0,
    opacity: 1,
    transition: { duration: 0.35, ease: POWER2_OUT },
  },
};

/**
 * The rows the user console can actually open.
 *
 * Everything else that used to sit here — Explore, Agent, Analytics, Slack,
 * Discord, Security, Plan, Account, Documentation, Contact Support — had no
 * destination: the console has no route for any of them, so a click either did
 * nothing or (for Agent) answered 404. They come back one at a time, in the
 * commit that ships the surface behind them.
 */
const primaryNavItems: NavItem[] = [
  {
    icon: NavIcon(Search),
    label: "Search",
    labelClassName: "text-[14px] leading-[20px] text-[oklch(0.949_0.0035_305)]",
  },
  { icon: NavIcon(FolderKanban), label: "Projects" },
];

// The command palette is 245 lines plus 14 icons and renders nothing until the
// user opens it, so it is loaded on intent instead of shipping in every page's
// bundle. `loadSearchPalette` is exposed separately so hover/focus can warm the
// chunk before the user commits to opening it.
const loadSearchPalette = () => import("./search-palette");
const SearchPalette = dynamic(
  () => loadSearchPalette().then((module) => module.SearchPalette),
  { ssr: false },
);

/** The user console's project surfaces: its home and any single project. */
function isProjectsPath(pathname: string) {
  return pathname === "/user" || pathname.startsWith("/projects/");
}

function NavRow({
  icon,
  label,
  badge,
  expandable = false,
  labelClassName = "",
  isActive = false,
  layoutId,
  onSelect,
  onIntent,
  collapsed = false,
}: NavRowProps) {
  const reduce = useReducedMotion() ?? false;
  return (
    // layout wrapper lets rows below an opening/closing submenu glide
    <motion.div
      layout="position"
      transition={SPRING_LAYOUT}
      variants={NAV_ITEM_VARIANTS}
    >
      <motion.button
        type="button"
        className={rowClass}
        onClick={onSelect}
        onHoverStart={onIntent}
        onFocus={onIntent}
        aria-current={isActive ? "page" : undefined}
        whileTap={tap(reduce)}
        transition={SPRING_PRESS}
      >
        {isActive && <ActiveRowPill layoutId={layoutId} reduce={reduce} />}
        {icon}
        <RailLabel
          collapsed={collapsed}
          className={`flex-1 origin-left truncate ${labelClassName}`}
        >
          {label}
        </RailLabel>
        {badge && (
          <motion.span
            initial={false}
            aria-hidden={collapsed}
            animate={{
              opacity: collapsed ? 0 : 1,
              maxWidth: collapsed ? 0 : 80,
            }}
            transition={
              collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION
            }
            className="overflow-hidden"
          >
            <Badge kind={badge} />
          </motion.span>
        )}
        {expandable && (
          <motion.span
            initial={false}
            aria-hidden={collapsed}
            animate={{
              opacity: collapsed ? 0 : 1,
              maxWidth: collapsed ? 0 : 20,
            }}
            transition={
              collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION
            }
            className="overflow-hidden"
          >
            <ChevronToggle className="size-[13px] shrink-0 text-[#737078]" />
          </motion.span>
        )}
      </motion.button>
    </motion.div>
  );
}

function Header({
  onClose,
  collapsed = false,
  onToggleCollapse,
}: {
  onClose?: () => void;
  collapsed?: boolean;
  onToggleCollapse?: () => void;
}) {
  const reduce = useReducedMotion() ?? false;
  const account = useCurrentAccount();
  const organizationId = useCurrentOrganizationId();
  const plan = useOrganizationPlan(organizationId ?? undefined);
  const identity = account.data?.account;
  const name =
    identity?.display_name?.trim() ||
    identity?.email?.split("@")[0] ||
    "Account";
  const avatar = identity?.avatar_url ?? "";

  // The chip reads the plan endpoint for whichever organization this surface
  // belongs to. It renders nothing until that answer exists: an empty slot is
  // honest, a placeholder that always says the same word is not.
  const orgPlan = plan.data?.plan;
  const label = planLabel(orgPlan?.plan_key);
  const planTitle = orgPlan
    ? planUsageText(orgPlan.usage.projects, orgPlan.limits.projects)
    : undefined;

  return (
    <header className="flex h-[44px] shrink-0 items-center gap-2 px-4 lg:h-[48px]">
      {/* The identity slot folds away entirely in the rail — it must not
       * reserve space, or it pushes the collapse toggle out of the rail.
       *
       * The ceiling is the widest panel this header can appear in, not a
       * design width: at 220px the name was cut short ("Aulia Rahma…") while
       * the row still had empty space beside it. */}
      <motion.div
        initial={false}
        animate={{ maxWidth: collapsed ? 0 : 560 }}
        transition={collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION}
        className="flex min-w-0 flex-1 items-center overflow-hidden"
      >
        {avatar ? (
          <motion.img
            alt=""
            initial={false}
            animate={{ opacity: collapsed ? 0 : 1 }}
            transition={
              collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION
            }
            className="size-5 shrink-0 rounded-md object-cover"
            src={avatar}
          />
        ) : (
          <span className="flex size-5 shrink-0 items-center justify-center rounded-md bg-[#2b2930] text-[10px] font-semibold text-[#edecf1]">
            {name.slice(0, 1).toUpperCase()}
          </span>
        )}
        <RailLabel
          collapsed={collapsed}
          className="flex min-w-0 flex-1 items-center"
        >
          <span
            title={name}
            className="ml-2 min-w-0 flex-1 truncate text-[13.5px] font-medium leading-[20px] tracking-[-0.01em] text-[oklch(0.949_0.0035_305)]"
          >
            {name}
          </span>
          {label ? (
            <span
              title={planTitle}
              className="ml-2 shrink-0 whitespace-nowrap rounded-[5px] bg-[#201E22] px-[6px] py-[2px] text-[12px] font-medium leading-[16px] text-[oklch(0.767_0.0105_305)]"
            >
              {label}
            </span>
          ) : null}
        </RailLabel>
      </motion.div>
      {onClose ? (
        <motion.button
          type="button"
          onClick={onClose}
          whileTap={tap(reduce)}
          transition={SPRING_PRESS}
          className="-mr-2 inline-flex size-11 shrink-0 items-center justify-center rounded-lg text-[#AAA6AE] transition-colors hover:bg-white/[0.06] hover:text-[#EEEAF0] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/30 lg:hidden"
          aria-label="Close sidebar"
        >
          <X size={16} strokeWidth={1.8} />
        </motion.button>
      ) : null}
      {onToggleCollapse ? (
        <motion.button
          type="button"
          onClick={onToggleCollapse}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          aria-expanded={!collapsed}
          whileTap={tap(reduce)}
          transition={SPRING_PRESS}
          className={cn(
            "group/btn hidden size-7 shrink-0 items-center justify-center rounded-md text-[#AAA6AE] transition-colors hover:bg-white/[0.06] hover:text-[#EEEAF0] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/30 lg:inline-flex",
            // in the rail the toggle belongs to the icon column (reference
            // center 23.5); only the expanded panel pins it to the right edge
            collapsed ? "mr-auto" : "ml-auto",
          )}
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 16 16"
            fill="none"
            className="size-4"
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
            <rect
              x="4"
              y="5"
              width="2"
              height="6"
              rx="1"
              fill="currentColor"
              className="transition-[width] duration-300 ease-out [width:2px] group-hover/btn:[width:1px]"
            />
          </svg>
        </motion.button>
      ) : null}
    </header>
  );
}

/** Everything inside the panel, shared by the desktop rail and mobile sheet. */
function SidebarContentImpl({
  onMobileClose,
  collapsed = false,
  onToggleCollapse,
  entrance = true,
}: {
  onMobileClose?: () => void;
  collapsed?: boolean;
  onToggleCollapse?: () => void;
  /**
   * Whether the nav rows play their staggered entrance. The desktop rail
   * mounts once per page load and earns the flourish; the mobile sheet is
   * opened repeatedly and is already the thing that just animated, so it opts
   * out and paints its rows in the first frame.
   */
  entrance?: boolean;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [active, setActive] = useState<string | null>(() =>
    isProjectsPath(pathname) ? "Projects" : null,
  );
  const [searchOpen, setSearchOpen] = useState(false);
  // The palette is mounted only once it is first wanted, so its chunk is not
  // pulled into every page load.
  const [searchMounted, setSearchMounted] = useState(false);

  const openSearch = useCallback(() => {
    setSearchMounted(true);
    setSearchOpen(true);
  }, []);

  // Warms the chunk on hover/focus so opening the palette is instant even the
  // first time. Deliberately does not open it.
  const preloadSearch = useCallback(() => {
    setSearchMounted(true);
    void loadSearchPalette();
  }, []);
  const pillId = useId();
  const reduce = useReducedMotion() ?? false;

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (isProjectsPath(pathname)) setActive("Projects");
  }, [pathname]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        openSearch();
      }
    };
    window.addEventListener("open-command-palette", openSearch);
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("open-command-palette", openSearch);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [openSearch]);

  // Selecting from the rail navigates without unfolding the panel, so the
  // sidebar stays closed when jumping to another page.
  const select = (label: string) => {
    setActive(label);
  };

  const selectNavigation = (label: string) => {
    select(label);
    // "Projects" is the user console's front page. It used to be the root path,
    // which now belongs to the admin console, so a user tapping it landed in
    // the wrong product entirely.
    if (label === "Projects" && pathname !== "/user") router.push("/user");
    // The mobile sheet covers most of the screen, so choosing a destination
    // dismisses it; on the desktop rail this prop is undefined.
    onMobileClose?.();
  };

  const renderNavItem = (item: NavItem) => (
    <NavRow
      key={item.label}
      {...item}
      isActive={active === item.label}
      layoutId={pillId}
      onSelect={() => {
        if (item.label === "Search") openSearch();
        else selectNavigation(item.label);
      }}
      onIntent={item.label === "Search" ? preloadSearch : undefined}
      collapsed={collapsed}
    />
  );

  return (
    <>
      <Header
        onClose={onMobileClose}
        collapsed={collapsed}
        onToggleCollapse={onToggleCollapse}
      />
      <motion.nav
        className="sidebar-scrollbar min-h-0 flex-1 overflow-x-hidden overflow-y-auto pb-1 [overscroll-behavior:contain]"
        aria-label="Sidebar links"
        variants={NAV_VARIANTS}
        initial={reduce || !entrance ? false : "hidden"}
        animate="visible"
      >
        {primaryNavItems.map(renderNavItem)}
      </motion.nav>
      <BottomProfile
        collapsed={collapsed}
        onToggleCollapse={onToggleCollapse}
      />
      {searchMounted ? (
        <SearchPalette
          open={searchOpen}
          onClose={() => setSearchOpen(false)}
          onSelect={(result) => {
            setSearchOpen(false);
            // The palette only emits entries that name a real destination.
            if (result.navigateTo) selectNavigation(result.navigateTo);
          }}
        />
      ) : null}
    </>
  );
}

/**
 * Memoised on purpose: opening the mobile sheet re-renders its parent, and
 * without this the whole nav tree (176 nodes, 25 icons) re-rendered in the same
 * commit that starts the animation — a 183ms frame on a throttled CPU, right
 * where the motion needed to begin. Its props are stable, so nothing is lost.
 */
export const SidebarContent = memo(SidebarContentImpl);
