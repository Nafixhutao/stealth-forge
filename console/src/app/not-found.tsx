"use client";

import { ApiError } from "@/api/client";
import { useCurrentAccount } from "@/api/queries/account";
import { NotFoundGlitch } from "@/components/motion/not-found/glitch";

/**
 * Root 404, shared by every surface: the admin console, the customer console,
 * and signed-out visitors.
 *
 * The primary action targets `/`, which already resolves by role: the root
 * redirects to /admin, AdminGate forwards non-admins to /user, and anonymous
 * visitors to /login. That keeps one page correct for every role.
 *
 * "Sign in" is offered only once the account check confirms there is no
 * session. While that check is pending, or when a session exists, the single
 * primary action is the correct offer, so the page never shows a control that
 * does not apply to the visitor.
 */
export default function NotFound() {
  const account = useCurrentAccount();
  const signedOut =
    account.error instanceof ApiError && account.error.status === 401;

  return (
    <main
      data-error
      className="flex min-h-dvh flex-col justify-center px-4 py-16"
    >
      <NotFoundGlitch
        className="w-full"
        homeHref="/"
        homeLabel="Return to the Console"
        browseHref={signedOut ? "/login" : undefined}
        browseLabel={signedOut ? "Sign in" : undefined}
      />
    </main>
  );
}
