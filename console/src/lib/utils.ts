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

// Segment by grapheme, not UTF-16 code unit, so emoji and combined scripts
// produce a whole character instead of half a surrogate pair.
const GRAPHEME_SEGMENTER =
  typeof Intl !== "undefined" && typeof Intl.Segmenter === "function"
    ? new Intl.Segmenter(undefined, { granularity: "grapheme" })
    : null;

function firstGrapheme(value: string) {
  if (!value) return "";
  if (GRAPHEME_SEGMENTER) {
    for (const segment of GRAPHEME_SEGMENTER.segment(value)) {
      return segment.segment;
    }
    return "";
  }
  return Array.from(value)[0] ?? "";
}

export function getInitials(value: string) {
  const parts = value.split(WHITESPACE).filter(Boolean);
  if (!parts.length) return "";
  const first = firstGrapheme(parts[0]);
  const last = parts.length > 1 ? firstGrapheme(parts[parts.length - 1]) : "";
  return `${first}${last}`.toUpperCase();
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
