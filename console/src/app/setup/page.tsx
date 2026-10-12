import type { Metadata } from "next";
import { Suspense } from "react";
import { notFound } from "next/navigation";
import { BrowserSetupView } from "@/features/auth/browser-setup-view";
import { LoadingState } from "@/components/feedback/loading-state";
import { PixelSkeleton } from "@/components/ui/pixel-skeleton";

export const metadata: Metadata = {
  title: "Browser setup",
  description: "Continue the Stealth browser installation.",
};

// The probe below runs per request, so this route must never be prerendered.
export const dynamic = "force-dynamic";

// The setup API is reachable on the internal network under the same alias in
// both the production and setup Compose projects, so one default covers both.
const INTERNAL_API_URL = (
  process.env.STEALTH_API_INTERNAL_URL ?? "http://stealth-api:8080"
).replace(/\/+$/, "");

const PROBE_TIMEOUT_MS = 2_000;

/**
 * Asks the API whether this deployment still offers first-run setup. The setup
 * API answers 404 once setup is unavailable, which is the signal that this
 * route does not exist on this host.
 *
 * Fails open: a probe error or timeout must never hide the wizard, because the
 * setup origin is the one place the wizard has to work.
 */
async function setupIsOffered(): Promise<boolean> {
  try {
    const response = await fetch(`${INTERNAL_API_URL}/v1/setup/status`, {
      cache: "no-store",
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
    });
    return response.status !== 404;
  } catch {
    return true;
  }
}

/**
 * The setup wizard owns the whole viewport, so it deliberately lives outside the
 * (auth) split-screen group and renders its own full-screen shell.
 *
 * The route only exists where setup is actually offered. The host CLI publishes
 * the wizard through a Cloudflare quick tunnel that terminates at setup-proxy,
 * so on any production host this path is not a route at all and answers 404
 * rather than rendering a wizard the API would refuse.
 */
export default async function SetupPage() {
  if (!(await setupIsOffered())) {
    notFound();
  }

  return (
    <Suspense
      fallback={
        <div
          data-setup
          className="flex min-h-dvh items-center justify-center bg-[var(--setup-bg)] px-4 py-8"
        >
          <LoadingState
            label="Loading browser setup…"
            className="w-full max-w-3xl"
          >
            <PixelSkeleton className="h-[28rem] w-full rounded-xl" />
          </LoadingState>
        </div>
      }
    >
      <BrowserSetupView />
    </Suspense>
  );
}
