"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { ApiError } from "@/api/client";
import { useCurrentAccount } from "@/api/queries/account";
import { ErrorState } from "@/components/feedback/error-state";
import { LoadingState } from "@/components/feedback/loading-state";

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

  if (account.isPending || unauthorized || notAdmin) {
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <LoadingState label="Checking admin access…" className="w-72" />
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
