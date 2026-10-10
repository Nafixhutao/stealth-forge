"use client";

import { useEffect, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useCurrentAccount } from "@/api/queries/account";
import { ApiError } from "@/api/client";
import { ErrorState } from "@/components/feedback/error-state";
import { LoadingState } from "@/components/feedback/loading-state";
import { PixelSkeleton } from "@/components/ui/pixel-skeleton";
import { Sidebar } from "@/features/navigation/sidebar";
import { Topbar } from "@/components/layout/topbar";
import {
  ConsoleRouteContextProvider,
  useConsoleRouteContext,
} from "@/components/navigation/console-route-context";
import { ProjectRealtimeListener } from "@/realtime/project-realtime-listener";
import { AdminRealtimeListener } from "@/realtime/admin-realtime-listener";

function ConsoleShellContent({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const router = useRouter();
  const { projectId } = useConsoleRouteContext();
  const account = useCurrentAccount();
  const [mobileOpen, setMobileOpen] = useState(false);
  const mobileMenuTriggerRef = useRef<HTMLButtonElement>(null);
  const pathname = usePathname();
  const unauthorized =
    account.error instanceof ApiError && account.error.status === 401;
  // Instance owners and admins operate the platform from the Admin Console, so
  // the customer console (organizations, projects, services) is not their
  // workspace. /account stays reachable so they can manage their own
  // credentials and sign out.
  const role = account.data?.account.instance_role;
  const instanceAdmin = role === "instance_owner" || role === "instance_admin";
  const adminOnlyRedirect = instanceAdmin && pathname !== "/account";

  useEffect(() => {
    if (unauthorized) router.replace("/login");
    else if (adminOnlyRedirect) router.replace("/admin");
  }, [router, unauthorized, adminOnlyRedirect]);

  if (account.isPending)
    return (
      <div className="flex min-h-dvh items-center justify-center bg-void">
        <span role="status" aria-live="polite" className="sr-only">
          Loading account…
        </span>
      </div>
    );
  if (unauthorized)
    return (
      <div className="flex min-h-dvh items-center justify-center bg-void">
        <LoadingState
          label="Session expired. Returning to sign in…"
          className="w-72"
        >
          <PixelSkeleton className="mx-auto size-12 rounded-2xl" />
          <p className="text-center text-xs text-slate-500">
            Session expired. Returning to sign in…
          </p>
        </LoadingState>
      </div>
    );
  if (adminOnlyRedirect)
    return (
      <div className="flex min-h-dvh items-center justify-center bg-void">
        <LoadingState label="Opening the Admin Console…" className="w-72">
          <PixelSkeleton className="mx-auto size-12 rounded-2xl" />
          <p className="text-center text-xs text-slate-500">
            Instance owners and admins work from the Admin Console.
          </p>
        </LoadingState>
      </div>
    );
  if (account.error && !account.data)
    return (
      <main className="mx-auto flex min-h-dvh max-w-2xl items-center px-6">
        <ErrorState
          title="Could not load account"
          error={account.error}
          retry={() => account.refetch()}
        />
      </main>
    );

  return (
    <div data-app-shell className="min-h-dvh bg-void">
      <ProjectRealtimeListener projectId={projectId} />
      {(account.data?.account.instance_role === "instance_owner" ||
        account.data?.account.instance_role === "instance_admin") && (
        <AdminRealtimeListener />
      )}
      <a
        href="#main-content"
        className="sr-only fixed left-4 top-4 z-[60] rounded-md bg-acid-lime px-3 py-2 text-sm font-medium text-void focus:not-sr-only"
      >
        Skip to content
      </a>
      <div className="flex min-h-dvh">
        <Sidebar open={mobileOpen} onClose={() => setMobileOpen(false)} />
        <div className="min-w-0 flex-1">
          <Topbar
            onMenu={() => setMobileOpen(true)}
            menuButtonRef={mobileMenuTriggerRef}
          />
          <main
            id="main-content"
            className="mx-auto w-full max-w-[1200px] px-4 py-6 sm:px-6 sm:py-8 lg:px-8"
          >
            {children}
          </main>
        </div>
      </div>
    </div>
  );
}

export function ConsoleShell({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <ConsoleRouteContextProvider>
      <ConsoleShellContent>{children}</ConsoleShellContent>
    </ConsoleRouteContextProvider>
  );
}
