"use client";

import Link from "next/link";
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
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { safeError } from "./helpers";

function fieldError(id: string, message: string | undefined) {
  return message ? (
    <p id={id} className="text-xs text-rose-300" role="alert">
      {message}
    </p>
  ) : null;
}

export function BrandMark() {
  return (
    <div className="flex items-center gap-2.5">
      <span className="flex size-9 items-center justify-center rounded-md border border-acid-lime/40 bg-acid-lime text-sm font-semibold text-void">
        S
      </span>
      <span className="text-sm font-semibold tracking-[-0.012em] text-paper">
        Stealth
      </span>
    </div>
  );
}

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
      className={`flex min-h-28 w-full items-start gap-3 rounded-lg border p-4 text-left transition-colors duration-150 focus-visible:ring-2 focus-visible:ring-acid-lime/40 ${active ? "border-acid-lime/60 bg-acid-lime/[0.08]" : "border-graphite bg-carbon hover:border-smoke"}`}
    >
      <Icon
        className={`mt-0.5 size-5 shrink-0 ${active ? "text-acid-lime" : "text-fog"}`}
      />
      <span>
        <span className="block text-sm font-medium text-paper">{title}</span>
        <span className="mt-1 block text-xs leading-5 text-fog">
          {description}
        </span>
      </span>
      {active ? (
        <Check className="ml-auto size-4 shrink-0 text-acid-lime" />
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
    ready: "border-emerald-300/25 bg-emerald-300/10 text-emerald-200",
    pending: "border-acid-lime/25 bg-acid-lime/10 text-mist",
    warning: "border-amber-300/25 bg-amber-300/10 text-amber-200",
    error: "border-rose-300/25 bg-rose-300/10 text-rose-200",
  }[status];
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-xs border px-1.5 py-1 text-xs font-medium ${styles}`}
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
      className="flex items-start gap-2 rounded-lg border border-rose-300/20 bg-rose-400/[0.08] px-3 py-2.5 text-xs leading-5 text-rose-200"
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
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  error?: string;
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
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {control}
      {hint ? (
        <p id={hintID} className="text-xs leading-5 text-slate-600">
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
    <header className="mb-7">
      <p className="mb-2 text-xs font-medium uppercase tracking-[0.14em] text-fog">
        {eyebrow}
      </p>
      <h1 className="text-2xl font-semibold leading-tight tracking-[-0.022em] text-paper sm:text-3xl">
        {title}
      </h1>
      <p className="mt-3 max-w-2xl text-sm leading-6 text-fog">{description}</p>
    </header>
  );
}

export function StageActions({
  back,
  next,
  nextLabel = "Continue",
  nextDisabled,
  pending,
}: {
  back?: () => void;
  next?: () => void;
  nextLabel?: string;
  nextDisabled?: boolean;
  pending?: boolean;
}) {
  return (
    <div className="mt-8 flex flex-col-reverse gap-3 border-t border-stealth-border pt-5 sm:flex-row sm:items-center sm:justify-between">
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
          {pending ? "Saving" : nextLabel}
          {!pending ? <ArrowRight className="size-4" /> : null}
        </Button>
      ) : null}
    </div>
  );
}

export function ReviewRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1 border-b border-stealth-border/70 py-3 last:border-b-0 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
      <dt className="text-xs uppercase tracking-[0.12em] text-slate-600">
        {label}
      </dt>
      <dd className="break-all text-sm text-slate-200 sm:text-right">
        {value}
      </dd>
    </div>
  );
}

export function ProductionLink({ publicURL }: { publicURL?: string }) {
  return (
    <div className="flex flex-col gap-4 rounded-xl border border-emerald-300/20 bg-emerald-300/[0.06] p-5">
      <div className="flex items-start gap-3">
        <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-emerald-300" />
        <div>
          <p className="text-sm font-medium text-emerald-100">
            Production is ready
          </p>
          <p className="mt-1 text-xs leading-5 text-emerald-200/70">
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
    <Card className="mx-auto max-w-xl bg-carbon">
      <CardContent
        className="flex items-center gap-3 p-6 text-sm text-fog"
        aria-live="polite"
      >
        <Loader2 className="size-5 animate-spin text-acid-lime" /> {label}
      </CardContent>
    </Card>
  );
}

export function InfoCard({
  icon: Icon,
  title,
  description,
}: {
  icon: typeof Terminal;
  title: string;
  description: string;
}) {
  return (
    <div className="rounded-lg border border-graphite bg-carbon p-4">
      <Icon className="size-5 text-fog" />
      <p className="mt-4 text-sm font-medium text-paper">{title}</p>
      <p className="mt-1 text-xs leading-5 text-fog">{description}</p>
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
    <section className="rounded-lg border border-graphite bg-carbon/40 p-5">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <Icon className="mt-0.5 size-5 shrink-0 text-fog" />
          <div>
            <h2 className="text-sm font-medium text-paper">{title}</h2>
            <p className="mt-1 text-xs text-fog">
              {mode === "bundled"
                ? "Managed by this setup Compose project."
                : "Your endpoint is tested before install."}
            </p>
          </div>
        </div>
        <div className="flex gap-1 rounded-md border border-graphite p-1">
          {(["bundled", "external"] as const).map((option) => (
            <button
              key={option}
              type="button"
              onClick={() => onModeChange(option)}
              className={`min-h-11 rounded-sm px-3 text-xs font-medium transition-colors duration-150 ${mode === option ? "bg-acid-lime text-void" : "text-fog hover:text-paper"}`}
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
