"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useRouter } from "next/navigation";
import {
  ChevronsUpDown,
  LogOut,
  Settings,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";
import { useCurrentAccount } from "@/api/queries/account";
import { useLogout } from "@/api/mutations/auth";
import {
  LABEL_ENTER_TRANSITION,
  LABEL_EXIT_TRANSITION,
  SPRING_PRESS,
} from "@/lib/ease";
import { cn } from "@/lib/utils";
import { Divider, menuRowClass, RailLabel, tap } from "./sidebar-shared";

const GOO_OPEN_SPRING = {
  type: "spring",
  visualDuration: 0.3,
  bounce: 0.15,
} as const;

const GOO_CLOSE_SPRING = {
  type: "spring",
  visualDuration: 0.21,
  bounce: 0.15,
} as const;

const GOOEY_PANEL_VARIANTS = {
  hidden: {
    opacity: 0,
    scale: 0.96,
    transition: GOO_CLOSE_SPRING,
  },
  show: {
    opacity: 1,
    scale: 1,
    transition: GOO_OPEN_SPRING,
  },
};

function ProfileMenuItem({
  Icon,
  label,
  reduce,
  className,
  onSelect,
}: {
  Icon: LucideIcon;
  label: string;
  reduce: boolean;
  className?: string;
  onSelect?: () => void;
}) {
  return (
    <motion.button
      type="button"
      role="menuitem"
      onClick={onSelect}
      className={cn(menuRowClass, className)}
      whileTap={tap(reduce)}
      transition={SPRING_PRESS}
    >
      <Icon size={15} strokeWidth={1.8} aria-hidden="true" />
      {label}
    </motion.button>
  );
}

function ProfileMenu({
  onClose,
  onNavigate,
}: {
  onClose: () => void;
  onNavigate?: () => void;
}) {
  const account = useCurrentAccount();
  const logout = useLogout();
  const router = useRouter();
  const menuRef = useRef<HTMLDivElement>(null);
  const reduce = useReducedMotion() ?? false;
  const identity = account.data?.account;
  const name =
    identity?.display_name?.trim() ||
    identity?.email?.split("@")[0] ||
    "Account";
  const handle = identity?.email ? `@${identity.email.split("@")[0]}` : "";

  useEffect(() => {
    const el = menuRef.current;
    if (!el) return;

    const onPointerDown = (e: PointerEvent) => {
      if (!el.contains(e.target as Node)) onClose();
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [onClose]);

  const signOut = () => {
    onClose();
    logout.mutate(undefined, {
      onSuccess: () => {
        toast.success("Signed out");
        router.replace("/login");
      },
    });
  };

  return (
    <motion.div
      className="absolute bottom-[calc(100%+5px)] left-3 right-3 z-50"
      style={{ transformOrigin: "bottom left" }}
      variants={reduce ? undefined : GOOEY_PANEL_VARIANTS}
      initial={reduce ? false : "hidden"}
      animate={reduce ? { opacity: 1 } : "show"}
      exit={reduce ? { opacity: 0, transition: { duration: 0.12 } } : "hidden"}
    >
      <div
        ref={menuRef}
        role="menu"
        aria-label="Account menu"
        className="relative z-10 rounded-[12px] border border-[#2d2d35] bg-[#232127] p-1.5 shadow-[0_16px_40px_rgba(0,0,0,0.5)]"
      >
        <div
          aria-hidden="true"
          className="absolute -bottom-[5px] left-4 size-2.5 rotate-45 border-b border-r border-[#2d2d35] bg-[#232127]"
        />

        <div className="flex items-center gap-2.5 px-2.5 py-2.5">
          {identity?.avatar_url ? (
            <motion.img
              alt=""
              initial={false}
              className="size-9 shrink-0 rounded-full object-cover"
              src={identity.avatar_url}
            />
          ) : (
            <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-[#26242b] text-[13px] font-semibold text-[#C9C5CD]">
              {name.slice(0, 1).toUpperCase()}
            </span>
          )}
          <div className="min-w-0 leading-tight">
            <p className="m-0 truncate text-[13px] font-semibold text-[#edecf1]">
              {name}
            </p>
            <p className="m-0 mt-[2px] truncate text-[12px] text-[#8a8791]">
              {handle}
            </p>
          </div>
        </div>

        <Divider />

        <ProfileMenuItem
          Icon={Settings}
          label="Account settings"
          reduce={reduce}
          onSelect={() => {
            onClose();
            onNavigate?.();
            router.push("/account");
          }}
        />
        <ProfileMenuItem
          Icon={LogOut}
          label="Log out"
          reduce={reduce}
          className="text-[#f2708a] hover:text-[#f2708a]"
          onSelect={signOut}
        />
      </div>
    </motion.div>
  );
}

export function BottomProfile({
  collapsed = false,
  onToggleCollapse,
  onNavigate,
}: {
  collapsed?: boolean;
  onToggleCollapse?: () => void;
  onNavigate?: () => void;
}) {
  const account = useCurrentAccount();
  const [open, setOpen] = useState(false);
  const reduce = useReducedMotion() ?? false;
  const close = useCallback(() => setOpen(false), []);
  const identity = account.data?.account;
  const name =
    identity?.display_name?.trim() ||
    identity?.email?.split("@")[0] ||
    "Account";
  const handle = identity?.email ? `@${identity.email.split("@")[0]}` : "";

  return (
    <footer className="relative shrink-0">
      <motion.button
        type="button"
        onClick={() => {
          // the profile menu cannot live in the rail either — unfold first
          if (collapsed) onToggleCollapse?.();
          else setOpen((v) => !v);
        }}
        aria-haspopup="menu"
        aria-expanded={open}
        whileTap={tap(reduce)}
        transition={SPRING_PRESS}
        className="flex w-full items-center px-[14px] py-[10px] text-left transition-colors hover:bg-white/[0.035]"
      >
        {/* rail shows a small 20px avatar, the panel wraps it in the full row */}
        {identity?.avatar_url ? (
          <motion.img
            alt=""
            initial={false}
            animate={{
              width: collapsed ? 20 : 32,
              height: collapsed ? 20 : 32,
            }}
            transition={
              collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION
            }
            className="shrink-0 rounded-full object-cover"
            src={identity.avatar_url}
          />
        ) : (
          <motion.span
            initial={false}
            animate={{
              width: collapsed ? 20 : 32,
              height: collapsed ? 20 : 32,
            }}
            transition={
              collapsed ? LABEL_EXIT_TRANSITION : LABEL_ENTER_TRANSITION
            }
            className="flex shrink-0 items-center justify-center rounded-full bg-[#26242b] text-[11px] font-semibold text-[#C9C5CD]"
          >
            {name.slice(0, 1).toUpperCase()}
          </motion.span>
        )}
        <RailLabel
          collapsed={collapsed}
          className="flex min-w-0 flex-1 items-center"
        >
          <div className="ml-2 min-w-0 leading-tight">
            <p className="m-0 truncate text-[14px] leading-[20px] text-[oklch(0.767_0.0105_305)]">
              {name}
            </p>
            <p className="m-0 mt-[2px] truncate text-[12px] leading-[16px] text-[oklch(0.585_0.0161_305)]">
              {handle}
            </p>
          </div>
          <ChevronsUpDown
            size={12}
            strokeWidth={1.7}
            className="ml-auto shrink-0 text-[#737078]"
            aria-hidden="true"
          />
        </RailLabel>
      </motion.button>
      <AnimatePresence>
        {open && (
          <ProfileMenu
            key="profile-menu"
            onClose={close}
            onNavigate={onNavigate}
          />
        )}
      </AnimatePresence>
    </footer>
  );
}
