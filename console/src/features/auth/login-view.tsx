"use client";

import { useEffect, useState, type MouseEvent, type ReactNode } from "react";
import Link from "next/link";
import Image from "next/image";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { Check, CircleAlert, Eye, EyeOff, Lock } from "lucide-react";
import { useLogin } from "@/api/mutations";
import { errorMessage } from "@/components/feedback/error-state";
import { getSafeNextPath } from "@/lib/navigation";

const schema = z.object({
  email: z.string().email("Enter a valid email address."),
  password: z.string().min(1, "Enter your password."),
});
type FormValues = z.infer<typeof schema>;

/** Remembered email is a device-local convenience, not an auth feature. */
const REMEMBERED_EMAIL_KEY = "stealth.login.email";

// Shared style tokens for the standalone sign-in screen, ported verbatim from
// the Stealth marketing console design so the screen stays pixel-identical.
// The trailing `!` on border/type utilities beats the Console's unlayered
// global resets (`* { border-color }`, `button { font: inherit }`); the spacing,
// radii, and font weights are restored by [data-login-standalone] in
// styles/login-standalone.css.
const secondaryButton =
  "flex h-10 items-center justify-center gap-2 rounded-lg border border-[#333338]! bg-[#0f0f0f] text-[13.5px]! font-medium! text-[#f2f2f3] transition-colors hover:border-[#4a4a50]! hover:bg-[#1c1c1c] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#186cee]!";
const inputClass =
  "h-10 w-full rounded-lg border bg-[#171717] px-3.5 text-[14px]! text-white outline-none transition-colors placeholder:text-[#6b6b72]! focus-visible:outline-none!";
const inputBorder = "border-[#2a2e38]! focus:border-[#186cee]!";
const inputBorderError = "border-[#e5484d]! focus:border-[#e5484d]!";
const errorText =
  "mt-1.5 flex items-center gap-1.5 text-[12.5px] leading-4 text-[#f87171]";
const linkClass =
  "font-medium! text-[#4d8dff] underline underline-offset-2 hover:text-[#7fabff]";
const submitButton =
  "mt-6 h-10 w-full rounded-lg bg-[linear-gradient(180deg,#2e83f7_0%,#186cee_48%,#0e5cd6_100%)] text-[14px]! font-semibold! text-white transition-[filter,opacity] hover:brightness-110 disabled:cursor-default disabled:opacity-70 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#186cee]!";

const preventPlaceholderNav = (event: MouseEvent<HTMLAnchorElement>) =>
  event.preventDefault();

// Brand marks are tiny inline SVGs — not worth an icon-package dependency.
function GoogleMark() {
  return (
    <svg viewBox="0 0 24 24" className="size-4" aria-hidden="true">
      <path
        fill="#4285F4"
        d="M23.49 12.27c0-.79-.07-1.54-.19-2.27H12v4.51h6.47a5.57 5.57 0 0 1-2.4 3.58v3h3.86c2.26-2.09 3.56-5.17 3.56-8.82z"
      />
      <path
        fill="#34A853"
        d="M12 24c3.24 0 5.95-1.08 7.93-2.91l-3.86-3c-1.08.72-2.45 1.16-4.07 1.16-3.13 0-5.78-2.11-6.73-4.96H1.29v3.09A11.99 11.99 0 0 0 12 24z"
      />
      <path
        fill="#FBBC05"
        d="M5.27 14.29A7.16 7.16 0 0 1 4.89 12c0-.8.14-1.57.38-2.29V6.62H1.29a11.99 11.99 0 0 0 0 10.76l3.98-3.09z"
      />
      <path
        fill="#EA4335"
        d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.42-3.42C17.95 1.19 15.24 0 12 0 7.31 0 3.26 2.7 1.29 6.62l3.98 3.09C6.22 6.86 8.87 4.75 12 4.75z"
      />
    </svg>
  );
}

function GitHubMark() {
  return (
    <svg viewBox="0 0 24 24" className="size-4 fill-white" aria-hidden="true">
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.55 0-.27-.01-1.17-.02-2.12-3.2.7-3.87-1.36-3.87-1.36-.52-1.33-1.28-1.68-1.28-1.68-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.18 1.76 1.18 1.03 1.76 2.69 1.25 3.35.96.1-.75.4-1.25.72-1.54-2.55-.29-5.23-1.28-5.23-5.68 0-1.26.45-2.28 1.18-3.09-.12-.29-.51-1.46.11-3.05 0 0 .96-.31 3.15 1.18a10.9 10.9 0 0 1 5.74 0c2.19-1.49 3.15-1.18 3.15-1.18.62 1.59.23 2.76.11 3.05.74.81 1.18 1.83 1.18 3.09 0 4.41-2.69 5.38-5.25 5.67.41.35.77 1.05.77 2.12 0 1.53-.01 2.76-.01 3.14 0 .3.2.67.8.55A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z" />
    </svg>
  );
}

const SIGNIN_PROVIDERS: Array<{ name: string; mark: ReactNode }> = [
  { name: "Google", mark: <GoogleMark /> },
  { name: "GitHub", mark: <GitHubMark /> },
];

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
      router.replace(destination);
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
    <main
      data-login-standalone
      className="relative flex min-h-dvh flex-col items-center bg-[#0f0f0f] px-4 py-14 font-medium text-white sm:py-10"
    >
      <a
        href="#login-main-content"
        className="sr-only fixed left-4 top-4 z-[60] rounded-md bg-acid-lime px-3 py-2 text-sm font-medium text-void focus:not-sr-only"
      >
        Skip to content
      </a>
      <div id="login-main-content" className="my-auto w-full max-w-[364px]">
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
          {SIGNIN_PROVIDERS.map((provider) => (
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
          <Link href="/recovery" className={linkClass}>
            email
          </Link>{" "}
          or{" "}
          <Link href="/recovery" className={linkClass}>
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
      </div>
    </main>
  );
}
