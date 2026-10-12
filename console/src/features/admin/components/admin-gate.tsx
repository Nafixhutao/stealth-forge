"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { ErrorState } from "@/components/feedback/error-state";
import { useSessionGuard } from "@/components/session-gate";

/**
 * AdminGate guards the standalone /admin area. The redesigned admin chrome was
 * authored as a self-contained prototype without an auth boundary; this wraps
 * it with the Console session check so the admin surface requires an
 * authenticated instance owner or instance admin, matching the API.
 */
export function AdminGate({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const { account, checking } = useSessionGuard();
  const role = account.data?.account.instance_role;
  const notAdmin =
    Boolean(account.data) &&
    role !== "instance_owner" &&
    role !== "instance_admin";

  useEffect(() => {
    if (notAdmin) router.replace("/user");
  }, [notAdmin, router]);

  // The access check is a short, silent wait: rendering a placeholder here
  // only flashes grey boxes before the admin chrome paints.
  if (checking || notAdmin) {
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <span role="status" aria-live="polite" className="sr-only">
          {notAdmin ? "Opening the user console…" : "Checking admin access…"}
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
