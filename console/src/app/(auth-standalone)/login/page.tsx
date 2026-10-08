import type { Metadata } from "next";
import { Suspense } from "react";
import "@fontsource-variable/inter";
import { StandaloneAuthFallback } from "@/features/auth/auth-standalone";
import { LoginView } from "@/features/auth/login-view";

export const metadata: Metadata = {
  title: "Sign in",
  description: "Sign in to the Stealth Console.",
};

export default function LoginPage() {
  return (
    <Suspense
      fallback={<StandaloneAuthFallback label="Loading sign-in form…" />}
    >
      <LoginView />
    </Suspense>
  );
}
