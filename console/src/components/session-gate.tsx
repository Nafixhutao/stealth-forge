"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { ApiError } from "@/api/client";
import { useCurrentAccount } from "@/api/queries/account";
import { ErrorState } from "@/components/feedback/error-state";

/**
 * Resolves the current Console session and forwards anonymous visitors to
 * sign-in. Shared by the admin and user surfaces so each only layers its own
 * role rule on top of the same authentication boundary.
 */
export function useSessionGuard() {
  const router = useRouter();
  const account = useCurrentAccount();
  const unauthorized =
    account.error instanceof ApiError && account.error.status === 401;

  useEffect(() => {
    if (unauthorized) router.replace("/login");
  }, [unauthorized, router]);

  return {
    account,
    unauthorized,
    /** True while the access check is unresolved or already redirecting. */
    checking: account.isPending || unauthorized,
  };
}

/**
 * Requires an authenticated Console session. The customer console needs no
 * role rule beyond this: any signed-in account may use it.
 */
export function SessionGate({ children }: { children: React.ReactNode }) {
  const { account, checking } = useSessionGuard();

  // The access check is a short, silent wait: a full placeholder here would
  // only flash grey boxes before the console chrome paints.
  if (checking) {
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <span role="status" aria-live="polite" className="sr-only">
          Checking your session…
        </span>
      </div>
    );
  }

  if (account.error && !account.data) {
    return (
      <div className="mx-auto flex min-h-dvh max-w-2xl items-center px-6">
        <ErrorState
          title="Could not load account"
          error={account.error}
          retry={() => account.refetch()}
        />
      </div>
    );
  }

  return children;
}
