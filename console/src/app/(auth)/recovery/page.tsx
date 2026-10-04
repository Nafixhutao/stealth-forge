import type { Metadata } from "next";
import { Suspense } from "react";
import { PasswordRecoveryView } from "@/features/auth/auth-flow-views";
import { LoadingState } from "@/components/feedback/loading-state";
import { Skeleton } from "@/components/ui/skeleton";

export const metadata: Metadata = {
  title: "Recover account",
  description: "Request a Stealth Console password reset.",
};

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
