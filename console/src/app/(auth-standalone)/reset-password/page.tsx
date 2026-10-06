import type { Metadata } from "next";
import { Suspense } from "react";
import "@fontsource-variable/inter";
import { PasswordRecoveryView } from "@/features/auth/password-recovery-views";
import { LoadingState } from "@/components/feedback/loading-state";
import { Skeleton } from "@/components/ui/skeleton";

export const metadata: Metadata = {
  title: "Reset password",
  description: "Choose a new Stealth Console password.",
};

export default function ResetPasswordPage() {
  return (
    <Suspense
      fallback={
        <LoadingState
          label="Loading password reset form…"
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
