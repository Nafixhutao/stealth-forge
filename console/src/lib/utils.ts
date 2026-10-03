import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

const WHITESPACE = /\s+/;
const DASH_OR_UNDERSCORE = /[-_]/g;
const WORD_INITIAL = /\b\w/g;
const NON_SLUG = /[^a-z0-9]+/g;
const EDGE_HYPHENS = /^-+|-+$/g;
const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{1,62}$/;

export function getInitials(value: string) {
  let initials = "";
  for (const part of value.split(WHITESPACE)) {
    if (!part) continue;
    initials += part[0]?.toUpperCase() ?? "";
    if (initials.length === 2) break;
  }
  return initials;
}

export function humanize(value: string) {
  return value
    .replace(DASH_OR_UNDERSCORE, " ")
    .replace(WORD_INITIAL, (letter) => letter.toUpperCase());
}

export function toSlug(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(NON_SLUG, "-")
    .replace(EDGE_HYPHENS, "")
    .slice(0, 63);
}

export function isSlug(value: string) {
  return SLUG_PATTERN.test(value);
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
