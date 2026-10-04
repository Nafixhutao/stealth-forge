import type { Metadata } from "next";
import { Suspense } from "react";
import { RegisterView } from "@/features/auth/register-view";
import { LoadingState } from "@/components/feedback/loading-state";
import { Skeleton } from "@/components/ui/skeleton";

export const metadata: Metadata = {
  title: "Create account",
  description: "Create your Stealth Console account.",
};

export default function RegisterPage() {
  return (
    <Suspense
      fallback={
        <LoadingState
          label="Loading registration form…"
          className="h-[28rem] w-full max-w-md"
        >
          <Skeleton className="h-full w-full rounded-2xl" />
        </LoadingState>
      }
    >
      <RegisterView />
    </Suspense>
  );
}
