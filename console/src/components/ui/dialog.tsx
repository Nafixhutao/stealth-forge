"use client";

import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

export const Dialog = DialogPrimitive.Root;
export const DialogTrigger = DialogPrimitive.Trigger;
export const DialogClose = DialogPrimitive.Close;

export const DialogContent = ({
  className,
  children,
  showClose = true,
  animated = true,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & {
  showClose?: boolean;
  /**
   * Dialogs are "occasional" and earn an entrance. Set false for keyboard-
   * initiated surfaces (the command palette) that must never animate.
   */
  animated?: boolean;
}) => (
  <DialogPrimitive.Portal>
    <DialogPrimitive.Overlay
      className={cn(
        "fixed inset-0 z-50 bg-void/80",
        animated &&
          "data-[state=open]:animate-overlay-in data-[state=closed]:animate-overlay-out",
      )}
    />
    <DialogPrimitive.Content
      className={cn(
        "fixed left-1/2 top-1/2 z-50 max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2 overflow-y-auto overscroll-contain rounded-xl border border-graphite bg-obsidian p-6 shadow-xl",
        animated &&
          "data-[state=open]:animate-dialog-in data-[state=closed]:animate-dialog-out",
        className,
      )}
      {...props}
    >
      {children}
      {showClose ? (
        <DialogPrimitive.Close
          className="absolute right-3 top-3 flex size-11 items-center justify-center rounded-md text-fog transition-colors duration-150 hover:bg-white/[0.06] hover:text-paper focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-acid-lime/50"
          aria-label="Close dialog"
        >
          <X className="size-4" aria-hidden="true" />
        </DialogPrimitive.Close>
      ) : null}
    </DialogPrimitive.Content>
  </DialogPrimitive.Portal>
);

export const DialogHeader = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn("mb-5 flex flex-col gap-1.5", className)} {...props} />
);
export const DialogFooter = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn(
      "mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end",
      className,
    )}
    {...props}
  />
);
export const DialogTitle = ({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Title>) => (
  <DialogPrimitive.Title
    className={cn(
      "text-lg font-semibold tracking-[-0.012em] text-paper",
      className,
    )}
    {...props}
  />
);
export const DialogDescription = ({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Description>) => (
  <DialogPrimitive.Description
    className={cn("text-sm leading-6 text-fog", className)}
    {...props}
  />
);
