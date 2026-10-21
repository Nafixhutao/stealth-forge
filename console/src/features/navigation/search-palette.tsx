"use client";

import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import {
  FolderKanban,
  LogOut,
  Search,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useLogout } from "@/api/mutations/auth";
import { EASE_OUT } from "@/lib/ease";
import { cn } from "@/lib/utils";

/**
 * One palette entry.
 *
 * Every entry does something: either it names a route the navigation knows how
 * to open (`navigateTo`), or the palette performs it itself (`run`). The list
 * used to carry fifteen destinations — Repositories, Agents, Dashboard,
 * Organization Settings, Billing, Documentation, Support, Release Notes — that
 * had no route behind them, so choosing one closed the palette and changed
 * nothing. They come back when their surfaces do.
 */
type SearchResult = {
  label: string;
  section: string;
  Icon: LucideIcon;
  /** A destination the sidebar can open. */
  navigateTo?: string;
  /** An action the palette carries out itself. */
  run?: "logout" | "status";
};

const SEARCH_RESULTS: SearchResult[] = [
  {
    section: "Navigation",
    label: "Go to Projects",
    Icon: FolderKanban,
    navigateTo: "Projects",
  },
  {
    section: "Help Center",
    label: "Status page",
    Icon: ShieldCheck,
    run: "status",
  },
  { section: "Account", label: "Log out", Icon: LogOut, run: "logout" },
];

export function SearchPalette({
  open,
  onClose,
  onSelect,
}: {
  open: boolean;
  onClose: () => void;
  onSelect: (result: SearchResult) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const reduce = useReducedMotion() ?? false;
  const router = useRouter();
  const logout = useLogout();
  const [mounted, setMounted] = useState(false);

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => setMounted(true), []);

  // Entries that carry their own action never reach the sidebar, so a palette
  // result cannot be a dead end by construction.
  const select = (result: SearchResult) => {
    if (result.run === "status") {
      onClose();
      // The status site is a separate static document served by the ingress at
      // /status/, not a route in this Next.js app, so the router cannot reach
      // it: a full navigation is the correct one here.
      // eslint-disable-next-line @next/next/no-location-assign-relative-destination
      window.location.assign("/status/");
      return;
    }
    if (result.run === "logout") {
      onClose();
      logout.mutate(undefined, {
        onSuccess: () => {
          toast.success("Signed out");
          router.replace("/login");
        },
      });
      return;
    }
    onSelect(result);
  };

  const groups = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    const matches = SEARCH_RESULTS.filter((result) =>
      result.label.toLowerCase().includes(normalizedQuery),
    );
    return Array.from(new Set(matches.map((result) => result.section))).map(
      (section) => ({
        section,
        results: matches.filter((result) => result.section === section),
      }),
    );
  }, [query]);

  // Index is computed once per keystroke instead of searching per rendered
  // item, keeping lookup O(1) instead of O(n) inside the render loop.
  const { results, indexByResult } = useMemo(() => {
    const flat = groups.flatMap((group) => group.results);
    return {
      results: flat,
      indexByResult: new Map(flat.map((result, index) => [result, index])),
    };
  }, [groups]);

  useEffect(() => {
    if (!open) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setQuery("");
    setActiveIndex(0);
    const focusTimer = window.setTimeout(() => inputRef.current?.focus(), 0);
    return () => window.clearTimeout(focusTimer);
  }, [open]);

  // Reads the latest index, results and handlers without re-subscribing: the
  // listener is attached once per open instead of on every keystroke.
  const onKeyDown = useEffectEvent((event: KeyboardEvent) => {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
    } else if (event.key === "ArrowDown" && results.length > 0) {
      event.preventDefault();
      setActiveIndex((index) => (index + 1) % results.length);
    } else if (event.key === "ArrowUp" && results.length > 0) {
      event.preventDefault();
      setActiveIndex((index) => (index - 1 + results.length) % results.length);
    } else if (event.key === "Enter" && results[activeIndex]) {
      event.preventDefault();
      select(results[activeIndex]);
    }
  });

  useEffect(() => {
    if (!open) return;
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open]);

  useEffect(() => {
    if (!open) return;

    // Lock the page scroll by clipping the root, the same way the mobile sheet
    // does. Pinning the body to a negative offset re-lays-out the whole
    // document, which is a long task on a large page.
    const root = document.documentElement;
    const previous = {
      overflow: root.style.overflow,
      overscrollBehavior: root.style.overscrollBehavior,
      scrollbarGutter: root.style.scrollbarGutter,
    };

    root.style.overflow = "hidden";
    root.style.overscrollBehavior = "none";
    root.style.scrollbarGutter = "stable";

    return () => {
      root.style.overflow = previous.overflow;
      root.style.overscrollBehavior = previous.overscrollBehavior;
      root.style.scrollbarGutter = previous.scrollbarGutter;
    };
  }, [open]);

  // Portalled to the document body. The palette is rendered inside the
  // sidebar, and the sidebar is animated with a transform; a transformed
  // ancestor becomes the containing block for `position: fixed` descendants, so
  // the palette was laid out relative to the off-screen drawer and opened
  // outside the viewport.
  if (!mounted) return null;

  return createPortal(
    <AnimatePresence>
      {open && (
        <motion.div
          key="search-backdrop"
          className="fixed inset-0 z-[100] flex items-start justify-center overscroll-contain bg-black/45 px-4 pt-[20vh] backdrop-blur-[3px]"
          initial={reduce ? false : { opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={
            reduce
              ? { opacity: 0 }
              : { opacity: 0, transition: { duration: 0.16 } }
          }
          onMouseDown={onClose}
          role="presentation"
        >
          <motion.div
            role="dialog"
            aria-modal="true"
            aria-label="Search"
            className="w-full max-w-[520px] overflow-hidden rounded-[8px] border border-[#3a373f] bg-[#232127] shadow-[0_0_0_1px_rgba(255,255,255,0.025),0_24px_80px_rgba(0,0,0,0.6)]"
            initial={reduce ? false : { opacity: 0, y: -10, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={reduce ? { opacity: 0 } : { opacity: 0, y: -8, scale: 0.98 }}
            transition={{ duration: 0.2, ease: EASE_OUT }}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="flex h-12 items-center gap-3 border-b border-[#322F37] px-3.5">
              <Search
                size={16}
                strokeWidth={1.8}
                className="shrink-0 text-[#8a8791]"
                aria-hidden="true"
              />
              <input
                ref={inputRef}
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  setActiveIndex(0);
                }}
                placeholder="Search..."
                aria-label="Search commands"
                className="min-w-0 flex-1 bg-transparent text-[14px] font-normal leading-[20px] text-[oklch(0.949_0.0035_305)] outline-none placeholder:text-[#8a8791]"
              />
            </div>

            <div className="search-palette-scrollbar h-[304px] overscroll-contain overflow-x-hidden overflow-y-auto p-1.5">
              {groups.length > 0 ? (
                groups.map((group) => (
                  <section key={group.section}>
                    <h2 className="px-2 py-2 text-[12px] font-normal leading-[16px] text-[oklch(0.585_0.0161_305)]">
                      {group.section}
                    </h2>
                    {group.results.map((result) => {
                      const index = indexByResult.get(result) ?? 0;
                      const active = index === activeIndex;
                      return (
                        <button
                          type="button"
                          key={result.label}
                          onClick={() => select(result)}
                          onMouseEnter={() => setActiveIndex(index)}
                          className={cn(
                            "flex h-8 w-full items-center gap-3 rounded-[5px] px-2.5 text-left text-[14px] font-normal leading-[20px] text-[oklch(0.949_0.0035_305)] transition-colors",
                            active ? "bg-[#302e34]" : "hover:bg-[#2b2930]",
                          )}
                        >
                          <result.Icon
                            size={15}
                            strokeWidth={1.8}
                            className="shrink-0 text-[#aaa6ae]"
                            aria-hidden="true"
                          />
                          <span className="truncate">{result.label}</span>
                        </button>
                      );
                    })}
                  </section>
                ))
              ) : (
                <p className="px-2.5 py-8 text-center text-[13px] text-[#8a8791]">
                  No results found.
                </p>
              )}
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>,
    document.body,
  );
}
