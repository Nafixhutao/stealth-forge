"use client";

import { useSyncExternalStore } from "react";

/**
 * The organization the user console is currently showing.
 *
 * The organization switcher lives on the home page, but the navigation panel —
 * a sibling mounted by the application shell — shows that organization's plan.
 * The selection therefore cannot stay page-local state: it is a module-level
 * store the panel subscribes to, the same shape the desktop rail already uses to
 * remember its collapsed width across route changes.
 *
 * Project routes do not read this: a project belongs to an organization, and the
 * project's own organization wins there.
 */

let currentOrganizationId: string | null = null;
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function getSnapshot() {
  return currentOrganizationId;
}

/** Server rendering has no selection yet; the first paint is the same either way. */
function getServerSnapshot() {
  return null;
}

export function setCurrentOrganizationId(organizationId: string | null) {
  if (currentOrganizationId === organizationId) return;
  currentOrganizationId = organizationId;
  for (const listener of listeners) listener();
}

export function useCurrentOrganizationId() {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
