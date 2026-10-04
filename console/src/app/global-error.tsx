"use client";

import { useEffect } from "react";

// The root boundary for the whole app. It replaces the root layout, so it must
// render its own <html> and <body>. Styles are inlined because the global
// stylesheet is loaded by the root layout this component replaces.
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error(error.digest ?? error);
  }, [error]);

  return (
    <html lang="en">
      <body
        style={{
          margin: 0,
          minHeight: "100vh",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "#0f1011",
          color: "#e8e8ea",
          fontFamily:
            "ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif",
        }}
      >
        <main
          role="alert"
          style={{
            maxWidth: "32rem",
            padding: "2rem",
            border: "1px solid rgba(235,87,87,0.25)",
            borderRadius: "0.75rem",
            background: "rgba(235,87,87,0.05)",
          }}
        >
          <h1 style={{ margin: 0, fontSize: "1.125rem", fontWeight: 600 }}>
            The console could not start
          </h1>
          <p style={{ marginTop: "0.75rem", fontSize: "0.875rem" }}>
            An unexpected error prevented the console from rendering. Reload to
            try again, and include the reference below if you report the issue.
          </p>
          {error.digest ? (
            <p
              style={{
                marginTop: "0.5rem",
                fontSize: "0.75rem",
                opacity: 0.7,
                fontFamily: "ui-monospace, monospace",
              }}
            >
              Reference: {error.digest}
            </p>
          ) : null}
          <button
            type="button"
            onClick={reset}
            style={{
              marginTop: "1.25rem",
              padding: "0.5rem 1rem",
              borderRadius: "0.5rem",
              border: "1px solid rgba(255,255,255,0.2)",
              background: "transparent",
              color: "inherit",
              cursor: "pointer",
              fontSize: "0.875rem",
            }}
          >
            Reload
          </button>
        </main>
      </body>
    </html>
  );
}
