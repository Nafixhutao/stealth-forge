"use client";

import { useCallback, useSyncExternalStore } from "react";

/** Device-local memory of the last sign-in method, used to badge the matching
    button with "Last used" like other developer consoles. It is a convenience
    only, never a security signal, and every storage access fails closed. */
const LAST_METHOD_KEY = "stealth.login.last-method:v1";
const LAST_METHOD_EVENT = "stealth:last-auth-method";

export type AuthMethod = "google" | "github" | "password";

function isAuthMethod(value: string | null): value is AuthMethod {
  return value === "google" || value === "github" || value === "password";
}

export function readLastAuthMethod(): AuthMethod | null {
  try {
    const raw = window.localStorage.getItem(LAST_METHOD_KEY);
    return isAuthMethod(raw) ? raw : null;
  } catch {
    return null;
  }
}

export function writeLastAuthMethod(method: AuthMethod) {
  try {
    window.localStorage.setItem(LAST_METHOD_KEY, method);
  } catch {
    // Ignored: a storage failure must never block sign-in.
  }
  // Same-tab `storage` events do not fire, so notify listeners directly.
  window.dispatchEvent(new Event(LAST_METHOD_EVENT));
}

function subscribe(onChange: () => void) {
  window.addEventListener("storage", onChange);
  window.addEventListener(LAST_METHOD_EVENT, onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener(LAST_METHOD_EVENT, onChange);
  };
}

/** Reads the stored method through an external store so the server render
    (null) and the client render reconcile without a synchronous setState in
    an effect. */
export function useLastAuthMethod(): [
  AuthMethod | null,
  (m: AuthMethod) => void,
] {
  const method = useSyncExternalStore(
    subscribe,
    readLastAuthMethod,
    () => null,
  );
  const remember = useCallback((next: AuthMethod) => {
    writeLastAuthMethod(next);
  }, []);
  return [method, remember];
}
