"use client";

import { useId, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import {
  AnimatePresence,
  motion,
  useReducedMotion,
  type Variants,
} from "motion/react";
import {
  EASE_OUT,
  LABEL_ENTER_TRANSITION,
  LABEL_EXIT_TRANSITION,
  POWER2_INOUT,
  POWER2_OUT,
  REDUCED_TRANSITION,
  SPRING_LAYOUT,
  SPRING_PRESS,
  SUBMENU_TRANSITION,
} from "@/lib/ease";
import { cn } from "@/lib/utils";
import { ChevronDown, Search } from "lucide-react";
import { useCurrentAccount } from "@/api/queries/account";
import { BottomProfile } from "./profile-menu";
import { SearchPalette } from "./search-palette";
import {
  ActiveRowPill,
  Badge,
  ChevronToggle,
  NavIcon,
  RailDivider,
  RailLabel,
  rowClass,
  tap,
} from "./sidebar-shared";
import { useUserNav, type NavGroupData, type NavLink } from "./nav-data";
import type { NavRowProps } from "./types";

// Open keeps the original morph: clip-path reveal with a staggered blur-up.
// The close is GSAP-style — the measured height tweens to 0 while fading, so
// the rows below ride up with the collapsing box instead of waiting out an
// empty reserved gap.
const SUBMENU_VARIANTS: Variants = {
  closed: {
    opacity: 0,
    clipPath: "inset(0px 0px 100% 0)",
  },
  open: {
    opacity: 1,
    clipPath: "inset(0px 0px 0% 0)",
    transition: {
      duration: 0.2,
      delayChildren: 0.05,
      ease: EASE_OUT,
      staggerChildren: 0.035,
    },
  },
};

const SUBMENU_ITEM_VARIANTS: Variants = {
  closed: { opacity: 0, y: 4, filter: "blur(3px)" },
  open: {
    opacity: 1,
    y: 0,
    filter: "blur(0px)",
    transition: SUBMENU_TRANSITION,
  },
};

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

function isActive(pathname: string, href: string) {
  return pathname === href || (href !== "/" && pathname.startsWith(`${href}/`));
}

function NavRow({
  icon,
  label,
  href,
  badge,
  expandable = false,
  labelClassName = "",
  isActive: active = false,
  layoutId,
  onSelect,
  collapsed = false,
}: NavRowProps) {
  const reduce = useReducedMotion() ?? false;
  const router = useRouter();
  const select = () => {
    onSelect?.();
    if (href) router.push(href);
  };
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
        onClick={select}
        aria-current={active ? "page" : undefined}
        whileTap={tap(reduce)}
        transition={SPRING_PRESS}
      >
        {active && <ActiveRowPill layoutId={layoutId} reduce={reduce} />}
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
  const account = useCurrentAccount();
  const reduce = useReducedMotion() ?? false;
  const identity = account.data?.account;
  const name =
    identity?.display_name?.trim() ||
    identity?.email?.split("@")[0] ||
    "Account";
  const role =
    identity?.instance_role === "instance_owner"
      ? "Owner"
      : identity?.instance_role === "instance_admin"
        ? "Admin"
        : null;

  return (
    <header className="flex h-[44px] shrink-0 items-center px-4 lg:h-[48px]">
      {/* the identity slot folds away entirely in the rail — it must not
       * reserve space, or it pushes the collapse toggle out of the rail */}
      <motion.div
        initial={false}
        animate={{ maxWidth: collapsed ? 0 : 220 }}
        transition={collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION}
        className="flex min-w-0 items-center overflow-hidden"
      >
        {identity?.avatar_url ? (
          <motion.img
            alt=""
            initial={false}
            animate={{ opacity: collapsed ? 0 : 1 }}
            transition={
              collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION
            }
            className="size-5 shrink-0 rounded-md object-cover"
            src={identity.avatar_url}
          />
        ) : (
          <span className="flex size-5 shrink-0 items-center justify-center rounded-md bg-[#26242b] text-[10px] font-semibold text-[#C9C5CD]">
            {name.slice(0, 1).toUpperCase()}
          </span>
        )}
        <RailLabel collapsed={collapsed} className="flex items-center">
          <span className="ml-2 text-[14px] font-medium leading-[20px] tracking-[-0.01em] text-[oklch(0.949_0.0035_305)]">
            {name}
          </span>
          {role ? (
            <span className="ml-2 shrink-0 rounded-[5px] bg-[#201E22] px-[6px] py-[2px] text-[12px] font-medium leading-[16px] text-[oklch(0.767_0.0105_305)]">
              {role}
            </span>
          ) : null}
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2.2}
            strokeLinecap="round"
            strokeLinejoin="round"
            className="ml-2 size-[13px] shrink-0 text-[#737078]"
            aria-hidden="true"
          >
            <path d="m7 8 5-5 5 5" />
            <path d="m7 16 5 5 5-5" />
          </svg>
        </RailLabel>
      </motion.div>
      <motion.button
        type="button"
        onClick={onClose}
        whileTap={tap(reduce)}
        transition={SPRING_PRESS}
        className="ml-auto inline-flex h-6 w-6 items-center justify-center text-[#AAA6AE] transition-colors hover:text-[#EEEAF0] lg:hidden"
        aria-label="Close sidebar"
      >
        <svg
          viewBox="0 0 16 16"
          className="size-[14px]"
          fill="none"
          aria-hidden="true"
        >
          <path
            d="M4 4l8 8M12 4l-8 8"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
          />
        </svg>
      </motion.button>
      <motion.button
        type="button"
        onClick={onToggleCollapse}
        aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        aria-expanded={!collapsed}
        whileTap={tap(reduce)}
        transition={SPRING_PRESS}
        className={cn(
          "group/btn hidden shrink-0 text-[#AAA6AE] transition-colors hover:text-[#EEEAF0] lg:block",
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
    </header>
  );
}

function NavGroupSection({
  group,
  pathname,
  open,
  onToggle,
  onNavigate,
  collapsed = false,
}: {
  group: NavGroupData;
  pathname: string;
  open: boolean;
  onToggle: () => void;
  onNavigate?: () => void;
  collapsed?: boolean;
}) {
  const reduce = useReducedMotion() ?? false;
  const router = useRouter();
  const submenuRef = useRef<HTMLDivElement>(null);
  const groupActive = group.items.some((item) => isActive(pathname, item.href));

  // A resting filter forces a compositing layer and flips the submenu text
  // from subpixel to grayscale antialiasing. The clip-path stays.
  const clearSubmenuArtifacts = () => {
    submenuRef.current
      ?.querySelectorAll("button")
      .forEach((b) => b.style.removeProperty("filter"));
  };

  return (
    <motion.section
      layout="position"
      transition={SPRING_LAYOUT}
      variants={NAV_ITEM_VARIANTS}
    >
      <motion.button
        type="button"
        className={rowClass}
        onClick={onToggle}
        aria-expanded={open}
        aria-current={groupActive && !open ? "page" : undefined}
        whileTap={tap(reduce)}
        transition={SPRING_PRESS}
      >
        {groupActive && <ActiveRowPill reduce={reduce} />}
        {group.icon}
        <RailLabel collapsed={collapsed} className="flex-1">
          {group.label}
        </RailLabel>
        <motion.span
          initial={false}
          aria-hidden={collapsed}
          animate={{ opacity: collapsed ? 0 : 1, maxWidth: collapsed ? 0 : 20 }}
          transition={
            collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION
          }
          className="overflow-hidden"
        >
          <ChevronToggle
            className="size-[13px] shrink-0 text-[#737078]"
            open={open}
            reduce={reduce}
          />
        </motion.span>
      </motion.button>
      <AnimatePresence>
        {open && !collapsed && (
          <motion.div
            key="submenu"
            ref={submenuRef}
            className="ml-[22px] overflow-hidden border-l border-[#3A373F] pl-[20px]"
            variants={reduce ? undefined : SUBMENU_VARIANTS}
            initial={reduce ? false : "closed"}
            animate={reduce ? { opacity: 1 } : "open"}
            exit={
              reduce
                ? { opacity: 0, transition: { duration: 0.12 } }
                : {
                    opacity: 0,
                    height: 0,
                    transition: { duration: 0.28, ease: POWER2_INOUT },
                  }
            }
            onAnimationComplete={() => {
              if (!open) return;
              clearSubmenuArtifacts();
              setTimeout(clearSubmenuArtifacts, 500);
            }}
          >
            {group.items.map((item) => {
              const active = isActive(pathname, item.href);
              return (
                <motion.button
                  type="button"
                  key={item.href}
                  variants={reduce ? undefined : SUBMENU_ITEM_VARIANTS}
                  onClick={() => {
                    onNavigate?.();
                    router.push(item.href);
                  }}
                  aria-current={active ? "page" : undefined}
                  whileTap={tap(reduce)}
                  transition={SPRING_PRESS}
                  className={`flex h-[30px] w-full items-center text-left text-[12px] font-normal transition-colors lg:h-[34px] lg:text-[14px] ${
                    active
                      ? "bg-white/[0.03] text-[#EEEAF0]"
                      : "text-[#C5C1C9] hover:text-[#EEEAF0]"
                  }`}
                >
                  <span>{item.label}</span>
                  {item.badge && <Badge kind={item.badge} />}
                </motion.button>
              );
            })}
          </motion.div>
        )}
      </AnimatePresence>
    </motion.section>
  );
}

/** Everything inside the panel, shared by the desktop rail and mobile sheet. */
export function SidebarContent({
  onMobileClose,
  collapsed = false,
  showHeader = true,
  onToggleCollapse,
}: {
  onMobileClose?: () => void;
  collapsed?: boolean;
  showHeader?: boolean;
  onToggleCollapse?: () => void;
}) {
  const pathname = usePathname();
  const nav = useUserNav();
  const [searchOpen, setSearchOpen] = useState(false);
  const pillId = useId();
  const reduce = useReducedMotion() ?? false;

  const activeGroup = nav.groups.find((group) =>
    group.items.some((item) => isActive(pathname, item.href)),
  )?.label;
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({});
  const groupOpen = (group: NavGroupData) =>
    openGroups[group.label] ?? activeGroup === group.label;
  const anyGroupOpen = nav.groups.some((group) => groupOpen(group));

  const renderNavItem = (item: NavLink) => (
    <NavRow
      key={item.href}
      {...item}
      isActive={isActive(pathname, item.href)}
      layoutId={pillId}
      collapsed={collapsed}
    />
  );

  return (
    <>
      {showHeader ? (
        <Header
          onClose={onMobileClose}
          collapsed={collapsed}
          onToggleCollapse={onToggleCollapse}
        />
      ) : null}
      <motion.nav
        className="sidebar-scrollbar min-h-0 flex-1 overflow-x-hidden overflow-y-auto pb-1"
        aria-label="Sidebar links"
        variants={NAV_VARIANTS}
        initial={reduce ? false : "hidden"}
        animate="visible"
      >
        <NavRow
          icon={NavIcon(Search)}
          label="Search"
          labelClassName="text-[14px] leading-[20px] text-[oklch(0.949_0.0035_305)]"
          layoutId={pillId}
          onSelect={() => setSearchOpen(true)}
          collapsed={collapsed}
        />
        {nav.primary.map(renderNavItem)}
        {nav.groups.length > 0 ? <RailDivider collapsed={collapsed} /> : null}
        {nav.groups.map((group) => (
          <NavGroupSection
            key={group.label}
            group={group}
            pathname={pathname}
            open={groupOpen(group)}
            onToggle={() =>
              setOpenGroups((prev) => ({
                ...prev,
                [group.label]: !(
                  prev[group.label] ?? activeGroup === group.label
                ),
              }))
            }
            onNavigate={onMobileClose}
            collapsed={collapsed}
          />
        ))}
        {nav.admin ? (
          <>
            <RailDivider collapsed={collapsed} />
            {renderNavItem(nav.admin)}
          </>
        ) : null}
      </motion.nav>
      <AnimatePresence>
        {nav.groups.length > 0 && !collapsed && (
          <motion.div
            key="view-more"
            initial={reduce ? false : { opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{
              opacity: 0,
              y: 6,
              transition: { duration: 0.18, ease: EASE_OUT },
            }}
            transition={
              reduce ? REDUCED_TRANSITION : { duration: 0.25, ease: EASE_OUT }
            }
            className="relative flex shrink-0 items-center justify-center px-2 py-3"
          >
            <div className="absolute inset-x-2 h-px bg-[#322F37]" />
            <motion.button
              type="button"
              whileTap={tap(reduce)}
              transition={SPRING_PRESS}
              onClick={() =>
                setOpenGroups(
                  anyGroupOpen
                    ? {}
                    : Object.fromEntries(
                        nav.groups.map((group) => [group.label, true]),
                      ),
                )
              }
              className="relative z-10 inline-flex h-[26px] shrink-0 items-center gap-1.5 rounded-full border border-[#322F37] bg-[#121014] px-[10px] text-[12px] font-medium leading-[16px] text-[oklch(0.949_0.0035_305)] transition-colors hover:border-[#4a4650] hover:bg-[#121014]"
            >
              <ChevronDown size={12} strokeWidth={2} className="shrink-0" />
              View more
            </motion.button>
          </motion.div>
        )}
      </AnimatePresence>
      <BottomProfile
        collapsed={collapsed}
        onToggleCollapse={onToggleCollapse}
      />
      <SearchPalette
        open={searchOpen}
        onClose={() => setSearchOpen(false)}
        results={nav.primary
          .concat(nav.groups.flatMap((group) => group.items))
          .map((item) => ({
            label: `Go to ${item.label}`,
            section: "Navigation",
            href: item.href,
            Icon: item.icon,
          }))}
      />
    </>
  );
}
