"use client";

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Re-renders on a fixed interval and returns a tick counter.
 * Tick 0 is the deterministic pre-hydration render — callers must return
 * their base mock data for it and only nudge values once tick > 0.
 * The interval is cleaned up on unmount.
 */
export function useLiveTick(intervalMs = 4000, enabled = true): number {
  const [tick, setTick] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    const id = window.setInterval(() => setTick((value) => value + 1), intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs, enabled]);

  return tick;
}

/** Seconds since mount or the last reset — drives "Last updated Xs ago". */
export function useElapsedSeconds(resetKey?: unknown): { seconds: number; reset: () => void } {
  const [seconds, setSeconds] = useState(0);
  const startRef = useRef(0);

  const reset = useCallback(() => {
    startRef.current = Date.now();
    setSeconds(0);
  }, []);

  useEffect(() => {
    startRef.current = Date.now();
    setSeconds(0);
  }, [resetKey]);

  useEffect(() => {
    const id = window.setInterval(() => {
      setSeconds(Math.floor((Date.now() - startRef.current) / 1000));
    }, 1000);
    return () => window.clearInterval(id);
  }, []);

  return { seconds, reset };
}

/** Move a mock value a small random step, clamped to [min, max]. */
export function nudge(value: number, amplitude: number, min: number, max: number): number {
  const next = value + (Math.random() - 0.5) * 2 * amplitude;
  return Math.min(max, Math.max(min, next));
}

/** Append a point to a sparkline history, keeping it bounded. */
export function pushHistory(history: number[], value: number, max = 24): number[] {
  return [...history.slice(-(max - 1)), value];
}
