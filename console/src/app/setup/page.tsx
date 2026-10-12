import type { Metadata } from "next";
import { Suspense } from "react";
import { BrowserSetupView } from "@/features/auth/browser-setup-view";
import { LoadingState } from "@/components/feedback/loading-state";
import { PixelSkeleton } from "@/components/ui/pixel-skeleton";

export const metadata: Metadata = {
  title: "Browser setup",
  description: "Continue the Stealth browser installation.",
};

// The setup wizard owns the whole viewport, so it deliberately lives outside the
// (auth) split-screen group and renders its own full-screen shell.
export default function SetupPage() {
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
