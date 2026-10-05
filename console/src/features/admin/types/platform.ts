export type UserStatus = "active" | "idle" | "suspended";

/** A platform user, as seen from the admin user table. */
export interface AdminUser {
  id: string;
  name: string;
  email: string;
  role: "Owner" | "Admin" | "Member";
  runs: number;
  lastActive: string;
  status: UserStatus;
}

export type ProviderStatus = "healthy" | "degraded" | "down";

/** A model provider endpoint (OpenAI, Anthropic, ...). */
export interface Provider {
  id: string;
  name: string;
  status: ProviderStatus;
  latency: string;
  requestsToday: string;
  uptime: string;
  models: string[];
}

/** Per-model usage row for the usage/providers tables. */
export interface ModelUsage {
  id: string;
  model: string;
  provider: string;
  requests: string;
  tokens: string;
  avgLatency: string;
  errorRate: string;
  costToday: string;
}
