"use client";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { motion, useDragControls, useReducedMotion } from "motion/react";
import { EASE_OUT, REDUCED_TRANSITION, SHEET_SPRING } from "@/lib/ease";
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

/** How far, or how fast, the sheet must be pulled before it dismisses. */
const DISMISS_OFFSET = 96;
const DISMISS_VELOCITY = 700;

const SHEET_CLOSE = { duration: 0.2, ease: EASE_OUT } as const;

/**
 * Mobile navigation as a bottom sheet.
 *
 * A phone is held at the bottom, so the navigation arrives from there: the sheet
 * rises over the page on a spring, keeps its own scroll, and can be pulled back
 * down by its handle. It stays mounted while the viewport is mobile and hides
 * itself once the close has settled, so opening shows it in the same commit that
 * starts the motion. Esc closes, Tab is trapped inside, the page behind is
 * locked, and focus returns to the opener on close.
 *
 * Two performance rules hold this together, both learned by measuring it on a
 * throttled CPU rather than by eye:
 *
 * 1. The page lock is `overflow: hidden` on the document element, never
 *    `position: fixed` on the body. Pinning the body to a negative offset makes
 *    the browser re-lay-out and re-paint the entire document — on a project list
 *    that was a 1.6s long task, which is what the sheet's first frame used to
 *    wait behind. Clipping the root keeps the scroll position and touches no
 *    layout at all.
 * 2. Only `transform` and the backdrop's `opacity` animate. Both are compositor
 *    properties, so the sheet moves on the GPU while the main thread is free.
 *    The rounded corner and shadow live on that same composited layer instead of
 *    being re-rasterised every frame.
 */
export function MobileSheet({ open, onClose }: SidebarProps) {
  const reduce = useReducedMotion() ?? false;
  const panelRef = useRef<HTMLElement>(null);
  const dragControls = useDragControls();
  const [mounted, setMounted] = useState(false);

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => setMounted(true), []);

  useEffect(() => {
    if (!open) return;

    const opener =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const root = document.documentElement;
    const previous = {
      overflow: root.style.overflow,
      overscrollBehavior: root.style.overscrollBehavior,
      // Keeping the gutter reserved stops the page from shifting sideways when
      // the scrollbar disappears behind the sheet.
      scrollbarGutter: root.style.scrollbarGutter,
    };

    root.style.overflow = "hidden";
    root.style.overscrollBehavior = "none";
    root.style.scrollbarGutter = "stable";

    const focusFrame = requestAnimationFrame(() => {
      const firstFocusable =
        panelRef.current?.querySelector<HTMLElement>(FOCUSABLE_SELECTOR);
      (firstFocusable ?? panelRef.current)?.focus({ preventScroll: true });
    });

    return () => {
      cancelAnimationFrame(focusFrame);
      root.style.overflow = previous.overflow;
      root.style.overscrollBehavior = previous.overscrollBehavior;
      root.style.scrollbarGutter = previous.scrollbarGutter;
      opener?.focus({ preventScroll: true });
    };
  }, [open]);

  if (!mounted) return null;

  return createPortal(
    <div
      data-user
      className={cn(
        // Stays rendered and painted while closed: it sits a full height below
        // the viewport, so hiding it with `visibility` would only throw away the
        // composited layer and make the next open pay to build it again — that
        // was a 55ms hitch on the first open. `pointer-events-none` and `inert`
        // already keep a closed sheet out of the way.
        "pointer-events-none fixed left-0 top-0 z-50 size-0 lg:hidden",
        !open && "pointer-events-none",
      )}
    >
      <motion.button
        type="button"
        aria-label="Close sidebar overlay"
        tabIndex={open ? 0 : -1}
        initial={false}
        animate={{ opacity: open ? 1 : 0 }}
        transition={reduce ? REDUCED_TRANSITION : SHEET_CLOSE}
        // Dismisses on pointer-down, and only on a keyboard activation for the
        // click path. A pointer-derived click must never be able to close the
        // sheet: it can be the tail of the very tap that opened it, which made
        // the button look dead on a phone. Keyboard activation reports
        // `detail === 0`, so Enter and Space still work.
        onPointerDown={onClose}
        onClick={(event) => {
          if (event.detail === 0) onClose();
        }}
        className={cn(
          "fixed inset-0 bg-black/50",
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
        // `x` stays out of this entirely: the sheet is centred by margins, so
        // motion owns the whole transform and the drag cannot fight a class.
        animate={{
          y: reduce ? 0 : open ? "0%" : "100%",
        }}
        transition={
          reduce ? REDUCED_TRANSITION : open ? SHEET_SPRING : SHEET_CLOSE
        }
        drag={reduce ? false : "y"}
        dragListener={false}
        dragControls={dragControls}
        dragConstraints={{ top: 0, bottom: 0 }}
        dragElastic={{ top: 0, bottom: 0.55 }}
        dragMomentum={false}
        onDragEnd={(_, info) => {
          if (
            info.offset.y > DISMISS_OFFSET ||
            info.velocity.y > DISMISS_VELOCITY
          ) {
            onClose();
          }
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
          // No `translate-y-*` utility here: Tailwind v4 writes the `translate`
          // property, which motion's `transform` cannot override, so the sheet
          // would sit pushed off screen. Motion owns the whole offset, and
          // `initial={false}` means the first painted frame is already correct.
          // `contain: paint` bounds the raster the compositor has to touch while
          // the sheet moves; the shadow is deliberately tight because a wide
          // blur is re-rasterised with the layer and cost a dropped frame at the
          // start of the first open.
          "pointer-events-auto fixed inset-x-0 bottom-0 mx-auto flex h-[82dvh] max-h-[720px] w-full max-w-[560px] flex-col overflow-hidden rounded-t-[20px] border-t border-[#302E34] bg-[#232127] pb-[env(safe-area-inset-bottom)] shadow-[0_-4px_14px_rgba(0,0,0,0.42)] will-change-transform [contain:paint] sm:border-x",
          !open && "pointer-events-none",
        )}
        style={{ transformOrigin: "bottom center" }}
      >
        {/* The handle is the drag target. Closing also works through the
            header's close button, the backdrop, and Esc, so pulling is an
            accelerator rather than the only way out. */}
        <div
          role="presentation"
          onPointerDown={(event) => dragControls.start(event)}
          className="flex shrink-0 cursor-grab touch-none justify-center pb-1 pt-2.5 active:cursor-grabbing"
        >
          <span
            className="h-1 w-9 rounded-full bg-[#4a4650]"
            aria-hidden="true"
          />
        </div>
        <SidebarContent onMobileClose={onClose} entrance={false} />
      </motion.aside>
    </div>,
    document.body,
  );
}
