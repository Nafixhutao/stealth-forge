"use client";

import { useState } from "react";
import { Users } from "lucide-react";

/** Deterministic pastel avatar: hash the identity to pick a hue pair so the
    same user always renders the same gradient, without any image request. */
function hueFrom(seed: string): number {
  let hash = 0;
  for (let index = 0; index < seed.length; index += 1) {
    hash = (hash * 31 + seed.charCodeAt(index)) % 360;
  }
  return hash;
}

export function UserAvatar({
  email,
  name,
  size = 36,
}: {
  email: string;
  name?: string | null;
  size?: number;
}) {
  const hue = hueFrom(email.toLowerCase());
  const initials =
    (name?.trim() || email)
      .split(/[\s@._-]+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase() ?? "")
      .join("") || "?";

  return (
    <span
      aria-hidden="true"
      className="flex shrink-0 items-center justify-center rounded-full font-semibold text-white/90"
      style={{
        width: size,
        height: size,
        fontSize: Math.round(size * 0.36),
        background: `linear-gradient(135deg, hsl(${hue} 42% 34%), hsl(${(hue + 40) % 360} 48% 22%))`,
        boxShadow: `inset 0 0 0 1px hsl(${hue} 30% 60% / 0.25)`,
      }}
    >
      {initials}
    </span>
  );
}

/** Compact stat strip for the users list: total, active, blocked, pending. */
export function UsersStatStrip({
  total,
  active,
  blocked,
  pending,
}: {
  total: number;
  active: number;
  blocked: number;
  pending: number;
}) {
  const stats = [
    { label: "Total users", value: total, tone: "text-white" },
    { label: "Active", value: active, tone: "text-emerald-300" },
    { label: "Blocked", value: blocked, tone: "text-rose-300" },
    { label: "Unverified", value: pending, tone: "text-amber-300" },
  ];

  return (
    <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4 sm:gap-3">
      {stats.map((stat) => (
        <div
          key={stat.label}
          className="flex items-center gap-3 rounded-lg border border-stealth-border bg-[#141416] p-3 sm:p-3.5"
        >
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-stealth-border bg-white/[0.03] text-slate-400">
            <Users size={14} strokeWidth={1.8} aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <p
              className={`admin-tnum m-0 text-[18px] font-semibold leading-6 ${stat.tone}`}
            >
              {stat.value}
            </p>
            <p className="m-0 truncate text-[11.5px] leading-4 text-slate-500">
              {stat.label}
            </p>
          </div>
        </div>
      ))}
    </div>
  );
}

/** Relative "3d ago" joined label. The snapshot time comes from a state
    initialized once per mount, so render stays pure and the label is stable
    for the lifetime of the view — the absolute date stays in the tooltip. */
export function RelativeDate({ iso }: { iso: string }) {
  const [now] = useState(() => Date.now());
  const date = new Date(iso);
  const seconds = Math.max(0, (now - date.getTime()) / 1000);
  let label: string;
  if (seconds < 60) label = "just now";
  else if (seconds < 3600) label = `${Math.round(seconds / 60)}m ago`;
  else if (seconds < 86400) label = `${Math.round(seconds / 3600)}h ago`;
  else label = `${Math.round(seconds / 86400)}d ago`;

  return (
    <span title={date.toLocaleString()} className="whitespace-nowrap">
      {label}
    </span>
  );
}
