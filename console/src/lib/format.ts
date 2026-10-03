import { formatDistanceToNowStrict, format } from "date-fns";

// Intl formatter construction is comparatively expensive and formatCount runs
// during list rendering, so the two notations are built once at module scope.
const compactCountFormat = new Intl.NumberFormat("en-US", {
  notation: "compact",
});
const standardCountFormat = new Intl.NumberFormat("en-US", {
  notation: "standard",
});

export function formatDate(value: string | null | undefined) {
  if (!value) return "Not available";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "Not available";
  return format(date, "MMM d, yyyy · HH:mm");
}

export function formatRelative(value: string | null | undefined) {
  if (!value) return "Not available";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "Not available";
  return `${formatDistanceToNowStrict(date, { addSuffix: true })}`;
}

export function formatBytes(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value))
    return "Not available";
  if (value < 1024) return `${value} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let amount = value;
  let unit = -1;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${amount.toFixed(amount >= 10 ? 0 : 1)} ${units[unit]}`;
}

export function formatCount(value: number | null | undefined) {
  if (value === null || value === undefined) return "Not available";
  return (value > 9999 ? compactCountFormat : standardCountFormat).format(
    value,
  );
}

export function formatDuration(value: number | null | undefined) {
  if (value === null || value === undefined) return "Not available";
  if (value < 1000) return `${value} ms`;
  return `${(value / 1000).toFixed(2)} s`;
}
