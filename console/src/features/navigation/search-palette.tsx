"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  BookOpen,
  Bot,
  Building2,
  CircleDollarSign,
  Database,
  FileText,
  FolderKanban,
  Headphones,
  LayoutDashboard,
  LogOut,
  Search,
  ShieldCheck,
  UserRound,
  type LucideIcon,
} from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { EASE_OUT } from "@/lib/ease";
import { cn } from "@/lib/utils";

type SearchResult = {
  label: string;
  section: string;
  Icon: LucideIcon;
};

const SEARCH_RESULTS: SearchResult[] = [
  { section: "Navigation", label: "Go to Repositories", Icon: Database },
  { section: "Navigation", label: "Go to Projects", Icon: FolderKanban },
  { section: "Navigation", label: "Go to Agents", Icon: Bot },
  { section: "Navigation", label: "Go to Dashboard", Icon: LayoutDashboard },
  {
    section: "Settings",
    label: "Go to Organization Settings",
    Icon: Building2,
  },
  { section: "Settings", label: "Go to Billing", Icon: CircleDollarSign },
  { section: "Account", label: "Go to Profile", Icon: UserRound },
  { section: "Account", label: "Logout", Icon: LogOut },
  { section: "Help Center", label: "Go to Documentation", Icon: BookOpen },
  { section: "Help Center", label: "Contact Support", Icon: Headphones },
  { section: "Help Center", label: "Keyboard Shortcuts", Icon: FileText },
  { section: "Help Center", label: "Release Notes", Icon: FileText },
  { section: "Help Center", label: "Status", Icon: ShieldCheck },
  { section: "Help Center", label: "API Reference", Icon: BookOpen },
  { section: "Help Center", label: "Report a Problem", Icon: Headphones },
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

  useEffect(() => {
    if (!open) return;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      } else if (event.key === "ArrowDown" && results.length > 0) {
        event.preventDefault();
        setActiveIndex((index) => (index + 1) % results.length);
      } else if (event.key === "ArrowUp" && results.length > 0) {
        event.preventDefault();
        setActiveIndex(
          (index) => (index - 1 + results.length) % results.length,
        );
      } else if (event.key === "Enter" && results[activeIndex]) {
        event.preventDefault();
        onSelect(results[activeIndex]);
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [activeIndex, onClose, onSelect, open, results]);

  useEffect(() => {
    if (!open) return;

    // Lock the page scroll without losing the position: setting overflow on
    // <html> resets scrollY to 0, so pin the body and restore on close
    // (same pattern as the mobile sheet).
    const scrollY = window.scrollY;
    const body = document.body;
    const previousStyles = {
      position: body.style.position,
      top: body.style.top,
      left: body.style.left,
      right: body.style.right,
      overflow: body.style.overflow,
    };

    body.style.position = "fixed";
    body.style.top = `-${scrollY}px`;
    body.style.left = "0";
    body.style.right = "0";
    body.style.overflow = "hidden";

    return () => {
      body.style.position = previousStyles.position;
      body.style.top = previousStyles.top;
      body.style.left = previousStyles.left;
      body.style.right = previousStyles.right;
      body.style.overflow = previousStyles.overflow;
      window.scrollTo(0, scrollY);
    };
  }, [open]);

  return (
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
                          onClick={() => onSelect(result)}
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
    </AnimatePresence>
  );
}
