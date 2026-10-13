"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useRouter } from "next/navigation";
import {
  ChevronsUpDown,
  ExternalLink,
  LogOut,
  Monitor,
  Moon,
  Sun,
  User,
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

type Theme = "light" | "dark" | "system";

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

function ThemeToggle({
  theme,
  onToggle,
  reduce,
}: {
  theme: Theme;
  onToggle: (theme: Theme) => void;
  reduce: boolean;
}) {
  const options: Array<{ value: Theme; label: string; Icon: LucideIcon }> = [
    { value: "light", label: "Light mode", Icon: Sun },
    { value: "dark", label: "Dark mode", Icon: Moon },
    { value: "system", label: "System theme", Icon: Monitor },
  ];

  const selectTheme = (nextTheme: Theme) => {
    if (nextTheme === theme) return;

    if (reduce || !("startViewTransition" in document)) {
      onToggle(nextTheme);
      return;
    }

    const root = document.documentElement;
    root.style.setProperty("--beui-vt-origin", "50% 100%");
    root.setAttribute("data-beui-vt", "circle-blur");
    const transition = (
      document as Document & {
        startViewTransition(callback: () => void): { finished: Promise<void> };
      }
    ).startViewTransition(() => onToggle(nextTheme));

    transition.finished.finally(() => {
      root.removeAttribute("data-beui-vt");
    });
  };

  return (
    <div className="flex items-center gap-0.5 rounded-full border border-[#3a373f] bg-[#1a181d] p-0.5">
      {options.map(({ value, label, Icon }) => {
        const selected = theme === value;
        return (
          <motion.button
            key={value}
            type="button"
            aria-label={label}
            aria-pressed={selected}
            onClick={() => selectTheme(value)}
            whileTap={tap(reduce)}
            transition={SPRING_PRESS}
            className={cn(
              "flex size-6 items-center justify-center rounded-full transition-colors",
              selected
                ? "bg-white text-[#1a181d]"
                : "text-[#aaa6ae] hover:bg-white/[0.08] hover:text-white",
            )}
          >
            <AnimatePresence initial={false} mode="wait">
              {selected ? (
                <motion.span
                  key={value}
                  initial={
                    reduce
                      ? false
                      : { opacity: 0, filter: "blur(8px)", scale: 0.7 }
                  }
                  animate={{ opacity: 1, filter: "blur(0px)", scale: 1 }}
                  exit={
                    reduce
                      ? undefined
                      : { opacity: 0, filter: "blur(8px)", scale: 0.7 }
                  }
                  transition={{ duration: 0.2, ease: "easeInOut" }}
                  className="flex"
                >
                  <Icon size={14} strokeWidth={2} aria-hidden="true" />
                </motion.span>
              ) : (
                <Icon
                  key={`${value}-idle`}
                  size={14}
                  strokeWidth={1.8}
                  aria-hidden="true"
                />
              )}
            </AnimatePresence>
          </motion.button>
        );
      })}
    </div>
  );
}

/** The signed-in account's display identity, shared by the header and footer. */
function useIdentity() {
  const account = useCurrentAccount();
  const identity = account.data?.account;
  const name =
    identity?.display_name?.trim() ||
    identity?.email?.split("@")[0] ||
    "Account";
  return {
    name,
    handle: identity?.email ?? "",
    avatar: identity?.avatar_url ?? "",
  };
}

function ProfileMenu({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const logout = useLogout();
  const { name, handle, avatar } = useIdentity();
  const menuRef = useRef<HTMLDivElement>(null);
  const [theme, setTheme] = useState<Theme>(() => {
    if (typeof document === "undefined") return "dark";
    return document.documentElement.dataset.theme === "light"
      ? "light"
      : "dark";
  });
  const reduce = useReducedMotion() ?? false;

  useEffect(() => {
    const resolved =
      theme === "system"
        ? window.matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "light"
        : theme;
    document.documentElement.dataset.theme = resolved;
  }, [theme]);

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
          {avatar ? (
            /* eslint-disable-next-line @next/next/no-img-element */
            <img
              alt=""
              className="size-9 shrink-0 rounded-full object-cover"
              src={avatar}
            />
          ) : (
            <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-[#2b2930] text-[13px] font-semibold text-[#edecf1]">
              {name.slice(0, 1).toUpperCase()}
            </span>
          )}
          <div className="min-w-0 leading-tight">
            <p className="m-0 truncate text-[13px] font-semibold text-[#edecf1]">
              {name}
            </p>
            {handle ? (
              <p className="m-0 mt-[2px] truncate text-[12px] text-[#8a8791]">
                {handle}
              </p>
            ) : null}
          </div>
        </div>

        <Divider />

        <div className="flex items-center justify-between px-2.5 py-2">
          <span className="text-[13px] text-[#b3b0ba]">Theme</span>
          <ThemeToggle theme={theme} reduce={reduce} onToggle={setTheme} />
        </div>

        <Divider />

        <ProfileMenuItem Icon={User} label="Profile Settings" reduce={reduce} />
        <ProfileMenuItem
          Icon={ExternalLink}
          label="Refer and Earn"
          reduce={reduce}
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
}: {
  collapsed?: boolean;
  onToggleCollapse?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const reduce = useReducedMotion() ?? false;
  const close = useCallback(() => setOpen(false), []);
  const { name, handle, avatar } = useIdentity();

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
        {avatar ? (
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
            className="shrink-0 object-cover rounded-full"
            src={avatar}
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
            className="flex shrink-0 items-center justify-center rounded-full bg-[#2b2930] text-[12px] font-semibold text-[#edecf1]"
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
            {handle ? (
              <p className="m-0 mt-[2px] truncate text-[12px] leading-[16px] text-[oklch(0.585_0.0161_305)]">
                {handle}
              </p>
            ) : null}
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
        {open && <ProfileMenu key="profile-menu" onClose={close} />}
      </AnimatePresence>
    </footer>
  );
}
