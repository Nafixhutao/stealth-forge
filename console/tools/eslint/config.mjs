import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTypescript from "eslint-config-next/typescript";
import stealth from "./plugins/stealth.mjs";

export default defineConfig([
  ...nextVitals,
  ...nextTypescript,
  {
    plugins: { stealth },
    rules: {
      // Cache invalidation is a policy, not a per-call decision: the rule
      // keeps `invalidate()` the only way to mark queries stale.
      "stealth/single-cache-invalidation": "error",
    },
  },
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
  {
    // The beUI status-bar chart is vendored verbatim from
    // beui.dev/charts/status-bar. Its tooltip layer positions a floating
    // element with @floating-ui/dom, so it reads refs during render and
    // mutates them from effects by design; the React 19 rules flag that shape.
    // Two mechanical fixes were applied on port (the pointer callback ref was
    // renamed to end in `Ref`, and TooltipPositioner now passes its props by
    // name instead of spreading an object that carries refs). What remains is
    // structural to the vendored positioning code, so the rules are relaxed
    // here rather than rewritten. Revisit if this is ever forked.
    files: [
      "src/components/charts/status-bar.tsx",
      "src/components/charts/status-bar/**/*.{ts,tsx}",
      "src/components/motion/tooltip.tsx",
      "src/components/motion/tooltip/**/*.{ts,tsx}",
      "src/components/motion/tooltip-surface.tsx",
      "src/lib/hooks/use-dismiss.ts",
      "src/lib/hooks/use-hover-gesture.ts",
      "src/lib/hooks/use-tap-gesture.ts",
      "src/lib/touch.ts",
    ],
    rules: {
      "react-hooks/refs": "off",
      "react-hooks/immutability": "off",
      "react-hooks/exhaustive-deps": "off",
      "@typescript-eslint/no-unused-vars": "off",
    },
  },
  globalIgnores([
    ".next/**",
    "src/api/generated/**",
    "playwright-report/**",
    "test-results/**",
  ]),
]);
