"use client";

import { useEffect } from "react";
import { ErrorState } from "@/components/feedback/error-state";

// Route-level boundary: catches errors below app/layout.tsx. The root layout
// and Providers have their own boundary in global-error.tsx.
export default function RouteError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // error.digest correlates a client error with the server log entry.
    console.error(error.digest ?? error);
  }, [error]);

  return (
    <main className="mx-auto flex min-h-screen max-w-2xl items-center bg-void px-6">
      <ErrorState
        error={error}
        retry={reset}
        title="Something interrupted this view"
      />
    </main>
  );
}
