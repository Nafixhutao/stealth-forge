"use client";

import type { MouseEvent, ReactNode } from "react";

/**
 * Shared tokens and shell for the standalone auth screens (sign-in, forgot
 * password, reset password), ported verbatim from the Stealth marketing console
 * design so every screen stays pixel-identical.
 *
 * The trailing `!` on border/type utilities beats the Console's unlayered
 * global resets (`* { border-color }`, `button { font: inherit }`); the spacing,
 * radii, and font weights are restored by [data-auth-standalone] in
 * styles/auth-standalone.css.
 */
export const authSecondaryButton =
  "flex h-10 items-center justify-center gap-2 rounded-lg border border-[#333338]! bg-[#0f0f0f] text-[13.5px]! font-medium! text-[#f2f2f3] transition-colors hover:border-[#4a4a50]! hover:bg-[#1c1c1c] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#186cee]!";
export const authInputClass =
  "h-10 w-full rounded-lg border bg-[#171717] px-3.5 text-[14px]! text-white outline-none transition-colors placeholder:text-[#6b6b72]! focus-visible:outline-none!";
export const authInputBorder = "border-[#2a2e38]! focus:border-[#186cee]!";
export const authInputBorderError = "border-[#e5484d]! focus:border-[#e5484d]!";
export const authErrorText =
  "mt-1.5 flex items-center gap-1.5 text-[12.5px] leading-4 text-[#f87171]";
export const authLinkClass =
  "font-medium! text-[#4d8dff] underline underline-offset-2 hover:text-[#7fabff]";
export const authSubmitButton =
  "mt-6 h-10 w-full rounded-lg bg-[linear-gradient(180deg,#2e83f7_0%,#186cee_48%,#0e5cd6_100%)] text-[14px]! font-semibold! text-white transition-[filter,opacity] hover:brightness-110 disabled:cursor-default disabled:opacity-70 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#186cee]!";

export const preventPlaceholderNav = (event: MouseEvent<HTMLAnchorElement>) =>
  event.preventDefault();

/** Full-page shell shared by the standalone auth screens. */
export function StandaloneAuthShell({ children }: { children: ReactNode }) {
  return (
    <main
      data-auth-standalone
      className="relative flex min-h-dvh flex-col items-center bg-[#0f0f0f] px-4 py-14 font-medium text-white sm:py-10"
    >
      <a
        href="#auth-standalone-content"
        className="sr-only fixed left-4 top-4 z-[60] rounded-md bg-acid-lime px-3 py-2 text-sm font-medium text-void focus:not-sr-only"
      >
        Skip to content
      </a>
      <div
        id="auth-standalone-content"
        className="my-auto w-full max-w-[364px]"
      >
        {children}
      </div>
    </main>
  );
}
