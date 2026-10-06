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

// Brand marks are tiny inline SVGs — not worth an icon-package dependency.
export function GoogleMark() {
  return (
    <svg viewBox="0 0 24 24" className="size-4" aria-hidden="true">
      <path
        fill="#4285F4"
        d="M23.49 12.27c0-.79-.07-1.54-.19-2.27H12v4.51h6.47a5.57 5.57 0 0 1-2.4 3.58v3h3.86c2.26-2.09 3.56-5.17 3.56-8.82z"
      />
      <path
        fill="#34A853"
        d="M12 24c3.24 0 5.95-1.08 7.93-2.91l-3.86-3c-1.08.72-2.45 1.16-4.07 1.16-3.13 0-5.78-2.11-6.73-4.96H1.29v3.09A11.99 11.99 0 0 0 12 24z"
      />
      <path
        fill="#FBBC05"
        d="M5.27 14.29A7.16 7.16 0 0 1 4.89 12c0-.8.14-1.57.38-2.29V6.62H1.29a11.99 11.99 0 0 0 0 10.76l3.98-3.09z"
      />
      <path
        fill="#EA4335"
        d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.42-3.42C17.95 1.19 15.24 0 12 0 7.31 0 3.26 2.7 1.29 6.62l3.98 3.09C6.22 6.86 8.87 4.75 12 4.75z"
      />
    </svg>
  );
}

export function GitHubMark() {
  return (
    <svg viewBox="0 0 24 24" className="size-4 fill-white" aria-hidden="true">
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.55 0-.27-.01-1.17-.02-2.12-3.2.7-3.87-1.36-3.87-1.36-.52-1.33-1.28-1.68-1.28-1.68-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.18 1.76 1.18 1.03 1.76 2.69 1.25 3.35.96.1-.75.4-1.25.72-1.54-2.55-.29-5.23-1.28-5.23-5.68 0-1.26.45-2.28 1.18-3.09-.12-.29-.51-1.46.11-3.05 0 0 .96-.31 3.15 1.18a10.9 10.9 0 0 1 5.74 0c2.19-1.49 3.15-1.18 3.15-1.18.62 1.59.23 2.76.11 3.05.74.81 1.18 1.83 1.18 3.09 0 4.41-2.69 5.38-5.25 5.67.41.35.77 1.05.77 2.12 0 1.53-.01 2.76-.01 3.14 0 .3.2.67.8.55A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z" />
    </svg>
  );
}

export const AUTH_PROVIDERS: Array<{
  id: "google" | "github";
  name: string;
  mark: ReactNode;
}> = [
  { id: "google", name: "Google", mark: <GoogleMark /> },
  { id: "github", name: "GitHub", mark: <GitHubMark /> },
];

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
