// Fixtures use UTC timestamps while `formatDate` renders in the viewer's local
// zone, so an unpinned timezone makes date assertions pass only on UTC
// machines. Pinning here runs every developer and CI the same way; production
// keeps rendering local time.
process.env.TZ = "UTC";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
