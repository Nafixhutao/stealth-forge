"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";

/**
 * The header star is a personal bookmark, not shared state: the API has no
 * favorite field, so the list lives in this browser.
 *
 * It is read through `useSyncExternalStore` rather than an effect, so the
 * server renders an empty list, the first client frame agrees with it, and the
 * stored value arrives immediately afterwards without a second render pass.
 */
const STORAGE_KEY = "stealth:agent-favorites";
const listeners = new Set<() => void>();

function parse(raw: string): string[] {
  try {
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed)
      ? parsed.filter((value): value is string => typeof value === "string")
      : [];
  } catch {
    // A corrupt value is not worth failing the page over; start clean.
    return [];
  }
}

function subscribe(onChange: () => void) {
  listeners.add(onChange);
  // Another tab editing the same list is a real change to show.
  window.addEventListener("storage", onChange);
  return () => {
    listeners.delete(onChange);
    window.removeEventListener("storage", onChange);
  };
}

function getSnapshot() {
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? "";
  } catch {
    // Private mode without storage: the star still works for this visit.
    return "";
  }
}

export function useFavoriteAgents() {
  const raw = useSyncExternalStore(subscribe, getSnapshot, () => "");
  const favorites = useMemo(() => parse(raw), [raw]);

  const toggle = useCallback((agentId: string) => {
    const current = parse(getSnapshot());
    const next = current.includes(agentId)
      ? current.filter((id) => id !== agentId)
      : [...current, agentId];
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    } catch {
      // Storage refused the write; the list stays as it was.
      return;
    }
    for (const listener of listeners) listener();
  }, []);

  return { favorites, toggle };
}
