"use client";

import { useEffect, useRef } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { CheckCircle2, Loader2, XCircle } from "lucide-react";
import { useConfirmAccountVerification } from "@/api/mutations";
import { errorMessage } from "@/components/feedback/error-state";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export function AuthCard({
  children,
  title,
  description,
}: {
  children: React.ReactNode;
  title: string;
  description: string;
}) {
  return (
    <Card className="w-full max-w-md bg-carbon">
      <CardHeader className="p-7 pb-4">
        <p className="mb-2 text-xs font-medium uppercase tracking-[0.12em] text-fog">
          Account security
        </p>
        <CardTitle className="text-2xl">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="p-7 pt-2">{children}</CardContent>
    </Card>
  );
}

function tokenPayload(token: string, secret: string | null) {
  return secret ? { secret } : { token };
}

export function AccountVerificationView() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const mutation = useConfirmAccountVerification();
  const confirmVerification = mutation.mutateAsync;
  const attempted = useRef(false);
  const token = searchParams.get("token");
  const secret = searchParams.get("secret");
  const status = searchParams.get("status");

  useEffect(() => {
    if ((!token && !secret) || attempted.current) return;
    attempted.current = true;
    void confirmVerification(tokenPayload(token ?? secret ?? "", secret))
      .then(() => {
        router.replace("/verify?status=success");
      })
      .catch(() => undefined);
  }, [confirmVerification, router, secret, token]);

  if (status === "success") {
    return (
      <AuthCard
        title="Email verified"
        description="Your Console account is ready to use."
      >
        <CheckCircle2 className="size-6 text-emerald-300" />
        <p className="mt-4 text-sm leading-6 text-slate-400">
          Email verification is complete. You can return to the project console.
        </p>
        <Button asChild className="mt-6 w-full">
          <Link href="/organizations">Open console</Link>
        </Button>
      </AuthCard>
    );
  }

  if (!token && !secret) {
    return (
      <AuthCard
        title="Verification link required"
        description="Open the one-time link from your Stealth email to verify this account."
      >
        <XCircle className="size-6 text-amber-300" />
        <p className="mt-4 text-sm leading-6 text-slate-400">
          The link does not contain a usable token. Request another verification
          email from your account page.
        </p>
        <Button asChild variant="outline" className="mt-6 w-full">
          <Link href="/login">Return to sign in</Link>
        </Button>
      </AuthCard>
    );
  }

  return (
    <AuthCard
      title={mutation.error ? "Verification failed" : "Verifying your email"}
      description={
        mutation.error
          ? errorMessage(mutation.error)
          : "Stealth is checking the one-time token with the Go API."
      }
    >
      {mutation.isPending ? (
        <Loader2 className="size-6 animate-spin text-cyan-300" />
      ) : mutation.error ? (
        <>
          <XCircle className="size-6 text-rose-300" />
          <Button
            className="mt-6 w-full"
            onClick={() => {
              mutation.reset();
              attempted.current = false;
            }}
          >
            Try again
          </Button>
        </>
      ) : (
        <Loader2 className="size-6 animate-spin text-cyan-300" />
      )}
    </AuthCard>
  );
}
