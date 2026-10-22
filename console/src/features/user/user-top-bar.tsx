"use client";

import { Menu, Search } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useLogout } from "@/api/mutations/auth";
import { useCurrentAccount } from "@/api/queries/account";
import { useApplicationShell } from "@/components/application-shell";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

/**
 * The user console's mobile top bar: the brand mark on the left, then the
 * command palette, the account, and the navigation toggle on the right. It
 * renders below `lg` only — from `lg` up the desktop rail already carries the
 * chrome — and exists so the phone layout reads like the reference instead of a
 * floating hamburger.
 *
 * The mark is the way home, not a menu: it links to the console's front page,
 * which is what a brand mark does everywhere else. Navigation has its own
 * control — the three-line button beside the account, at the edge a thumb
 * reaches — and that opens the bottom sheet.
 */

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--projects-bg)]";

export function UserTopBar({ canOpenAdmin }: { canOpenAdmin: boolean }) {
  const shell = useApplicationShell();
  const account = useCurrentAccount();
  const logout = useLogout();
  const router = useRouter();

  const identity = account.data?.account;
  const name =
    identity?.display_name?.trim() ||
    identity?.email?.split("@")[0] ||
    "Account";
  const email = identity?.email ?? "";
  const avatar = identity?.avatar_url ?? "";

  const signOut = () => {
    logout.mutate(undefined, {
      onSuccess: () => {
        toast.success("Signed out");
        router.replace("/login");
      },
    });
  };

  return (
    <header className="fixed inset-x-0 top-0 z-40 flex !h-[59px] box-border items-center gap-1 border-b border-[var(--projects-divider)] bg-[var(--projects-bg)] px-4 lg:hidden">
      <Link
        href="/user"
        aria-label="Stealth home"
        title="Stealth home"
        className={cn(
          "inline-flex shrink-0 items-center justify-center rounded-lg transition-colors hover:bg-[var(--projects-control)]",
          FOCUS,
        )}
      >
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src="/stealth-cat.png"
          alt=""
          className="size-6 shrink-0 object-contain"
        />
      </Link>

      <div className="ml-auto flex items-center gap-1.5">
        <button
          type="button"
          onClick={() =>
            window.dispatchEvent(new Event("open-command-palette"))
          }
          aria-label="Search"
          title="Search"
          className={cn(
            "inline-flex size-9 items-center justify-center rounded-lg text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]",
            FOCUS,
          )}
        >
          <Search className="size-4" strokeWidth={1.8} aria-hidden="true" />
        </button>

        <DropdownMenu>
          <DropdownMenuTrigger
            aria-label="Account menu"
            title="Account menu"
            className={cn(
              "inline-flex size-9 items-center justify-center rounded-full transition-colors hover:bg-[var(--projects-control)]",
              FOCUS,
            )}
          >
            {avatar ? (
              /* eslint-disable-next-line @next/next/no-img-element */
              <img
                src={avatar}
                alt=""
                className="!size-6 rounded-full object-cover"
              />
            ) : (
              <span className="flex !size-6 items-center justify-center rounded-full bg-[var(--user-accent-strong)] text-xs font-semibold text-white">
                {name.slice(0, 1).toUpperCase()}
              </span>
            )}
          </DropdownMenuTrigger>
          {/* Portaled, so it opts into the user console's palette itself. */}
          <DropdownMenuContent
            align="end"
            data-user-surface=""
            className="min-w-[220px]"
          >
            <div className="px-2.5 py-2">
              <p className="m-0 truncate text-[13px] font-medium text-[var(--projects-text)]">
                {name}
              </p>
              {email ? (
                <p className="m-0 mt-0.5 truncate text-[12px] text-[var(--projects-muted)]">
                  {email}
                </p>
              ) : null}
            </div>
            <DropdownMenuSeparator />
            {canOpenAdmin ? (
              <DropdownMenuItem onSelect={() => router.push("/admin")}>
                Open admin console
              </DropdownMenuItem>
            ) : null}
            <DropdownMenuItem onSelect={signOut}>Log out</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <button
          type="button"
          // Opens on click, not pointer-down. Opening on pointer-down starts the
          // motion a few milliseconds sooner, but it puts the backdrop under the
          // finger before the tap finishes: the browser then dispatches the
          // tap's own click to the backdrop and the sheet closes in the same
          // gesture. On a phone that reads as "the button does nothing".
          onClick={() => shell?.openSidebar()}
          aria-label="Open navigation"
          aria-haspopup="dialog"
          title="Open navigation"
          className={cn(
            "-mr-1.5 inline-flex size-9 items-center justify-center rounded-lg text-[var(--projects-text)] transition-colors hover:bg-[var(--projects-control)]",
            FOCUS,
          )}
        >
          <Menu className="size-[18px]" strokeWidth={1.9} aria-hidden="true" />
        </button>
      </div>
    </header>
  );
}
