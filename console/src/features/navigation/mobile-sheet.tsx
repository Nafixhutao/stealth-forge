"use client";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { motion, useReducedMotion } from "motion/react";
import {
  PANEL_CLOSE_TRANSITION,
  PANEL_TRANSITION,
  REDUCED_TRANSITION,
} from "@/lib/ease";
import { cn } from "@/lib/utils";
import { SidebarContent } from "./sidebar-content";
import type { SidebarProps } from "./types";

const FOCUSABLE_SELECTOR = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

/**
 * Mobile sheet over the beui animated-sidebar pattern: stays mounted for as
 * long as the viewport is mobile and hides itself once the close slide has
 * settled, so opening shows it in the same commit that starts the slide.
 * Esc closes, Tab is trapped inside, the body scroll is locked, and focus
 * returns to the opener on close.
 */
export function MobileSheet({
  open,
  onClose,
  hasTopBar = false,
}: SidebarProps) {
  const reduce = useReducedMotion() ?? false;
  const panelRef = useRef<HTMLElement>(null);
  const [mounted, setMounted] = useState(false);
  const [hidden, setHidden] = useState(!open);
  // The completion callback fires for the open slide too, and it reads state
  // from whenever motion settles: a ref keeps it on the current one.
  const openRef = useRef(open);

  // Portal mounting and open-slide bookkeeping are inherently effect-driven;
  // these updates run once per mount / open transition, not per render.
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => setMounted(true), []);

  useEffect(() => {
    openRef.current = open;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (open) setHidden(false);
  }, [open]);

  useEffect(() => {
    if (!open) return;

    const opener =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const body = document.body;
    const scrollY = window.scrollY;
    const previousBodyStyles = {
      left: body.style.left,
      overflow: body.style.overflow,
      position: body.style.position,
      right: body.style.right,
      top: body.style.top,
    };

    body.style.position = "fixed";
    body.style.top = `-${scrollY}px`;
    body.style.left = "0";
    body.style.right = "0";
    body.style.overflow = "hidden";

    const focusFrame = requestAnimationFrame(() => {
      const firstFocusable =
        panelRef.current?.querySelector<HTMLElement>(FOCUSABLE_SELECTOR);
      (firstFocusable ?? panelRef.current)?.focus({ preventScroll: true });
    });

    return () => {
      cancelAnimationFrame(focusFrame);
      body.style.position = previousBodyStyles.position;
      body.style.top = previousBodyStyles.top;
      body.style.left = previousBodyStyles.left;
      body.style.right = previousBodyStyles.right;
      body.style.overflow = previousBodyStyles.overflow;
      window.scrollTo(0, scrollY);
      opener?.focus({ preventScroll: true });
    };
  }, [open]);

  if (!mounted) return null;

  return createPortal(
    <div
      className={cn(
        "pointer-events-none fixed left-0 top-0 z-50 size-0 lg:hidden",
        hidden && !open ? "invisible" : "visible",
      )}
    >
      <motion.button
        type="button"
        aria-label="Close sidebar overlay"
        tabIndex={open ? 0 : -1}
        initial={false}
        animate={{ opacity: open ? 1 : 0 }}
        transition={
          reduce
            ? REDUCED_TRANSITION
            : open
              ? PANEL_TRANSITION
              : PANEL_CLOSE_TRANSITION
        }
        onClick={onClose}
        className={cn(
          "fixed inset-x-0 bottom-0 bg-black/50",
          hasTopBar ? "top-12" : "top-0",
          open ? "pointer-events-auto" : "pointer-events-none",
        )}
      />
      <motion.aside
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label="Main navigation"
        aria-hidden={!open}
        inert={!open}
        tabIndex={-1}
        data-state={open ? "expanded" : "collapsed"}
        initial={false}
        animate={{
          opacity: reduce ? (open ? 1 : 0) : 1,
          x: reduce ? 0 : open ? "0%" : "-120%",
        }}
        transition={
          reduce
            ? REDUCED_TRANSITION
            : open
              ? PANEL_TRANSITION
              : PANEL_CLOSE_TRANSITION
        }
        onAnimationComplete={() => {
          if (!openRef.current) setHidden(true);
        }}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.preventDefault();
            onClose();
            return;
          }

          if (event.key !== "Tab") return;
          const focusable = panelRef.current
            ? Array.from(
                panelRef.current.querySelectorAll<HTMLElement>(
                  FOCUSABLE_SELECTOR,
                ),
              )
            : [];

          if (focusable.length === 0) {
            event.preventDefault();
            panelRef.current?.focus();
            return;
          }

          const first = focusable[0];
          const last = focusable[focusable.length - 1];
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last.focus();
          } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first.focus();
          }
        }}
        className={cn(
          "pointer-events-auto fixed bottom-0 left-0 flex w-[86vw] max-w-[360px] flex-col overflow-hidden border-r border-[#302E34] bg-[#232127] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.025),12px_0_32px_rgba(0,0,0,0.45)]",
          hasTopBar ? "top-12 h-[calc(100dvh-48px)]" : "top-0 h-dvh",
          !open && "pointer-events-none",
        )}
      >
        <SidebarContent onMobileClose={onClose} />
      </motion.aside>
    </div>,
    document.body,
  );
}
