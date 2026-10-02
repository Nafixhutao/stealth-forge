/**
 * Public entry point for the browser setup flow. The implementation lives in
 * ./browser-setup, split by responsibility (shell, primitives, and one file per
 * setup stage). Keep this barrel stable so route and test imports do not churn.
 */
export { BrowserSetupView } from "./browser-setup/browser-setup-view";
export { safeError, submitGitHubManifest } from "./browser-setup/helpers";
export { BrowserSetupField } from "./browser-setup/primitives";
