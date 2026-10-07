import type { Metadata } from "next";
import { Suspense } from "react";
import "@fontsource-variable/inter";
import { StandaloneAuthFallback } from "@/features/auth/auth-standalone";
import { PasswordRecoveryView } from "@/features/auth/password-recovery-views";

export const metadata: Metadata = {
  title: "Recover account",
  description: "Request a Stealth Console password reset.",
};

// Kept as a stable entry point: the recovery email and older links use
// /recovery, while the redesigned sign-in screen points at /forgot-password.
export default function RecoveryPage() {
  return (
    <Suspense
      fallback={<StandaloneAuthFallback label="Loading recovery form…" />}
    >
      <PasswordRecoveryView />
    </Suspense>
  );
}
