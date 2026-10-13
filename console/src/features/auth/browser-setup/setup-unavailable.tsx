"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

/**
 * The setup API answers 404 once the instance is no longer in setup mode, so
 * this route has nothing to configure. Rather than render a wizard shell that
 * claims the route is "sealed" while it is still served, send the visitor to
 * sign in. The route is also blocked at the edge in production; this covers a
 * stale ingress config and keeps the copy honest.
 */
export function SetupUnavailable() {
  const router = useRouter();

  useEffect(() => {
    router.replace("/login");
  }, [router]);

  return (
    <div
      data-setup
      className="flex min-h-dvh items-center justify-center bg-[var(--setup-bg)] px-4"
    >
      <span role="status" aria-live="polite" className="sr-only">
        Setup is not available on this instance. Opening sign in.
      </span>
    </div>
  );
}
