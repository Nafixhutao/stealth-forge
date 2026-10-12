"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useCurrentAccount } from "@/api/queries/account";

/**
 * Where a signed-in account belongs. Instance owners and admins operate the
 * Admin Console; every other account uses the customer console.
 */
export function consoleHomeForRole(role: string | null | undefined) {
  return role === "instance_owner" || role === "instance_admin"
    ? "/admin"
    : "/user";
}

/**
 * Sends an already-authenticated visitor away from a signed-out-only page
 * (/login, /register).
 *
 * Without this, a valid session that reaches these pages simply sees the form
 * again, with no way back to the console, which reads as having been signed
 * out. An explicit `next` wins so deep links and invitations keep working.
 *
 * Fails open: if the account check errors, the form renders as normal.
 */
export function useAuthenticatedRedirect(next: string | null) {
  const router = useRouter();
  const account = useCurrentAccount();
  const authenticated = Boolean(account.data);

  useEffect(() => {
    if (!authenticated) return;
    router.replace(
      next ?? consoleHomeForRole(account.data?.account.instance_role),
    );
  }, [account.data, authenticated, next, router]);

  return { authenticated, checking: account.isPending };
}
