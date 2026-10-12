"use client";

import Link from "next/link";
import Image from "next/image";
import { cloneElement, isValidElement, type ReactNode } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle2,
  CircleAlert,
  Cloud,
  Database,
  ExternalLink,
  Loader2,
  Terminal,
  Wifi,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { safeError } from "./helpers";

function fieldError(id: string, message: string | undefined) {
  return message ? (
    <p id={id} className="text-xs text-[#f0b4b7]" role="alert">
      {message}
    </p>
  ) : null;
}

export function BrandMark() {
  return (
    <div className="flex items-center gap-2.5">
      <Image
        src="/stealth-mark.png"
        alt="Stealth"
        width={32}
        height={32}
        className="size-8"
      />
      <span className="text-[14px] font-medium tracking-[-0.01em] text-[var(--setup-text)]">
        Stealth
      </span>
    </div>
  );
}

/**
 * Card used for the mutually-exclusive setup choices (network mode, database
 * mode, and so on): a radio-shaped tile that shows its selected state inline.
 */
export function ChoiceCard({
  active,
  icon: Icon,
  title,
  description,
  onClick,
}: {
  active: boolean;
  icon: typeof Cloud;
  title: string;
  description: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        "flex h-full min-h-24 w-full items-start gap-3 rounded-lg border p-4 text-left transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/15",
        active
          ? "border-[var(--setup-border-strong)] bg-[var(--setup-row-active)]"
          : "border-[var(--setup-border)] bg-[var(--setup-surface-raised)] hover:border-[var(--setup-border-strong)]",
      )}
    >
      <Icon
        className={cn(
          "mt-0.5 size-[18px] shrink-0",
          active
            ? "text-[var(--setup-text)]"
            : "text-[var(--setup-text-faint)]",
        )}
        strokeWidth={1.8}
      />
      <span className="min-w-0">
        <span className="block text-[13px] font-medium text-[var(--setup-text)]">
          {title}
        </span>
        <span className="mt-1 block text-[12px] leading-5 text-[var(--setup-text-muted)]">
          {description}
        </span>
      </span>
      {active ? (
        <Check className="ml-auto size-4 shrink-0 text-[var(--setup-text)]" />
      ) : null}
    </button>
  );
}

export function StatusPill({
  status,
  children,
}: {
  status: "ready" | "pending" | "warning" | "error";
  children: ReactNode;
}) {
  const styles = {
    ready: "border-[#76bc9a]/30 bg-[#76bc9a]/10 text-[#a3d6bd]",
    pending: "border-[var(--setup-border-strong)] bg-white/[0.04] text-[var(--setup-text-secondary)]",
    warning: "border-[#d4b36e]/30 bg-[#d4b36e]/10 text-[#ecd9b0]",
    error: "border-[#ea9198]/30 bg-[#ea9198]/10 text-[#f0b4b7]",
  }[status];
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-[5px] border px-1.5 py-1 text-[11px] font-medium",
        styles,
      )}
    >
      <span className="size-1.5 rounded-full bg-current" />
      {children}
    </span>
  );
}

export function ActionError({ error }: { error: unknown }) {
  if (!error) return null;
  return (
    <div
      className="flex items-start gap-2 rounded-lg border border-[#ea9198]/25 bg-[#ea9198]/[0.06] px-3 py-2.5 text-xs leading-5 text-[#f0b4b7]"
      role="alert"
    >
      <CircleAlert className="mt-0.5 size-4 shrink-0" />
      <span>{safeError(error)}</span>
    </div>
  );
}

export function Field({
  id,
  label,
  hint,
  error,
  className,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  error?: string;
  className?: string;
  children: ReactNode;
}) {
  const hintID = `${id}-hint`;
  const errorID = `${id}-error`;
  const control =
    error &&
    isValidElement<{
      "aria-describedby"?: string;
      "aria-invalid"?: boolean;
    }>(children)
      ? cloneElement(children, {
          "aria-describedby": hint ? `${hintID} ${errorID}` : errorID,
          "aria-invalid": true,
        })
      : children;

  return (
    <div className={cn("space-y-2", className)}>
      <Label htmlFor={id} className="text-[13px] text-[var(--setup-text-secondary)]">
        {label}
      </Label>
      {control}
      {hint ? (
        <p id={hintID} className="text-xs leading-5 text-[var(--setup-text-muted)]">
          {hint}
        </p>
      ) : null}
      {fieldError(errorID, error)}
    </div>
  );
}

export { Field as BrowserSetupField };

export function StageHeader({
  eyebrow,
  title,
  description,
}: {
  eyebrow: string;
  title: string;
  description: string;
}) {
  return (
    <header className="mb-6">
      <p className="mb-2.5 font-mono text-[11px] font-medium uppercase tracking-[0.12em] text-[var(--setup-text-muted)]">
        {eyebrow}
      </p>
      <h1 className="text-[22px] font-semibold leading-[1.15] tracking-[-0.02em] text-balance text-[var(--setup-text)] sm:text-[24px]">
        {title}
      </h1>
      <p className="mt-3 max-w-[62ch] text-[13px] leading-6 text-pretty text-[var(--setup-text-muted)]">
        {description}
      </p>
    </header>
  );
}

export function StageActions({
  back,
  next,
  nextLabel = "Continue",
  nextDisabled,
  pending,
  arrow = true,
}: {
  back?: () => void;
  next?: () => void;
  nextLabel?: string;
  nextDisabled?: boolean;
  pending?: boolean;
  /** Forward-navigation cue. Action buttons (Save, Install, Test) set this
      false: an arrow means "goes to the next screen", not "runs a command". */
  arrow?: boolean;
}) {
  return (
    <div className="mt-8 flex flex-col-reverse gap-3 border-t border-[var(--setup-border)] pt-5 sm:flex-row sm:items-center sm:justify-between">
      {back ? (
        <Button type="button" variant="ghost" onClick={back}>
          <ArrowLeft className="size-4" /> Back
        </Button>
      ) : (
        <span />
      )}
      {next ? (
        <Button type="button" onClick={next} disabled={nextDisabled || pending}>
          {pending ? <Loader2 className="size-4 animate-spin" /> : null}
          {pending ? "Saving…" : nextLabel}
          {!pending && arrow ? <ArrowRight className="size-4" /> : null}
        </Button>
      ) : null}
    </div>
  );
}

export function ReviewRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1 border-b border-[var(--setup-border)] px-4 py-3 last:border-b-0 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
      <dt className="font-mono text-[11px] uppercase tracking-[0.12em] text-[var(--setup-text-muted)]">
        {label}
      </dt>
      <dd className="break-all text-[13px] text-[var(--setup-text-secondary)] sm:text-right">
        {value}
      </dd>
    </div>
  );
}

export function ProductionLink({ publicURL }: { publicURL?: string }) {
  return (
    <div className="flex flex-col gap-4 rounded-lg border border-[#76bc9a]/25 bg-[#76bc9a]/[0.05] p-5">
      <div className="flex items-start gap-3">
        <CheckCircle2 className="mt-0.5 size-[18px] shrink-0 text-[#76bc9a]" />
        <div>
          <p className="text-[13px] font-medium text-[#a3d6bd]">
            Production is ready
          </p>
          <p className="mt-1 text-xs leading-5 text-[#a3d6bd]/70">
            The setup route is sealed. Open the production Console to sign in.
          </p>
        </div>
      </div>
      <Button asChild variant="secondary" className="w-full sm:w-auto">
        <Link href={publicURL ? `${publicURL}/login` : "/login"}>
          Open production Console <ExternalLink className="size-4" />
        </Link>
      </Button>
    </div>
  );
}

export function LoadingPanel({ label }: { label: string }) {
  return (
    <div
      className="flex items-center gap-3 rounded-lg border border-[var(--setup-border)] bg-[var(--setup-surface-raised)] p-6 text-[13px] text-[var(--setup-text-muted)]"
      aria-live="polite"
    >
      <Loader2 className="size-[18px] animate-spin text-[var(--setup-text-muted)]" />
      {label}
    </div>
  );
}

/**
 * Three short assurances about how setup behaves, rendered as one panel split
 * by hairlines instead of three floating cards, so it reads as a single
 * statement about the process rather than a generic feature-card row.
 */
export function AssuranceStrip({
  items,
}: {
  items: Array<{
    icon: typeof Terminal;
    title: string;
    description: string;
  }>;
}) {
  return (
    <div className="mt-6 grid gap-px overflow-hidden rounded-lg border border-[var(--setup-border)] bg-[var(--setup-border)] sm:grid-cols-3">
      {items.map(({ icon: Icon, title, description }) => (
        <div
          key={title}
          className="flex flex-col gap-2 bg-[var(--setup-surface-raised)] p-4"
        >
          <span className="flex items-center gap-2">
            <Icon
              className="size-4 shrink-0 text-[var(--setup-text-muted)]"
              strokeWidth={1.8}
            />
            <span className="text-[13px] font-medium text-[var(--setup-text)]">
              {title}
            </span>
          </span>
          <p className="text-[12px] leading-5 text-[var(--setup-text-muted)]">
            {description}
          </p>
        </div>
      ))}
    </div>
  );
}

export function DependencySection({
  icon: Icon,
  title,
  mode,
  onModeChange,
  tested,
  testing,
  onTest,
  children,
}: {
  icon: typeof Database;
  title: string;
  mode: "bundled" | "external";
  onModeChange: (value: "bundled" | "external") => void;
  tested: boolean;
  testing: boolean;
  onTest: () => void;
  children: ReactNode;
}) {
  return (
    <section className="rounded-lg border border-[var(--setup-border)] bg-[var(--setup-surface-raised)] p-5">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <Icon
            className="mt-0.5 size-[18px] shrink-0 text-[var(--setup-text-faint)]"
            strokeWidth={1.8}
          />
          <div>
            <h2 className="text-[13px] font-medium text-[var(--setup-text)]">
              {title}
            </h2>
            <p className="mt-1 text-[12px] text-[var(--setup-text-muted)]">
              {mode === "bundled"
                ? "Managed by this setup Compose project."
                : "Your endpoint is tested before install."}
            </p>
          </div>
        </div>
        <div className="flex gap-1 rounded-md border border-[var(--setup-border)] bg-[var(--setup-bg)] p-1">
          {(["bundled", "external"] as const).map((option) => (
            <button
              key={option}
              type="button"
              onClick={() => onModeChange(option)}
              className={cn(
                "min-h-9 rounded-[5px] px-3 text-[12px] font-medium transition-colors duration-150",
                mode === option
                  ? "bg-[var(--setup-row-active)] text-[var(--setup-text)]"
                  : "text-[var(--setup-text-muted)] hover:text-[var(--setup-text)]",
              )}
            >
              {option === "bundled" ? "Bundled" : "External"}
            </button>
          ))}
        </div>
      </div>
      {mode === "external" ? (
        <div className="mt-5 space-y-4">
          {children}
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="button"
              variant="outline"
              onClick={onTest}
              disabled={testing}
            >
              {testing ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Wifi className="size-4" />
              )}{" "}
              Test {title}
            </Button>
            {tested ? (
              <StatusPill status="ready">Connection tested</StatusPill>
            ) : (
              <StatusPill status="warning">Test required</StatusPill>
            )}
          </div>
        </div>
      ) : null}
    </section>
  );
}
