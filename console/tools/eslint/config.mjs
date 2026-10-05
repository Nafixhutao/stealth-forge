import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTypescript from "eslint-config-next/typescript";

export default defineConfig([
  ...nextVitals,
  ...nextTypescript,
  {
    // The redesigned admin console was ported from the standalone
    // stealth-console prototype. Its mock live-update simulations and
    // dialog-reset effects intentionally set state from effects (random
    // nudges must not run during render), which the React 19 rules flag. These
    // files are replaced when each admin surface is wired to the real API.
    files: [
      "src/features/admin/components/**/*.{ts,tsx}",
      "src/features/admin/hooks/**/*.{ts,tsx}",
      "src/features/admin/overview/**/*.{ts,tsx}",
      "src/features/admin/infrastructure/**/*.{ts,tsx}",
      "src/features/admin/workers/**/*.{ts,tsx}",
      "src/features/admin/incidents/**/*.{ts,tsx}",
    ],
    rules: {
      "react-hooks/set-state-in-effect": "off",
    },
  },
  globalIgnores([
    ".next/**",
    "src/api/generated/**",
    "playwright-report/**",
    "test-results/**",
  ]),
]);
