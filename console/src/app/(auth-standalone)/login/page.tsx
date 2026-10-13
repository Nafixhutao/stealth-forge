import type { Metadata } from "next";
import { Suspense } from "react";
import "@fontsource-variable/inter";
import { StandaloneAuthFallback } from "@/features/auth/auth-standalone";
import { redirectIfAuthenticated } from "@/features/auth/authenticated-redirect.server";
import { LoginView } from "@/features/auth/login-view";

export const metadata: Metadata = {
  title: "Sign in",
  description: "Sign in to the Stealth Console.",
};

// Reads the session cookie, so this page is per-request rather than static.
export const dynamic = "force-dynamic";

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ next?: string }>;
}) {
  const { next } = await searchParams;
  await redirectIfAuthenticated(next ?? null);

  return (
    <Suspense
      fallback={<StandaloneAuthFallback label="Loading sign-in form…" />}
    >
      <LoginView />
    </Suspense>
  );
}
