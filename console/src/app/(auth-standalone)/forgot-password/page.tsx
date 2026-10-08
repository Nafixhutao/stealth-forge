import type { Metadata } from "next";
import { Suspense } from "react";
import "@fontsource-variable/inter";
import { StandaloneAuthFallback } from "@/features/auth/auth-standalone";
import { PasswordRecoveryView } from "@/features/auth/password-recovery-views";

export const metadata: Metadata = {
  title: "Forgot password",
  description: "Request a Stealth Console password reset.",
};

export default function ForgotPasswordPage() {
  return (
    <Suspense
      fallback={<StandaloneAuthFallback label="Loading recovery form…" />}
    >
      <PasswordRecoveryView />
    </Suspense>
  );
}
