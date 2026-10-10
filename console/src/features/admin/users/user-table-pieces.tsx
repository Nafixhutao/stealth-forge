"use client";

import Image from "next/image";
import { Check } from "lucide-react";

/** Deterministic gradient avatar from the identity. When the account has a
    provider avatar it is shown as a background image (no next/image remote
    config needed); otherwise the initials gradient is used. */
export function GradientAvatar({
  email,
  url,
  size = 32,
}: {
  email: string;
  url?: string | null;
  size?: number;
}) {
  let hash = 0;
  const seed = email.toLowerCase();
  for (let index = 0; index < seed.length; index += 1) {
    hash = (hash * 31 + seed.charCodeAt(index)) % 360;
  }
  const initials = (
    seed.split(/[\s@._-]+/)[0]?.slice(0, 2) ?? "?"
  ).toUpperCase();

  if (url) {
    return (
      <span
        aria-hidden="true"
        className="shrink-0 rounded-full bg-[var(--projects-control)] bg-cover bg-center ring-1 ring-[var(--projects-border)]"
        style={{
          width: size,
          height: size,
          backgroundImage: `url(${url})`,
        }}
      />
    );
  }

  return (
    <span
      aria-hidden="true"
      className="flex shrink-0 items-center justify-center rounded-full font-semibold text-white/90"
      style={{
        width: size,
        height: size,
        fontSize: Math.round(size * 0.36),
        background: `linear-gradient(135deg, hsl(${hash} 42% 36%), hsl(${(hash + 42) % 360} 46% 22%))`,
        boxShadow: `inset 0 0 0 1px hsl(${hash} 30% 62% / 0.28)`,
      }}
    >
      {initials}
    </span>
  );
}

/**
 * Mask an email for the admin directory so the list can be shared or viewed
 * on a screen without exposing full addresses. The first and last character of
 * the local part stay visible; the domain is preserved so the provider is
 * still recognizable.
 */
export function maskEmail(email: string): string {
  const at = email.indexOf("@");
  if (at <= 0) return "•••";
  const local = email.slice(0, at);
  const domain = email.slice(at);
  if (local.length <= 2) return `${local[0] ?? ""}•••${domain}`;
  const head = local[0];
  const tail = local[local.length - 1];
  return `${head}${"•".repeat(Math.min(local.length - 2, 6))}${tail}${domain}`;
}

/** Instagram-style verified badge: a filled blue check. */
export function VerifiedBadge() {
  return (
    <span
      role="img"
      aria-label="Email verified"
      title="Email verified"
      className="inline-flex size-[15px] shrink-0 items-center justify-center rounded-full bg-[#3b82f6]"
    >
      <Check
        size={10}
        strokeWidth={3.4}
        aria-hidden="true"
        className="text-white"
      />
    </span>
  );
}

/** Provider mark for how the account signs in. Brand marks are inline SVGs so
    no icon package is added. */
export function ProviderMark({ provider }: { provider: string }) {
  if (provider === "github") {
    return (
      <span title="Signed in with GitHub" className="inline-flex items-center">
        <Image
          src="/github-mark.svg"
          alt="GitHub"
          width={14}
          height={14}
          unoptimized
          className="size-3.5 object-contain"
        />
      </span>
    );
  }
  if (provider === "google") {
    return (
      <span title="Signed in with Google" className="inline-flex items-center">
        <Image
          src="/google-g.svg"
          alt="Google"
          width={14}
          height={14}
          unoptimized
          className="size-3.5 object-contain"
        />
      </span>
    );
  }
  return (
    <span
      title="Signs in with a password"
      className="inline-flex items-center rounded border border-[var(--projects-border)] px-1 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--projects-muted)]"
    >
      pass
    </span>
  );
}

/** The sign-in methods an account has, with the most recent one emphasized. */
export function SignInMethods({
  providers,
  lastMethod,
}: {
  providers: string[];
  lastMethod?: string;
}) {
  const methods = providers.length > 0 ? providers : [];
  if (methods.length === 0) {
    return (
      <span className="text-[12px] text-[var(--projects-muted)]">Password</span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5">
      {methods.map((provider) => (
        <span
          key={provider}
          className={
            provider === lastMethod
              ? "rounded-md bg-white/[0.06] p-0.5 ring-1 ring-[var(--projects-accent)]/40"
              : "p-0.5"
          }
        >
          <ProviderMark provider={provider} />
        </span>
      ))}
    </span>
  );
}
