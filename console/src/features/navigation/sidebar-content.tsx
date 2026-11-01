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
import {
  organizationItems,
  useOrganizations,
  useOrganizationPlan,
} from "@/api/queries/organizations";
import { planLabel, planUsageText } from "@/lib/plans";
import { useCurrentOrganizationId } from "./use-current-organization";
import {
  ChartPie,
  ChevronsUpDown,
  Code,
  GitPullRequestArrow,
  Gift,
  Inbox,
  ScanFace,
  Search,
  Settings,
  ShieldCheck,
  X,
  Zap,
} from "lucide-react";
import { BottomProfile } from "./profile-menu";
import {
  ActiveRowPill,
  Badge,
  BrandIcon,
  ChevronToggle,
  NavIcon,
  PanelToggleIcon,
  RailLabel,
  rowClass,
  rowWidthClass,
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

/** The reference sidebar renders search as a filled field — no border, the
 * shortcut sits as plain muted text on the right. */
function SearchField({
  onOpen,
  onIntent,
  collapsed = false,
}: {
  onOpen: () => void;
  onIntent: () => void;
  collapsed?: boolean;
}) {
  const reduce = useReducedMotion() ?? false;
  return (
    <motion.button
      type="button"
      onClick={onOpen}
      onHoverStart={onIntent}
      onFocus={onIntent}
      whileTap={tap(reduce)}
      transition={SPRING_PRESS}
      aria-hidden={collapsed}
      tabIndex={collapsed ? -1 : undefined}
      className={cn(
        "flex items-center gap-2 rounded-[6px] text-left transition-colors",
        collapsed
          ? "mx-auto size-8 justify-center px-0 hover:bg-white/[0.05]"
          : "mx-2 mt-1.5 h-10 w-[calc(100%-1rem)] bg-[#171519] pl-2 pr-3.5 hover:bg-[#1C1A1F] lg:h-8",
      )}
    >
      <Search
        size={collapsed ? 18 : 16}
        strokeWidth={1.7}
        className="shrink-0 text-[#B5B2B9]"
        aria-hidden="true"
      />
      {!collapsed ? (
        <>
          <span className="min-w-0 flex-1 truncate text-[14px] font-medium leading-5 text-[#EEEDF0]">
            Search
          </span>
          <span className="shrink-0 text-[12px] font-[500] leading-[15px] text-[#B5B2B9]">
            Ctrl K
          </span>
        </>
      ) : null}
    </motion.button>
  );
}

/**
 * The navigation the reference sidebar draws, in its order.
 *
 * None of these surfaces has a route yet, so the rows hold the selection
 * locally instead of promising a destination; each becomes a link in the
 * commit that ships its surface. The bottom group's Organization settings is
 * the one row that opens a real surface (`/user`, where the switcher lives).
 */
const primaryNavItems: NavItem[] = [
  { icon: NavIcon(Inbox), label: "Triage" },
  { icon: NavIcon(GitPullRequestArrow), label: "Reviews" },
  {
    icon: NavIcon(Code),
    label: "Coding Agent",
    trailing: (
      <Gift
        size={16}
        strokeWidth={1.7}
        className="shrink-0 text-[#FFA057]"
        aria-hidden="true"
      />
    ),
  },
  { icon: NavIcon(ShieldCheck), label: "Security" },
  { icon: NavIcon(ChartPie), label: "Analytics" },
  { icon: NavIcon(Zap), label: "Automations" },
  { icon: <BrandIcon brand="slack" />, label: "Slack Bot" },
];

const secondaryNavItems: NavItem[] = [
  { icon: NavIcon(ScanFace), label: "Scopes" },
  { icon: NavIcon(Settings), label: "Organization settings" },
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

function NavRow({
  icon,
  label,
  badge,
  expandable = false,
  labelClassName = "",
  trailing,
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
        className={cn(
          rowClass,
          rowWidthClass[collapsed ? "collapsed" : "expanded"],
          // the rail has nothing beside the glyph, so the label gap would push
          // the icon off-centre even though the label itself is zero-width
          collapsed ? "gap-0" : "gap-1.5",
        )}
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
          className={cn(
            "origin-left truncate",
            collapsed ? "w-0 flex-none overflow-hidden" : "flex-1",
            labelClassName,
          )}
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
        {trailing ? (
          <motion.span
            initial={false}
            aria-hidden={collapsed}
            animate={{
              opacity: collapsed ? 0 : 1,
              maxWidth: collapsed ? 0 : 24,
            }}
            transition={
              collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION
            }
            className="flex shrink-0 items-center overflow-hidden"
          >
            {trailing}
          </motion.span>
        ) : null}
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
  const organizations = useOrganizations();
  const organizationId = useCurrentOrganizationId();
  const plan = useOrganizationPlan(organizationId ?? undefined);
  const pathname = usePathname();
  const router = useRouter();
  const identity = account.data?.account;
  const accountName =
    identity?.display_name?.trim() ||
    identity?.email?.split("@")[0] ||
    "Account";
  const avatar = identity?.avatar_url ?? "";

  // The selector names the organization the surface belongs to, falling back
  // to the account while the organization list is still in flight. The first
  // organization is only a preview when no organization is selected yet; a
  // selected id missing from the loaded page must not borrow another name.
  const orgs = organizationItems(organizations.data);
  const organization =
    orgs.find((item) => item.id === organizationId) ??
    (organizationId ? null : (orgs[0] ?? null));
  const name = organization?.name?.trim() || accountName;

  // The chip reads the plan endpoint for whichever organization this surface
  // belongs to. It renders nothing until that answer exists: an empty slot is
  // honest, a placeholder that always says the same word is not.
  const orgPlan = plan.data?.plan;
  const label = planLabel(orgPlan?.plan_key);
  const planTitle = orgPlan
    ? planUsageText(orgPlan.usage.projects, orgPlan.limits.projects)
    : undefined;

  return (
    <header
      className={cn(
        "flex h-12 shrink-0 items-center",
        collapsed ? "gap-0 px-0" : "gap-2 px-4",
      )}
    >
      {/* The identity slot folds away entirely in the rail — it must not
       * reserve space, or it pushes the collapse toggle out of the rail. */}
      <motion.div
        initial={false}
        animate={{ maxWidth: collapsed ? 0 : 560 }}
        transition={collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION}
        className="flex min-w-0 flex-1 items-center overflow-hidden"
      >
        <button
          type="button"
          onClick={() => {
            if (pathname !== "/user") router.push("/user");
          }}
          aria-label="Switch organization"
          className="flex min-w-0 items-center gap-2 rounded-md text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/30"
        >
          {avatar ? (
            <motion.img
              alt=""
              initial={false}
              animate={{ opacity: collapsed ? 0 : 1 }}
              transition={
                collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION
              }
              className="size-5 shrink-0 rounded-[6px] object-cover"
              src={avatar}
            />
          ) : (
            <span className="flex size-5 shrink-0 items-center justify-center rounded-[6px] bg-[#2b2930] text-[10px] font-semibold text-[#EEEDF0]">
              {name.slice(0, 1).toUpperCase()}
            </span>
          )}
          <span
            title={name}
            className="min-w-0 shrink truncate text-[14px] font-semibold leading-5 tracking-[-0.01em] text-[#EEEDF0]"
          >
            {name}
          </span>
          {label ? (
            <span
              title={planTitle}
              className="inline-flex h-5 shrink-0 items-center whitespace-nowrap rounded-[4px] bg-[#1F1D21] px-1.5 text-[12px] font-medium leading-[15px] text-[#B5B2B9]"
            >
              {label}
            </span>
          ) : null}
          <ChevronsUpDown
            size={14}
            strokeWidth={1.7}
            className="shrink-0 text-[#7E7A84]"
            aria-hidden="true"
          />
        </button>
      </motion.div>
      {onClose ? (
        <motion.button
          type="button"
          onClick={onClose}
          whileTap={tap(reduce)}
          transition={SPRING_PRESS}
          className="-mr-2 inline-flex size-8 shrink-0 items-center justify-center rounded-md text-[#B5B2B9] transition-colors hover:bg-white/[0.06] hover:text-[#EEEDF0] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/30 lg:hidden"
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
            "group/btn hidden size-8 shrink-0 items-center justify-center rounded-md text-[#B5B2B9] transition-colors hover:bg-white/[0.06] hover:text-[#EEEDF0] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/30 lg:inline-flex",
            // in the rail the toggle belongs to the icon column (center of the
            // 52px rail); only the expanded panel pins it to the right edge
            collapsed ? "mx-auto" : "ml-auto -mr-2",
          )}
        >
          <PanelToggleIcon className="size-[18px]" />
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
  // The reference opens on Triage; the console has no route for the other
  // surfaces yet, so the selection is local until they ship.
  const [active, setActive] = useState<string | null>("Triage");
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
        else if (item.label === "Organization settings") {
          // The organization home is the closest real surface: it is where the
          // organization is named and switched.
          setActive(item.label);
          if (pathname !== "/user") router.push("/user");
          onMobileClose?.();
        } else select(item.label);
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
      <SearchField
        onOpen={openSearch}
        onIntent={preloadSearch}
        collapsed={collapsed}
      />
      <motion.nav
        className="sidebar-scrollbar mt-2 flex min-h-0 flex-1 flex-col gap-0.5 overflow-x-hidden overflow-y-auto pb-2 [overscroll-behavior:contain]"
        aria-label="Sidebar links"
        variants={NAV_VARIANTS}
        initial={reduce || !entrance ? false : "hidden"}
        animate="visible"
      >
        {primaryNavItems.map(renderNavItem)}
      </motion.nav>
      {/* Bottom group: pinned to the base of the panel, never part of the
       * scrolling list. The two hairlines are inset 8px like the reference. */}
      <div className="shrink-0">
        <div aria-hidden="true" className="mx-2 h-px bg-[#322F37]" />
        <nav
          aria-label="Organization links"
          className="flex flex-col gap-0.5 pt-2"
        >
          {secondaryNavItems.map(renderNavItem)}
        </nav>
        <div aria-hidden="true" className="mx-2 h-px bg-[#322F37]" />
        <BottomProfile
          collapsed={collapsed}
          onToggleCollapse={onToggleCollapse}
        />
      </div>
      {searchMounted ? (
        <SearchPalette
          open={searchOpen}
          onClose={() => setSearchOpen(false)}
          onSelect={(result) => {
            setSearchOpen(false);
            // The palette only emits entries that name a real destination.
            if (result.navigateTo) {
              router.push(result.navigateTo);
              onMobileClose?.();
            }
          }}
        />
      ) : null}
    </>
  );
}

/**
 * Memoised on purpose: opening the mobile sheet re-renders its parent, and
 * without this the whole nav tree re-rendered in the same commit that starts
 * the animation — a 183ms frame on a throttled CPU, right where the motion
 * needed to begin. Its props are stable, so nothing is lost.
 */
export const SidebarContent = memo(SidebarContentImpl);
