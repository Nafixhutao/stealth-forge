import type { Metadata } from "next";
import { Suspense } from "react";
import { AccountVerificationView } from "@/features/auth/auth-flow-views";
import { LoadingState } from "@/components/feedback/loading-state";
import { PixelSkeleton } from "@/components/ui/pixel-skeleton";

export const metadata: Metadata = {
  title: "Verify email",
  description: "Verify your email for the Stealth Console.",
};

export default function VerifyEmailPage() {
  return (
    <Suspense
      fallback={
        <LoadingState
          label="Loading verification…"
          className="h-[20rem] w-full max-w-md"
        >
          <PixelSkeleton className="h-full w-full rounded-2xl" />
        </LoadingState>
      }
    >
      <AccountVerificationView />
    </Suspense>
  );
}
