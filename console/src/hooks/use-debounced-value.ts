"use client";

import { useEffect, useState } from "react";

/**
 * Returns a copy of `value` that only changes after `delay` milliseconds pass
 * without another update. Used to keep controlled text inputs responsive while
 * debouncing the expensive work they trigger, such as a network request or a
 * router navigation.
 */
export function useDebouncedValue<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);

  return debounced;
}
