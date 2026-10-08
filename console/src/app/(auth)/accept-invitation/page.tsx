import type { Metadata } from "next";
import { Suspense } from "react";
import { InvitationAcceptanceView } from "@/features/auth/invitation-acceptance-view";
import { LoadingState } from "@/components/feedback/loading-state";
import { PixelSkeleton } from "@/components/ui/pixel-skeleton";

export const metadata: Metadata = {
  title: "Accept invitation",
  description: "Accept your Stealth Console invitation.",
};

export default function AcceptInvitationPage() {
  return (
    <Suspense
      fallback={
        <LoadingState
          label="Loading invitation…"
          className="h-[24rem] w-full max-w-md"
        >
          <PixelSkeleton className="h-full w-full rounded-2xl" />
        </LoadingState>
      }
    >
      <InvitationAcceptanceView />
    </Suspense>
  );
}
