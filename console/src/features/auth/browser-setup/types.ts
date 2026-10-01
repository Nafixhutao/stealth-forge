import type { useBrowserSetupFlow } from "../browser-setup-flow";

/** The shared setup flow returned by useBrowserSetupFlow, passed to each stage. */
export type SetupFlow = ReturnType<typeof useBrowserSetupFlow>;
