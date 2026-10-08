import type { Metadata } from "next";
import { Suspense } from "react";
import "@fontsource-variable/inter";
import { StandaloneAuthFallback } from "@/features/auth/auth-standalone";
import { PasswordRecoveryView } from "@/features/auth/password-recovery-views";

export const metadata: Metadata = {
  title: "Reset password",
  description: "Choose a new Stealth Console password.",
};

export default function ResetPasswordPage() {
  return (
    <Suspense
      fallback={<StandaloneAuthFallback label="Loading password reset form…" />}
    >
      <PasswordRecoveryView />
    </Suspense>
  );
}
