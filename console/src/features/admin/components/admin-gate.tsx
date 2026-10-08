"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { ApiError } from "@/api/client";
import { useCurrentAccount } from "@/api/queries/account";
import { ErrorState } from "@/components/feedback/error-state";

/**
 * AdminGate guards the standalone /admin area. The redesigned admin chrome was
 * authored as a self-contained prototype without an auth boundary; this wraps
 * it with the Console session check so the admin surface requires an
 * authenticated instance owner or instance admin, matching the API.
 */
export function AdminGate({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const account = useCurrentAccount();
  const unauthorized =
    account.error instanceof ApiError && account.error.status === 401;
  const role = account.data?.account.instance_role;
  const notAdmin =
    Boolean(account.data) &&
    role !== "instance_owner" &&
    role !== "instance_admin";

  useEffect(() => {
    if (unauthorized) router.replace("/login");
    else if (notAdmin) router.replace("/organizations");
  }, [unauthorized, notAdmin, router]);

  // The access check is a short, silent wait: rendering a placeholder here
  // only flashes grey boxes before the admin chrome paints.
  if (account.isPending || unauthorized || notAdmin) {
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <span role="status" aria-live="polite" className="sr-only">
          Checking admin access…
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
