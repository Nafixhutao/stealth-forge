"use client";

import Link from "next/link";
import { cn } from "@/lib/utils";

/**
 * The user console's closing footer: the brand mark, the operator-only console
 * link, and the public status page. Shared by the organization home and the
 * project surface so both pages end the same way.
 *
 * The footer is visible to every signed-in member, so the Status link opens the
 * public status site rather than the admin console's operator view.
 */

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--projects-bg)]";

export function UserFooter({
  canOpenAdmin,
  role,
}: {
  canOpenAdmin: boolean;
  role: string | null;
}) {
  return (
    <footer className="border-t border-[var(--projects-divider)]">
      <div className="mx-auto flex h-12 w-full max-w-[1440px] items-center gap-6 px-4 sm:h-14 sm:px-6 lg:px-7">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src="/stealth-cat.png"
          alt=""
          className="size-5 shrink-0 object-contain opacity-80"
        />
        {canOpenAdmin ? (
          <Link
            href="/admin"
            aria-label={`${
              role === "instance_owner" ? "Instance owner" : "Instance admin"
            } — open the admin console`}
            className={cn(
              "rounded-md text-[13px] leading-5 text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)]",
              FOCUS,
            )}
          >
            Admin
          </Link>
        ) : null}
        {/* A plain anchor, not a `Link`: the status page is a static site the
            proxy serves beside the console, so there is no Next route to
            prefetch and prefetching one would 404 on every user page. */}
        <a
          href="/status"
          className={cn(
            "rounded-md text-[13px] leading-5 text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)]",
            FOCUS,
          )}
        >
          Status
        </a>
        <p className="m-0 ml-auto text-[13px] leading-5 text-[var(--projects-muted)]">
          © {new Date().getFullYear()} Stealth
        </p>
      </div>
    </footer>
  );
}
