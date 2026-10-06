import type { Metadata } from "next";
import { Suspense } from "react";
import "@fontsource-variable/inter";
import { PasswordRecoveryView } from "@/features/auth/password-recovery-views";
import { LoadingState } from "@/components/feedback/loading-state";
import { Skeleton } from "@/components/ui/skeleton";

export const metadata: Metadata = {
  title: "Recover account",
  description: "Request a Stealth Console password reset.",
};

// Kept as a stable entry point: the recovery email and older links use
// /recovery, while the redesigned sign-in screen points at /forgot-password.
export default function RecoveryPage() {
  return (
    <Suspense
      fallback={
        <LoadingState
          label="Loading recovery form…"
          className="h-[28rem] w-full max-w-md"
        >
          <Skeleton className="h-full w-full rounded-2xl" />
        </LoadingState>
      }
    >
      <PasswordRecoveryView />
    </Suspense>
  );
}
