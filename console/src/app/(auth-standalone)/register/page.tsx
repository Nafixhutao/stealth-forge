import type { Metadata } from "next";
import { Suspense } from "react";
import "@fontsource-variable/inter";
import { StandaloneAuthFallback } from "@/features/auth/auth-standalone";
import { redirectIfAuthenticated } from "@/features/auth/authenticated-redirect.server";
import { RegisterView } from "@/features/auth/register-view";

export const metadata: Metadata = {
  title: "Create account",
  description: "Create your Stealth Console account.",
};

// Reads the session cookie, so this page is per-request rather than static.
export const dynamic = "force-dynamic";

export default async function RegisterPage({
  searchParams,
}: {
  searchParams: Promise<{ next?: string }>;
}) {
  const { next } = await searchParams;
  await redirectIfAuthenticated(next ?? null);

  return (
    <Suspense
      fallback={<StandaloneAuthFallback label="Loading registration form…" />}
    >
      <RegisterView />
    </Suspense>
  );
}
