"use client";

import { useState } from "react";
import Link from "next/link";
import Image from "next/image";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { CircleAlert, Eye, EyeOff, Lock } from "lucide-react";
import { useRegister } from "@/api/mutations";
import { errorMessage } from "@/components/feedback/error-state";
import { getSafeNextPath } from "@/lib/navigation";
import {
  AUTH_PROVIDERS,
  authErrorText as errorText,
  authInputBorder as inputBorder,
  authInputBorderError as inputBorderError,
  authInputClass as inputClass,
  authLinkClass as linkClass,
  authSecondaryButton as secondaryButton,
  authSubmitButton as submitButton,
  StandaloneAuthShell,
} from "./auth-standalone";

const schema = z.object({
  email: z.string().email("Enter a valid email address."),
  password: z.string().min(12, "Use at least 12 characters."),
});
type FormValues = z.infer<typeof schema>;

/**
 * Standalone sign-up screen. The source design has no registration page, so
 * this mirrors the sign-in screen's shell and tokens for a consistent auth
 * surface. The form calls the real registration API; the provider buttons are
 * visual only until an OAuth/SSO adapter exists.
 */
export function RegisterView() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const mutation = useRegister();
  const rawNext = searchParams.get("next");
  const destination = getSafeNextPath(rawNext);
  const signInHref =
    rawNext === null
      ? "/login"
      : "/login?next=" + encodeURIComponent(destination);
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "" },
  });
  const [showPassword, setShowPassword] = useState(false);
  const [providerNotice, setProviderNotice] = useState<string | null>(null);

  const submit = form.handleSubmit(async (values) => {
    try {
      await mutation.mutateAsync(values);
      router.replace(destination);
    } catch {
      // The mutation error is rendered below; handleSubmit re-throws, so
      // swallowing here avoids an unhandled promise rejection.
    }
  });

  const showProviderNotice = (provider: string) =>
    setProviderNotice(
      `Sign-up with ${provider} is not configured on this instance yet. Use your email and password.`,
    );

  const emailError = form.formState.errors.email;
  const passwordError = form.formState.errors.password;

  return (
    <StandaloneAuthShell>
      <div className="mb-6 flex justify-center">
        <Image
          alt="Stealth"
          src="/stealth-cat.png"
          width={56}
          height={56}
          priority
          unoptimized
          style={{ color: "inherit" }}
          className="size-14"
        />
      </div>
      <h1 className="m-0 text-center text-2xl font-bold leading-8 tracking-[-0.01em]">
        Create your account
      </h1>

      <div className="mt-6 grid grid-cols-2 gap-2.5">
        {AUTH_PROVIDERS.map((provider) => (
          <button
            key={provider.name}
            type="button"
            className={secondaryButton}
            onClick={() => showProviderNotice(provider.name)}
          >
            {provider.mark}
            {provider.name}
          </button>
        ))}
      </div>

      <button
        type="button"
        className={`${secondaryButton} mt-2.5 w-full`}
        onClick={() => showProviderNotice("SSO")}
      >
        <Lock size={14} strokeWidth={2} aria-hidden="true" />
        Continue with SSO
      </button>

      <p
        role="status"
        aria-live="polite"
        className={
          providerNotice
            ? "mt-3 flex items-start gap-1.5 rounded-lg border border-[#5c4a1e]! bg-[#2a2310] px-3 py-2 text-[12.5px] leading-4 text-[#f3c96b]"
            : "sr-only"
        }
      >
        {providerNotice ?? ""}
      </p>

      <div className="my-5 flex items-center gap-3 text-[11px] font-medium tracking-[0.08em] text-[#8b8b92]">
        <span className="h-px flex-1 bg-[#26262b]" />
        OR
        <span className="h-px flex-1 bg-[#26262b]" />
      </div>

      <form onSubmit={submit} noValidate>
        <label
          htmlFor="register-email"
          className="mb-1.5 block text-[13.5px] font-medium leading-4"
        >
          Email
        </label>
        <input
          id="register-email"
          type="email"
          autoComplete="email"
          spellCheck={false}
          {...form.register("email")}
          aria-invalid={emailError ? true : undefined}
          aria-describedby={emailError ? "register-email-error" : undefined}
          className={`${inputClass} ${emailError ? inputBorderError : inputBorder}`}
        />
        {emailError ? (
          <p id="register-email-error" className={errorText}>
            <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
            {emailError.message}
          </p>
        ) : null}

        <label
          htmlFor="register-password"
          className="mb-1.5 mt-4 block text-[13.5px] font-medium leading-4"
        >
          Password
        </label>
        <div className="relative">
          <input
            id="register-password"
            type={showPassword ? "text" : "password"}
            autoComplete="new-password"
            {...form.register("password")}
            aria-invalid={passwordError ? true : undefined}
            aria-describedby={
              passwordError
                ? "register-password-error"
                : "register-password-hint"
            }
            className={`${inputClass} pr-11 ${
              passwordError ? inputBorderError : inputBorder
            }`}
          />
          <button
            type="button"
            onClick={() => setShowPassword((value) => !value)}
            aria-label={showPassword ? "Hide password" : "Show password"}
            aria-pressed={showPassword}
            className="absolute right-1.5 top-1/2 flex size-8 -translate-y-1/2 items-center justify-center rounded-md text-[#9a9aa2] transition-colors hover:text-white"
          >
            {showPassword ? (
              <EyeOff size={17} strokeWidth={1.8} />
            ) : (
              <Eye size={17} strokeWidth={1.8} />
            )}
          </button>
        </div>
        <p
          id="register-password-hint"
          className="mt-1.5 text-[12.5px] leading-4 text-[#8b8b92]"
        >
          Minimum 12 characters.
        </p>
        {passwordError ? (
          <p id="register-password-error" className={errorText}>
            <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
            {passwordError.message}
          </p>
        ) : null}

        {mutation.error ? (
          <p role="alert" className={errorText}>
            <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
            {errorMessage(mutation.error)}
          </p>
        ) : null}

        <button
          type="submit"
          disabled={mutation.isPending}
          aria-busy={mutation.isPending}
          className={submitButton}
        >
          {mutation.isPending ? "Creating…" : "Create account"}
        </button>
      </form>

      <p className="mt-5 text-center text-[13.5px] leading-5 text-[#b3b3ba]">
        Already have an account?{" "}
        <Link href={signInHref} className={linkClass}>
          Sign in
        </Link>
      </p>

      <p className="mt-10 text-center text-[12.5px] leading-[18px] text-[#8b8b92]">
        By continuing, I agree to Stealth&apos;s{" "}
        <span className="underline">terms</span>,{" "}
        <span className="underline">privacy policy</span> and{" "}
        <span className="underline">cookie policy</span>.
      </p>
    </StandaloneAuthShell>
  );
}
