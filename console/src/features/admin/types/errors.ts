export type ErrorStatus = "unresolved" | "resolved";

/** One representative occurrence inside an error group. */
export interface ErrorEvent {
  id: string;
  message: string;
  /** Stack trace lines, rendered top to bottom. */
  stack: string[];
  timestamp: string;
  environment: string;
  requestId: string;
  traceId: string;
}

/** A grouped error signature (name + service) tracked over time. */
export interface ErrorGroup {
  id: string;
  name: string;
  service: string;
  events: number;
  users: number;
  status: ErrorStatus;
  lastSeen: string;
  firstSeen: string;
  /** Per-hour event counts for the mini trend. */
  trend: number[];
  message: string;
  sample: ErrorEvent;
}
