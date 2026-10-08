"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { CircleAlert, CircleCheck } from "lucide-react";
import { useConfirmAccountRecovery, useRecoveryRequest } from "@/api/mutations";
import { errorMessage } from "@/components/feedback/error-state";
import {
  authErrorText as errorText,
  authInputBorder as inputBorder,
  authInputBorderError as inputBorderError,
  authInputClass as inputClass,
  authLinkClass as linkClass,
  authSubmitButton as submitButton,
  StandaloneAuthShell,
} from "./auth-standalone";

const emailSchema = z.object({
  email: z.string().email("Enter a valid email address."),
});
const passwordSchema = z.object({
  password: z.string().min(12, "Use at least 12 characters."),
});

function tokenPayload(token: string, secret: string | null) {
  return secret ? { secret } : { token };
}

/**
 * Standalone password recovery, matching the marketing console design. Without
 * a token it requests a reset link; with a `token`/`secret` query value it sets
 * a new password. Both states share the sign-in screen's shell and tokens.
 */
export function PasswordRecoveryView() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const request = useRecoveryRequest();
  const confirm = useConfirmAccountRecovery();
  const token = searchParams.get("token");
  const secret = searchParams.get("secret");
  const hasToken = Boolean(token || secret);
  const [sent, setSent] = useState(false);
  const [sentEmail, setSentEmail] = useState("");

  const emailForm = useForm<z.infer<typeof emailSchema>>({
    resolver: zodResolver(emailSchema),
    defaultValues: { email: "" },
  });
  const passwordForm = useForm<z.infer<typeof passwordSchema>>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { password: "" },
  });

  const requestRecovery = emailForm.handleSubmit(async (values) => {
    try {
      await request.mutateAsync({ email: values.email });
      setSentEmail(values.email.trim());
      setSent(true);
    } catch {
      // The mutation error is rendered below; handleSubmit re-throws.
    }
  });
  const resetPassword = passwordForm.handleSubmit(async (values) => {
    try {
      await confirm.mutateAsync({
        ...tokenPayload(token ?? secret ?? "", secret),
        password: values.password,
      });
      router.replace("/login?reset=success");
    } catch {
      // The mutation error is rendered below; handleSubmit re-throws.
    }
  });

  if (hasToken) {
    const passwordError = passwordForm.formState.errors.password;
    return (
      <StandaloneAuthShell>
        <h1 className="m-0 text-center text-2xl font-bold leading-8 tracking-[-0.01em]">
          Choose a new password
        </h1>
        <p className="mt-2 text-center text-[13.5px] leading-5 text-[#b3b3ba]">
          Enter a new password for your account.
        </p>

        <form onSubmit={resetPassword} noValidate className="mt-6">
          <label
            htmlFor="reset-password"
            className="mb-1.5 block text-[13.5px] font-medium leading-4"
          >
            New password
          </label>
          <input
            id="reset-password"
            type="password"
            autoComplete="new-password"
            {...passwordForm.register("password")}
            aria-invalid={passwordError ? true : undefined}
            aria-describedby={
              passwordError ? "reset-password-error" : "reset-password-hint"
            }
            className={`${inputClass} ${passwordError ? inputBorderError : inputBorder}`}
          />
          <p
            id="reset-password-hint"
            className="mt-1.5 text-[12.5px] leading-4 text-[#8b8b92]"
          >
            Minimum 12 characters.
          </p>
          {passwordError ? (
            <p id="reset-password-error" className={errorText}>
              <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
              {passwordError.message}
            </p>
          ) : null}

          {confirm.error ? (
            <p role="alert" className={errorText}>
              <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
              {errorMessage(confirm.error)}
            </p>
          ) : null}

          <button
            type="submit"
            disabled={confirm.isPending}
            aria-busy={confirm.isPending}
            className={submitButton}
          >
            {confirm.isPending ? "Updating password…" : "Set new password"}
          </button>
        </form>

        <p className="mt-5 text-center text-[13.5px] leading-5 text-[#b3b3ba]">
          <Link href="/login" className={linkClass}>
            Back to sign in
          </Link>
        </p>
      </StandaloneAuthShell>
    );
  }

  if (sent) {
    return (
      <StandaloneAuthShell>
        <div className="flex flex-col items-center text-center">
          <span className="flex size-12 items-center justify-center rounded-full bg-[#12351f]">
            <CircleCheck
              size={26}
              strokeWidth={1.8}
              className="text-[#4ade80]"
              aria-hidden="true"
            />
          </span>
          <h1 className="m-0 mt-4 text-2xl font-bold leading-8 tracking-[-0.01em]">
            Check your email
          </h1>
          <p className="mt-2 text-[13.5px] leading-5 text-[#b3b3ba]">
            If an account exists for{" "}
            <span className="font-semibold text-white">{sentEmail}</span>,
            we&apos;ve sent a link to reset your password.
          </p>
          <button
            type="button"
            onClick={() => router.push("/login")}
            className={submitButton}
          >
            Back to sign in
          </button>
          <p className="mt-5 text-[13.5px] leading-5 text-[#b3b3ba]">
            Didn&apos;t receive it? Check your spam folder or{" "}
            <button
              type="button"
              onClick={() => {
                setSent(false);
                request.reset();
              }}
              className={linkClass}
            >
              try another email
            </button>
          </p>
        </div>
      </StandaloneAuthShell>
    );
  }

  const emailError = emailForm.formState.errors.email;
  return (
    <StandaloneAuthShell>
      <h1 className="m-0 text-center text-2xl font-bold leading-8 tracking-[-0.01em]">
        Forgot password?
      </h1>
      <p className="mt-2 text-center text-[13.5px] leading-5 text-[#b3b3ba]">
        Enter the email associated with your account and we&apos;ll send
        instructions to reset your password.
      </p>

      <form onSubmit={requestRecovery} noValidate className="mt-6">
        <label
          htmlFor="forgot-email"
          className="mb-1.5 block text-[13.5px] font-medium leading-4"
        >
          Email
        </label>
        <input
          id="forgot-email"
          type="email"
          autoComplete="email"
          spellCheck={false}
          {...emailForm.register("email")}
          aria-invalid={emailError ? true : undefined}
          aria-describedby={emailError ? "forgot-email-error" : undefined}
          className={`${inputClass} ${emailError ? inputBorderError : inputBorder}`}
        />
        {emailError ? (
          <p id="forgot-email-error" className={errorText}>
            <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
            {emailError.message}
          </p>
        ) : null}

        {request.error ? (
          <p role="alert" className={errorText}>
            <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
            {errorMessage(request.error)}
          </p>
        ) : null}

        <button
          type="submit"
          disabled={request.isPending}
          aria-busy={request.isPending}
          className={submitButton}
        >
          {request.isPending ? "Sending…" : "Send reset link"}
        </button>
      </form>

      <p className="mt-5 text-center text-[13.5px] leading-5 text-[#b3b3ba]">
        Remembered it?{" "}
        <Link href="/login" className={linkClass}>
          Back to sign in
        </Link>
      </p>
    </StandaloneAuthShell>
  );
}
