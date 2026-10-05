export type RunStatus = "running" | "completed" | "failed" | "queued";

export type RunStepState = "done" | "failed" | "running";

/** One step in a run's execution timeline. */
export interface RunStep {
  label: string;
  state: RunStepState;
}

/** An agent run observed from the platform's point of view. */
export interface AgentRun {
  id: string;
  user: string;
  agent: string;
  provider: string;
  model: string;
  tokensIn: string;
  tokensOut: string;
  cost: string;
  duration: string;
  status: RunStatus;
  startedAt: string;
  steps: RunStep[];
  error?: string;
  repository: string;
  traceId: string;
}
