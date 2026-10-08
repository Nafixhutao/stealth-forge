import type { Metadata } from "next";
import { Suspense } from "react";
import "@fontsource-variable/inter";
import { StandaloneAuthFallback } from "@/features/auth/auth-standalone";
import { RegisterView } from "@/features/auth/register-view";

export const metadata: Metadata = {
  title: "Create account",
  description: "Create your Stealth Console account.",
};

export default function RegisterPage() {
  return (
    <Suspense
      fallback={<StandaloneAuthFallback label="Loading registration form…" />}
    >
      <RegisterView />
    </Suspense>
  );
}
