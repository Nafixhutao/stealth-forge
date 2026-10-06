"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import Image from "next/image";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { Check, CircleAlert, Eye, EyeOff, Lock } from "lucide-react";
import { useLogin } from "@/api/mutations";
import { fetchCurrentAccount } from "@/api/queries/account";
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
  preventPlaceholderNav,
  StandaloneAuthShell,
} from "./auth-standalone";

const schema = z.object({
  email: z.string().email("Enter a valid email address."),
  password: z.string().min(1, "Enter your password."),
});
type FormValues = z.infer<typeof schema>;

/** Remembered email is a device-local convenience, not an auth feature. */
const REMEMBERED_EMAIL_KEY = "stealth.login.email";

/**
 * Standalone sign-in screen, ported from the Stealth marketing console design.
 * It renders its own full-page shell instead of the shared two-column auth
 * layout. Email + password talks to the real session API; the provider buttons
 * render the target design but report honestly until an OAuth/SSO adapter
 * exists on the instance.
 */
export function LoginView() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const mutation = useLogin();
  const rawNext = searchParams.get("next");
  const destination = getSafeNextPath(rawNext);
  const registerHref =
    rawNext === null
      ? "/register"
      : "/register?next=" + encodeURIComponent(destination);
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "" },
  });
  const [showPassword, setShowPassword] = useState(false);
  const [rememberEmail, setRememberEmail] = useState(true);
  const [providerNotice, setProviderNotice] = useState<string | null>(null);

  // Prefill the remembered email once per mount. The password never leaves
  // the browser session.
  useEffect(() => {
    const saved = window.localStorage.getItem(REMEMBERED_EMAIL_KEY);
    if (saved) form.setValue("email", saved);
  }, [form]);

  const submit = form.handleSubmit(async (values) => {
    if (rememberEmail) {
      window.localStorage.setItem(REMEMBERED_EMAIL_KEY, values.email.trim());
    } else {
      window.localStorage.removeItem(REMEMBERED_EMAIL_KEY);
    }
    try {
      await mutation.mutateAsync(values);
      // Instance owners and admins operate the platform, so send them to the
      // Admin Console. An explicit ?next= always wins (invitations, deep links).
      let target = destination;
      if (rawNext === null) {
        try {
          const account = await fetchCurrentAccount();
          const role = account?.account.instance_role;
          if (role === "instance_owner" || role === "instance_admin") {
            target = "/admin";
          }
        } catch {
          // Fall back to the default destination if the role cannot be read.
        }
      }
      router.replace(target);
    } catch {
      // The mutation error is rendered below; handleSubmit re-throws, so
      // swallowing here avoids an unhandled promise rejection.
    }
  });

  const showProviderNotice = (provider: string) =>
    setProviderNotice(
      `Sign-in with ${provider} is not configured on this instance yet. Use your email and password.`,
    );

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
        Sign in to Stealth
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
          htmlFor="login-email"
          className="mb-1.5 block text-[13.5px] font-medium leading-4"
        >
          Email
        </label>
        <input
          id="login-email"
          type="email"
          autoComplete="email"
          spellCheck={false}
          {...form.register("email")}
          aria-invalid={form.formState.errors.email ? true : undefined}
          aria-describedby={
            form.formState.errors.email ? "login-email-error" : undefined
          }
          className={`${inputClass} ${
            form.formState.errors.email ? inputBorderError : inputBorder
          }`}
        />
        {form.formState.errors.email ? (
          <p id="login-email-error" className={errorText}>
            <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
            {form.formState.errors.email.message}
          </p>
        ) : null}

        <label
          htmlFor="login-password"
          className="mb-1.5 mt-4 block text-[13.5px] font-medium leading-4"
        >
          Password
        </label>
        <div className="relative">
          <input
            id="login-password"
            type={showPassword ? "text" : "password"}
            autoComplete="current-password"
            {...form.register("password")}
            aria-invalid={form.formState.errors.password ? true : undefined}
            aria-describedby={
              form.formState.errors.password
                ? "login-password-error"
                : undefined
            }
            className={`${inputClass} pr-11 ${
              form.formState.errors.password ? inputBorderError : inputBorder
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
        {form.formState.errors.password ? (
          <p id="login-password-error" className={errorText}>
            <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
            {form.formState.errors.password.message}
          </p>
        ) : null}

        {mutation.error ? (
          <p
            role="alert"
            className="mt-1.5 flex items-center gap-1.5 text-[12.5px] leading-4 text-[#f87171]"
          >
            <CircleAlert size={13} strokeWidth={2} aria-hidden="true" />
            {errorMessage(mutation.error)}
          </p>
        ) : null}

        <label className="mt-6 flex cursor-pointer select-none items-center gap-2.5 text-[13.5px] leading-4">
          <input
            type="checkbox"
            checked={rememberEmail}
            onChange={(event) => setRememberEmail(event.target.checked)}
            className="peer sr-only"
          />
          <span
            aria-hidden="true"
            className="flex size-[18px] shrink-0 items-center justify-center rounded-[5px] border-2 border-[#57575f]! bg-transparent transition-colors peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-[#186cee] peer-checked:border-[#186cee]! peer-checked:bg-[#186cee]"
          >
            {rememberEmail ? (
              <Check size={13} strokeWidth={3} className="text-white" />
            ) : null}
          </span>
          Save email and login method on this device
        </label>

        <button
          type="submit"
          disabled={mutation.isPending}
          aria-busy={mutation.isPending}
          className={submitButton}
        >
          {mutation.isPending ? "Signing in…" : "Sign in"}
        </button>
      </form>

      {searchParams.get("reset") === "success" ? (
        <p className="mt-4 rounded-lg border border-[#1e4433]! bg-[#12241b] px-3 py-2 text-center text-[12.5px] leading-4 text-[#4ade80]">
          Password updated. Sign in with your new password.
        </p>
      ) : null}

      <p className="mt-5 text-center text-[13.5px] leading-5 text-[#b3b3ba]">
        Don&apos;t have an account?{" "}
        <Link href={registerHref} className={linkClass}>
          Sign up
        </Link>
      </p>
      <p className="mt-1 text-center text-[13.5px] leading-5 text-[#b3b3ba]">
        Forgot your{" "}
        <Link href="/forgot-password" className={linkClass}>
          email
        </Link>{" "}
        or{" "}
        <Link href="/forgot-password" className={linkClass}>
          password
        </Link>
        ?
      </p>

      <p className="mt-10 text-center text-[12.5px] leading-[18px] text-[#8b8b92]">
        By continuing, I agree to Stealth&apos;s{" "}
        <a
          href="#"
          onClick={preventPlaceholderNav}
          className="underline hover:text-white"
        >
          terms
        </a>
        ,{" "}
        <a
          href="#"
          onClick={preventPlaceholderNav}
          className="underline hover:text-white"
        >
          privacy policy
        </a>{" "}
        and{" "}
        <a
          href="#"
          onClick={preventPlaceholderNav}
          className="underline hover:text-white"
        >
          cookie policy
        </a>
        .
      </p>
    </StandaloneAuthShell>
  );
}
