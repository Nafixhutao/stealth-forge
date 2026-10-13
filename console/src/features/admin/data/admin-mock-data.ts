import type {
  MetricPoint,
  PlatformStat,
  ServiceHealth,
  SystemMetric,
  TimeRange,
} from "../types/telemetry";
import type { Host, ResourceSeries, Worker } from "../types/infrastructure";
import type { LogEntry, LogLevel } from "../types/logs";
import type { Trace, TraceSpan } from "../types/traces";
import type { ErrorGroup } from "../types/errors";
import type { AgentRun } from "../types/runs";
import type { Incident } from "../types/incidents";
import type { ModelUsage, Provider } from "../types/platform";

// ---------------------------------------------------------------------------
// Deterministic mock generation. Every series comes from a seeded PRNG so the
// server render and the first client render produce identical data — live
// updates only ever mutate state after hydration.
// ---------------------------------------------------------------------------

function mulberry32(seed: number) {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function round(value: number, decimals = 2) {
  const factor = 10 ** decimals;
  return Math.round(value * factor) / factor;
}

/** Random walk that mean-reverts toward the middle of [min, max]. */
function walk(
  seed: number,
  count: number,
  min: number,
  max: number,
  volatility = 0.16,
): number[] {
  const rng = mulberry32(seed);
  const span = max - min;
  const mid = min + span / 2;
  let value = min + span * (0.35 + rng() * 0.3);
  const out: number[] = [];
  for (let i = 0; i < count; i += 1) {
    value += (rng() - 0.5) * 2 * span * volatility;
    value += (mid - value) * 0.08;
    value = Math.min(max, Math.max(min, value));
    out.push(round(value, 2));
  }
  return out;
}

function toPoints(values: number[], labels: string[]): MetricPoint[] {
  return labels.map((timestamp, index) => ({
    timestamp,
    value: values[index] ?? 0,
  }));
}

/** "12:05"-style axis labels, wrapping past midnight like a rolling window. */
function clockLabels(
  count: number,
  stepMinutes: number,
  startMinutes: number,
): string[] {
  return Array.from({ length: count }, (_, index) => {
    const total = (startMinutes + index * stepMinutes) % (24 * 60);
    const hours = Math.floor(total / 60);
    const minutes = total % 60;
    return `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}`;
  });
}

/** "10:42:18.291"-style log timestamps from milliseconds since midnight. */
export function formatLogClock(msSinceMidnight: number): string {
  const normalized = ((msSinceMidnight % 86400000) + 86400000) % 86400000;
  const hours = Math.floor(normalized / 3600000);
  const minutes = Math.floor((normalized % 3600000) / 60000);
  const seconds = Math.floor((normalized % 60000) / 1000);
  const millis = normalized % 1000;
  return `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}.${String(millis).padStart(3, "0")}`;
}

// ---------------------------------------------------------------------------
// Overview — primary metrics, platform stats, main chart
// ---------------------------------------------------------------------------

export const PRIMARY_METRICS: SystemMetric[] = [
  {
    id: "cpu",
    label: "CPU Usage",
    value: "34%",
    change: "+4.2%",
    changeLabel: "from previous hour",
    tone: "negative",
    history: walk(101, 24, 18, 52),
  },
  {
    id: "memory",
    label: "Memory",
    value: "9.2 GB",
    change: "-1.8%",
    changeLabel: "of 16 GB",
    tone: "positive",
    history: walk(102, 24, 7.4, 11.4),
  },
  {
    id: "storage",
    label: "Storage",
    value: "124 GB",
    change: "+0.4%",
    changeLabel: "of 250 GB",
    tone: "neutral",
    history: walk(103, 24, 118, 126, 0.05),
  },
  {
    id: "network",
    label: "Network",
    value: "24 MB/s",
    hint: "↑ 8 MB/s out",
    change: "+2.1 MB/s",
    changeLabel: "ingress, from previous hour",
    tone: "neutral",
    history: walk(104, 24, 12, 38, 0.3),
  },
];

export const PLATFORM_STATS: PlatformStat[] = [
  { id: "runs", label: "Agent Runs", value: "128", hint: "today" },
  { id: "running", label: "Running", value: "8", tone: "success" },
  { id: "failed", label: "Failed", value: "3", tone: "danger" },
  {
    id: "queue",
    label: "Queue",
    value: "14",
    hint: "waiting",
    tone: "warning",
  },
  {
    id: "workers",
    label: "Workers",
    value: "4 / 4",
    hint: "online",
    tone: "success",
  },
  { id: "sandboxes", label: "Sandboxes", value: "7", hint: "active" },
  { id: "latency", label: "API Latency", value: "182 ms", hint: "p50" },
  { id: "error-rate", label: "Error Rate", value: "0.42%", tone: "warning" },
];

const RANGE_CONFIG: Record<
  Exclude<TimeRange, "7d">,
  { count: number; step: number; start: number }
> = {
  "1h": { count: 13, step: 5, start: 12 * 60 },
  "6h": { count: 13, step: 30, start: 12 * 60 },
  "24h": { count: 12, step: 120, start: 12 * 60 },
};

const DAY_LABELS = [
  "Aug 25",
  "Aug 26",
  "Aug 27",
  "Aug 28",
  "Aug 29",
  "Aug 30",
  "Aug 31",
];

export const RANGE_SEEDS: Record<TimeRange, number> = {
  "1h": 11,
  "6h": 22,
  "24h": 33,
  "7d": 44,
};

/** Labels for the overview main chart for each range. */
export function rangeLabels(range: TimeRange): string[] {
  if (range === "7d") return DAY_LABELS;
  const { count, step, start } = RANGE_CONFIG[range];
  return clockLabels(count, step, start);
}

/** Overview main telemetry chart: one keyed series per tab. */
export function overviewSeries(range: TimeRange): {
  cpu: MetricPoint[];
  memory: MetricPoint[];
  network: MetricPoint[];
} {
  const labels = rangeLabels(range);
  const seed = RANGE_SEEDS[range];
  return {
    cpu: toPoints(walk(seed * 3 + 1, labels.length, 16, 64), labels),
    memory: toPoints(walk(seed * 3 + 2, labels.length, 6.8, 12.6), labels),
    network: toPoints(walk(seed * 3 + 3, labels.length, 10, 44), labels),
  };
}

// ---------------------------------------------------------------------------
// Service health + incidents
// ---------------------------------------------------------------------------

export const SERVICES: ServiceHealth[] = [
  {
    id: "api",
    name: "API",
    status: "healthy",
    latency: "182 ms",
    availability: "99.99%",
    lastCheck: "8s ago",
  },
  {
    id: "database",
    name: "Database",
    status: "healthy",
    latency: "12 ms",
    availability: "100%",
    lastCheck: "8s ago",
  },
  {
    id: "redis",
    name: "Redis",
    status: "healthy",
    latency: "4 ms",
    availability: "100%",
    lastCheck: "8s ago",
  },
  {
    id: "agent-worker",
    name: "Agent Worker",
    status: "healthy",
    latency: "38 ms",
    availability: "4 instances",
    lastCheck: "5s ago",
  },
  {
    id: "sandbox",
    name: "Sandbox Service",
    status: "degraded",
    latency: "412 ms",
    availability: "99.2%",
    lastCheck: "5s ago",
  },
  {
    id: "openai",
    name: "OpenAI",
    status: "healthy",
    latency: "142 ms",
    availability: "99.98%",
    lastCheck: "11s ago",
  },
  {
    id: "anthropic",
    name: "Anthropic",
    status: "healthy",
    latency: "168 ms",
    availability: "99.99%",
    lastCheck: "11s ago",
  },
];

export const INCIDENTS: Incident[] = [
  {
    id: "INC-1042",
    title: "API latency spike",
    severity: "warning",
    services: ["API", "Database"],
    status: "investigating",
    startedAt: "12m ago",
    duration: "Ongoing for 12m",
    updates: [
      {
        time: "12m ago",
        kind: "investigating",
        message:
          "p95 latency elevated to 820 ms on POST endpoints; investigating database connection pool saturation.",
      },
      {
        time: "9m ago",
        kind: "identified",
        message:
          "Slow query on agent_runs identified; EXPLAIN shows a missing index after the last migration.",
      },
      {
        time: "4m ago",
        kind: "monitoring",
        message:
          "Index applied on staging replica; latency recovering. Staying in monitoring until p95 < 400 ms for 10 minutes.",
      },
    ],
  },
  {
    id: "INC-1041",
    title: "Worker timeout increase",
    severity: "critical",
    services: ["Agent Worker"],
    status: "resolved",
    startedAt: "50m ago",
    duration: "Resolved · lasted 18m",
    updates: [
      {
        time: "50m ago",
        kind: "investigating",
        message:
          "Tool execution timeouts climbing on worker-01 (71% CPU, 6 active jobs).",
      },
      {
        time: "40m ago",
        kind: "identified",
        message:
          "Runaway npm install loop in sandbox snapshot cache; caching layer pinned to previous image.",
      },
      {
        time: "32m ago",
        kind: "resolved",
        message:
          "Cache rolled back, timeouts back to baseline. Incident resolved.",
      },
    ],
  },
  {
    id: "INC-1040",
    title: "Sandbox provisioning failures",
    severity: "warning",
    services: ["Sandbox Service"],
    status: "monitoring",
    startedAt: "2h ago",
    duration: "Ongoing for 2h 4m",
    updates: [
      {
        time: "2h ago",
        kind: "investigating",
        message:
          "2% of sandbox provisions fail with image pull timeouts from the registry.",
      },
      {
        time: "1h ago",
        kind: "identified",
        message: "Registry CDN node degraded in sgp-1; traffic rerouted.",
      },
      {
        time: "35m ago",
        kind: "monitoring",
        message: "Failure rate back under 0.2%; monitoring overnight.",
      },
    ],
  },
  {
    id: "INC-1039",
    title: "Database failover drill",
    severity: "info",
    services: ["Database"],
    status: "resolved",
    startedAt: "Yesterday",
    duration: "Resolved · lasted 22m",
    updates: [
      {
        time: "Yesterday",
        kind: "monitoring",
        message:
          "Planned failover drill to the standby region; brief write pauses expected.",
      },
      {
        time: "Yesterday",
        kind: "resolved",
        message: "Drill completed, promotion took 22s. No action required.",
      },
    ],
  },
];

// ---------------------------------------------------------------------------
// Agent runs
// ---------------------------------------------------------------------------

const runSteps = {
  cloneDone: { label: "Repository cloned", state: "done" },
  agentsRead: { label: "AGENTS.md read", state: "done" },
  inspected: { label: "Project inspected", state: "done" },
  edited: { label: "Files edited", state: "done" },
  npmInstall: { label: "npm install", state: "failed" },
  review: { label: "Review posted", state: "done" },
  diffLoaded: { label: "Diff loaded", state: "done" },
  modelRequest: { label: "model.request", state: "failed" },
  testing: { label: "Running tests", state: "running" },
  docs: { label: "Docs updated", state: "done" },
} as const;

export const RUNS: AgentRun[] = [
  {
    id: "run_9AF31",
    user: "Alex",
    agent: "Frontend Engineer",
    provider: "OpenAI",
    model: "GPT-5.6",
    tokensIn: "84k",
    tokensOut: "12k",
    cost: "$1.84",
    duration: "2m 14s",
    status: "failed",
    startedAt: "2m ago",
    repository: "stealth-console",
    traceId: "tr_5QQ44",
    steps: [
      runSteps.cloneDone,
      runSteps.agentsRead,
      runSteps.inspected,
      runSteps.edited,
      runSteps.npmInstall,
    ],
    error: "Command timed out after 120 seconds.",
  },
  {
    id: "run_8BF21",
    user: "Sarah",
    agent: "Code Reviewer",
    provider: "Anthropic",
    model: "Claude Sonnet 4.5",
    tokensIn: "21k",
    tokensOut: "3k",
    cost: "$0.42",
    duration: "48s",
    status: "failed",
    startedAt: "5m ago",
    repository: "stealth-admin-ui",
    traceId: "tr_1KB47",
    steps: [runSteps.cloneDone, runSteps.diffLoaded, runSteps.modelRequest],
    error: "ProviderRateLimitError: Anthropic rate limit exceeded (retry 5/5).",
  },
  {
    id: "run_711AC",
    user: "John",
    agent: "Backend Engineer",
    provider: "OpenAI",
    model: "GPT-5.6",
    tokensIn: "12k",
    tokensOut: "—",
    cost: "$0.08",
    duration: "2m 06s",
    status: "running",
    startedAt: "Now",
    repository: "stealth-docs-site",
    traceId: "tr_6MX18",
    steps: [
      runSteps.cloneDone,
      runSteps.inspected,
      { label: "Files edited", state: "running" },
    ],
  },
  {
    id: "run_6DD08",
    user: "Mia",
    agent: "Docs Writer",
    provider: "Anthropic",
    model: "Claude Haiku 4.5",
    tokensIn: "8k",
    tokensOut: "4k",
    cost: "$0.06",
    duration: "1m 12s",
    status: "completed",
    startedAt: "9m ago",
    repository: "stealth-docs-site",
    traceId: "tr_2WN90",
    steps: [runSteps.cloneDone, runSteps.docs],
  },
  {
    id: "run_5EF74",
    user: "Alex",
    agent: "QA Agent",
    provider: "OpenAI",
    model: "GPT-5.6 mini",
    tokensIn: "31k",
    tokensOut: "6k",
    cost: "$0.31",
    duration: "3m 45s",
    status: "completed",
    startedAt: "12m ago",
    repository: "stealth-console",
    traceId: "tr_4HT77",
    steps: [
      runSteps.cloneDone,
      { label: "Tests generated", state: "done" },
      { label: "Test run passed", state: "done" },
    ],
  },
  {
    id: "run_4CA19",
    user: "Leo",
    agent: "Frontend Engineer",
    provider: "Anthropic",
    model: "Claude Sonnet 4.5",
    tokensIn: "—",
    tokensOut: "—",
    cost: "—",
    duration: "—",
    status: "queued",
    startedAt: "1m ago",
    repository: "stealth-admin-ui",
    traceId: "—",
    steps: [],
  },
  {
    id: "run_3BQ62",
    user: "Sarah",
    agent: "Backend Engineer",
    provider: "OpenAI",
    model: "GPT-5.6",
    tokensIn: "96k",
    tokensOut: "18k",
    cost: "$2.10",
    duration: "4m 51s",
    status: "completed",
    startedAt: "26m ago",
    repository: "stealth-console",
    traceId: "tr_3GD52",
    steps: [
      runSteps.cloneDone,
      runSteps.inspected,
      runSteps.edited,
      { label: "Tests passed", state: "done" },
    ],
  },
  {
    id: "run_2XN37",
    user: "John",
    agent: "Code Reviewer",
    provider: "OpenAI",
    model: "GPT-5.6 mini",
    tokensIn: "17k",
    tokensOut: "2k",
    cost: "$0.14",
    duration: "58s",
    status: "completed",
    startedAt: "41m ago",
    repository: "stealth-admin-ui",
    traceId: "tr_2JD83",
    steps: [runSteps.cloneDone, runSteps.diffLoaded, runSteps.review],
  },
  {
    id: "run_1VK83",
    user: "Mia",
    agent: "Frontend Engineer",
    provider: "OpenAI",
    model: "GPT-5.6",
    tokensIn: "44k",
    tokensOut: "9k",
    cost: "$0.96",
    duration: "2m 27s",
    status: "failed",
    startedAt: "1h ago",
    repository: "stealth-docs-site",
    traceId: "tr_1AF06",
    steps: [
      runSteps.cloneDone,
      { label: "Sandbox provisioned", state: "failed" },
    ],
    error:
      "SandboxProvisionError: sandbox image pull failed (registry timeout).",
  },
];

// ---------------------------------------------------------------------------
// Infrastructure — hosts, fleet resource charts, workers
// ---------------------------------------------------------------------------

export const HOSTS: Host[] = [
  {
    id: "host-01",
    name: "production-01",
    status: "online",
    os: "Ubuntu 24.04",
    ip: "10.0.12.21",
    region: "sgp-1",
    cpu: 42,
    memory: 61,
    storage: 49,
    memoryTotalGb: 16,
    storageTotalGb: 250,
    uptime: "32d 4h",
    workers: 2,
    jobs: 4,
  },
  {
    id: "host-02",
    name: "production-02",
    status: "online",
    os: "Ubuntu 24.04",
    ip: "10.0.12.22",
    region: "sgp-1",
    cpu: 21,
    memory: 38,
    storage: 66,
    memoryTotalGb: 16,
    storageTotalGb: 250,
    uptime: "32d 4h",
    workers: 1,
    jobs: 2,
  },
  {
    id: "host-03",
    name: "worker-01",
    status: "online",
    os: "Ubuntu 24.04",
    ip: "10.0.12.31",
    region: "sgp-1",
    cpu: 71,
    memory: 83,
    storage: 38,
    memoryTotalGb: 16,
    storageTotalGb: 500,
    uptime: "12d 9h",
    workers: 1,
    jobs: 6,
  },
];

const INFRA_LABELS = clockLabels(60, 1, 12 * 60);

export const INFRA_SERIES: ResourceSeries[] = [
  {
    id: "cpu",
    label: "CPU Usage",
    unit: "%",
    tone: "accent",
    current: "45%",
    peak: "78%",
    average: "41%",
    data: toPoints(walk(201, 60, 18, 78), INFRA_LABELS),
  },
  {
    id: "memory",
    label: "Memory Usage",
    unit: "GB",
    tone: "info",
    current: "29.1 GB",
    peak: "34.2 GB",
    average: "27.8 GB",
    data: toPoints(walk(202, 60, 22, 34), INFRA_LABELS),
  },
  {
    id: "disk",
    label: "Disk Usage",
    unit: "%",
    tone: "muted",
    current: "51%",
    peak: "51%",
    average: "50%",
    data: toPoints(walk(203, 60, 49, 52, 0.04), INFRA_LABELS),
  },
  {
    id: "network",
    label: "Network Throughput",
    unit: "MB/s",
    tone: "warning",
    current: "62 MB/s",
    peak: "141 MB/s",
    average: "58 MB/s",
    data: toPoints(walk(204, 60, 24, 141, 0.3), INFRA_LABELS),
  },
];

/** Small per-host CPU history for the host detail strip. */
export const HOST_CPU_HISTORY: Record<string, number[]> = Object.fromEntries(
  HOSTS.map((host, index) => [
    host.id,
    walk(
      210 + index * 7,
      30,
      Math.max(8, host.cpu - 22),
      Math.min(96, host.cpu + 22),
    ),
  ]),
);

export const WORKERS: Worker[] = [
  {
    id: "wk-01",
    name: "worker-01",
    host: "worker-01",
    status: "busy",
    cpu: 71,
    memoryUsed: "6.6 / 8 GB",
    jobs: 6,
    heartbeat: "2s ago",
    uptime: "12d 9h",
    currentRun: "run_711AC",
    queue: 3,
  },
  {
    id: "wk-02",
    name: "worker-02",
    host: "production-01",
    status: "online",
    cpu: 42,
    memoryUsed: "4.8 / 8 GB",
    jobs: 3,
    heartbeat: "3s ago",
    uptime: "32d 4h",
    currentRun: "run_5EF74",
    queue: 1,
  },
  {
    id: "wk-03",
    name: "worker-03",
    host: "production-01",
    status: "online",
    cpu: 17,
    memoryUsed: "2.4 / 8 GB",
    jobs: 1,
    heartbeat: "5s ago",
    uptime: "32d 4h",
    queue: 0,
  },
  {
    id: "wk-04",
    name: "worker-04",
    host: "production-02",
    status: "online",
    cpu: 28,
    memoryUsed: "3.1 / 8 GB",
    jobs: 2,
    heartbeat: "4s ago",
    uptime: "21d 1h",
    currentRun: "run_3BQ62",
    queue: 0,
  },
];

export const WORKER_CPU_HISTORY: Record<string, number[]> = Object.fromEntries(
  WORKERS.map((worker, index) => [
    worker.id,
    walk(
      230 + index * 5,
      30,
      Math.max(6, worker.cpu - 20),
      Math.min(95, worker.cpu + 18),
    ),
  ]),
);

// ---------------------------------------------------------------------------
// Logs
// ---------------------------------------------------------------------------

type LogSeed = [
  /** ms back from the newest log line */
  offsetMs: number,
  level: LogLevel,
  service: string,
  message: string,
  meta?: string,
  extra?: Record<string, string | number>,
];

const LOG_BASE_MS = 10 * 3600000 + 42 * 60000 + 18291;

const LOG_SEEDS: LogSeed[] = [
  [
    0,
    "INFO",
    "api",
    "POST /api/runs",
    "200 · 182ms",
    {
      method: "POST",
      path: "/api/runs",
      status: 200,
      duration_ms: 182,
      user: "Alex",
      run: "run_9AF31",
    },
  ],
  [
    240,
    "INFO",
    "worker",
    "run_9AF31 started",
    undefined,
    { run: "run_9AF31", worker: "worker-02", sandbox: "sbx_9K2F" },
  ],
  [
    640,
    "INFO",
    "sandbox",
    "sandbox sbx_9K2F provisioned in 1.8s",
    "1.8s",
    { sandbox: "sbx_9K2F", image: "agent-node:22", region: "sgp-1" },
  ],
  [
    1420,
    "INFO",
    "api",
    "POST /api/chat",
    "200 · 94ms",
    {
      method: "POST",
      path: "/api/chat",
      status: 200,
      duration_ms: 94,
      user: "Sarah",
    },
  ],
  [
    2210,
    "WARN",
    "sandbox",
    "command execution approaching timeout",
    "94s elapsed",
    { sandbox: "sbx_9K2F", command: "npm install", timeout_s: 120 },
  ],
  [
    3810,
    "ERROR",
    "worker",
    "tool execution timeout",
    "120s",
    {
      run: "run_9AF31",
      tool: "terminal",
      command: "npm install",
      timeout_s: 120,
    },
  ],
  [
    4460,
    "INFO",
    "api",
    "GET /api/projects",
    "200 · 23ms",
    { method: "GET", path: "/api/projects", status: 200, duration_ms: 23 },
  ],
  [
    5320,
    "INFO",
    "database",
    "slow query logged: SELECT * FROM agent_runs WHERE ...",
    "214ms",
    { duration_ms: 214, table: "agent_runs", rows: 1284 },
  ],
  [
    6240,
    "INFO",
    "scheduler",
    "queue depth 14, scaling workers 3 → 4",
    undefined,
    { queue_depth: 14, workers: 4 },
  ],
  [
    7180,
    "WARN",
    "api",
    "provider rate limit backoff engaged",
    "429",
    { provider: "Anthropic", status: 429, retry_after_s: 12 },
  ],
  [
    8460,
    "INFO",
    "gateway",
    "websocket client connected",
    undefined,
    { session: "sess_5D21", ip: "103.22.141.7" },
  ],
  [
    9620,
    "INFO",
    "worker",
    "job claimed: run_8BF21",
    undefined,
    { run: "run_8BF21", worker: "worker-01" },
  ],
  [
    11040,
    "ERROR",
    "worker",
    "ProviderRateLimitError: Anthropic rate limit exceeded (retry 5/5)",
    undefined,
    { run: "run_8BF21", provider: "Anthropic", retries: 5 },
  ],
  [
    12480,
    "INFO",
    "api",
    "POST /api/auth/token",
    "200 · 41ms",
    { method: "POST", path: "/api/auth/token", status: 200, duration_ms: 41 },
  ],
  [
    13940,
    "DEBUG",
    "sandbox",
    "snapshot cache miss, pulling base image",
    undefined,
    { image: "agent-node:22", layer: "6/9" },
  ],
  [
    15230,
    "INFO",
    "database",
    "migration 0042_agent_run_index applied",
    "128ms",
    { migration: "0042_agent_run_index", duration_ms: 128 },
  ],
  [
    16620,
    "WARN",
    "database",
    "connection pool at 87% capacity",
    "87%",
    { pool: "primary", used: 87, max: 100 },
  ],
  [
    17980,
    "INFO",
    "worker",
    "run_6DD08 completed",
    "1m 12s",
    { run: "run_6DD08", duration_s: 72, tokens_out: 4120 },
  ],
  [
    19420,
    "INFO",
    "api",
    "POST /api/projects",
    "201 · 118ms",
    { method: "POST", path: "/api/projects", status: 201, duration_ms: 118 },
  ],
  [
    20840,
    "ERROR",
    "sandbox",
    "snapshot upload failed, retrying (1/3)",
    undefined,
    { sandbox: "sbx_2XK1", attempt: 1, error: "ECONNRESET" },
  ],
  [
    22160,
    "DEBUG",
    "api",
    "cache hit for GET /api/usage",
    "3ms",
    { path: "/api/usage", cache: "hit" },
  ],
  [
    23740,
    "INFO",
    "gateway",
    "websocket client disconnected (idle)",
    undefined,
    { session: "sess_4B02", reason: "idle_timeout" },
  ],
  [
    25380,
    "INFO",
    "worker",
    "heartbeat ok",
    undefined,
    { worker: "worker-03", cpu: 17, jobs: 1 },
  ],
  [
    26920,
    "WARN",
    "api",
    "GET /api/runs/run_8BF21 not found",
    "404 · 12ms",
    { method: "GET", path: "/api/runs/run_8BF21", status: 404 },
  ],
  [
    28460,
    "INFO",
    "scheduler",
    "reaped 3 idle sandboxes after 30m",
    undefined,
    { reaped: 3, idle_min: 30 },
  ],
  [
    30140,
    "ERROR",
    "api",
    "POST /api/runs rejected by validation",
    "422 · 9ms",
    { method: "POST", path: "/api/runs", status: 422, field: "agent_id" },
  ],
  [
    31780,
    "INFO",
    "worker",
    "run_5EF74 completed",
    "3m 45s",
    { run: "run_5EF74", duration_s: 225, tokens_out: 6240 },
  ],
  [
    33420,
    "DEBUG",
    "worker",
    "gc pause 42ms",
    "42ms",
    { worker: "worker-04", pause_ms: 42 },
  ],
  [
    35060,
    "INFO",
    "api",
    "GET /api/usage",
    "200 · 51ms",
    { method: "GET", path: "/api/usage", status: 200, duration_ms: 51 },
  ],
  [
    36780,
    "WARN",
    "sandbox",
    "registry latency elevated in sgp-1",
    "1204ms",
    { registry: "sgp-1", latency_ms: 1204 },
  ],
  [
    38420,
    "INFO",
    "database",
    "checkpoint complete",
    "86ms",
    { duration_ms: 86 },
  ],
  [
    40140,
    "INFO",
    "api",
    "POST /api/runs",
    "202 · 96ms",
    {
      method: "POST",
      path: "/api/runs",
      status: 202,
      duration_ms: 96,
      user: "John",
      run: "run_711AC",
    },
  ],
];

const LOG_ENVIRONMENTS = ["production", "production", "production", "staging"];

function hex(seed: number, length: number): string {
  const localRng = mulberry32(seed);
  let out = "";
  for (let i = 0; i < length; i += 1)
    out += "0123456789ABCDEF"[Math.floor(localRng() * 16)];
  return out;
}

function buildLog(seed: LogSeed, index: number): LogEntry {
  const [offsetMs, level, service, message, meta, extra] = seed;
  return {
    id: `log_${hex(index * 131 + 17, 5)}`,
    timestamp: formatLogClock(LOG_BASE_MS - offsetMs),
    level,
    service,
    environment: LOG_ENVIRONMENTS[index % LOG_ENVIRONMENTS.length],
    message,
    meta,
    requestId: `req_${hex(index * 331 + 7, 6)}`,
    traceId: `tr_${hex(index * 947 + 13, 5)}`,
    user: typeof extra?.user === "string" ? extra.user : undefined,
    agentRun: typeof extra?.run === "string" ? extra.run : undefined,
    attributes: {
      pid: 1200 + (index % 7) * 4,
      host:
        service === "worker" || service === "sandbox"
          ? "worker-01"
          : "production-01",
      ...extra,
    },
  };
}

export const LOGS: LogEntry[] = LOG_SEEDS.map(buildLog);

/** Extra entries the live stream cycles through once enabled (client only). */
const LIVE_LOG_SEEDS: LogSeed[] = [
  [
    0,
    "INFO",
    "api",
    "POST /api/runs",
    "200 · 176ms",
    { method: "POST", path: "/api/runs", status: 200, duration_ms: 176 },
  ],
  [
    0,
    "INFO",
    "worker",
    "heartbeat ok",
    undefined,
    { worker: "worker-01", cpu: 71, jobs: 6 },
  ],
  [0, "INFO", "scheduler", "queue depth 14", undefined, { queue_depth: 14 }],
  [
    0,
    "WARN",
    "sandbox",
    "command execution approaching timeout",
    "88s elapsed",
    { command: "npm test", timeout_s: 120 },
  ],
  [
    0,
    "INFO",
    "gateway",
    "websocket client connected",
    undefined,
    { session: "sess_6E10" },
  ],
  [0, "INFO", "database", "query plan cached", "2ms", { table: "runs" }],
  [
    0,
    "INFO",
    "api",
    "GET /api/health",
    "200 · 3ms",
    { method: "GET", path: "/api/health", status: 200, duration_ms: 3 },
  ],
  [
    0,
    "ERROR",
    "worker",
    "tool execution timeout",
    "120s",
    { tool: "terminal", timeout_s: 120 },
  ],
  [
    0,
    "DEBUG",
    "sandbox",
    "snapshot cache hit",
    undefined,
    { image: "agent-node:22" },
  ],
  [
    0,
    "INFO",
    "worker",
    "job claimed: run_711AC",
    undefined,
    { run: "run_711AC", worker: "worker-01" },
  ],
];

export function buildLiveLog(counter: number, nowMs: number): LogEntry {
  const seed = LIVE_LOG_SEEDS[counter % LIVE_LOG_SEEDS.length];
  const entry = buildLog(seed, 1000 + counter);
  return { ...entry, id: `live_${counter}`, timestamp: formatLogClock(nowMs) };
}

// ---------------------------------------------------------------------------
// Traces
// ---------------------------------------------------------------------------

type SpanSpec = [
  name: string,
  service: string,
  start: number,
  duration: number,
  status?: "error",
];

function buildTrace(
  id: string,
  service: string,
  operation: string,
  duration: number,
  status: Trace["status"],
  timestamp: string,
  specs: SpanSpec[],
): Trace {
  const spans: TraceSpan[] = specs.map(
    ([name, spanService, start, spanDuration, spanStatus], index) => ({
      id: `${id}_span_${index}`,
      name,
      service: spanService,
      start,
      duration: spanDuration,
      status: spanStatus ?? "success",
    }),
  );
  return {
    id,
    service,
    operation,
    duration,
    status,
    timestamp,
    spanList: spans,
  };
}

export const TRACES: Trace[] = [
  buildTrace("tr_8AF21", "api", "POST /api/runs", 482, "success", "2m ago", [
    ["POST /api/runs", "api", 0, 482],
    ["auth.verify", "auth", 0, 12],
    ["db.agent.find", "database", 14, 18],
    ["queue.add", "queue", 40, 7],
    ["worker.start", "worker", 52, 38],
    ["sandbox.provision", "sandbox", 92, 48],
    ["model.response", "openai", 150, 302],
    ["database.update", "database", 456, 22],
  ]),
  buildTrace("tr_7ZK93", "api", "POST /api/chat", 296, "success", "4m ago", [
    ["POST /api/chat", "api", 0, 296],
    ["auth.verify", "auth", 0, 11],
    ["rate_limit.check", "redis", 12, 7],
    ["model.response", "anthropic", 26, 245],
    ["usage.log", "database", 274, 15],
  ]),
  buildTrace("tr_6MX18", "worker", "job.process", 1240, "success", "6m ago", [
    ["job.process", "worker", 0, 1240],
    ["queue.claim", "redis", 0, 18],
    ["sandbox.resume", "sandbox", 22, 138],
    ["tool.git_diff", "sandbox", 180, 240],
    ["tool.read_file", "sandbox", 425, 45],
    ["model.response", "openai", 480, 640],
    ["database.update", "database", 1150, 75],
  ]),
  buildTrace("tr_5QQ44", "api", "POST /api/runs", 821, "error", "7m ago", [
    ["POST /api/runs", "api", 0, 821, "error"],
    ["auth.verify", "auth", 0, 13],
    ["db.agent.find", "database", 15, 16],
    ["queue.add", "queue", 38, 8],
    ["worker.start", "worker", 51, 38],
    ["sandbox.provision", "sandbox", 92, 48],
    ["model.response", "openai", 148, 664, "error"],
  ]),
  buildTrace("tr_4HT77", "sandbox", "command.exec", 8400, "success", "9m ago", [
    ["command.exec", "sandbox", 0, 8400],
    ["shell.spawn", "sandbox", 0, 40],
    ["npm.install", "sandbox", 45, 7855],
    ["fs.write", "sandbox", 7910, 210],
    ["snapshot.upload", "storage", 8150, 240],
  ]),
  buildTrace("tr_3GD52", "api", "GET /api/projects", 34, "success", "11m ago", [
    ["GET /api/projects", "api", 0, 34],
    ["auth.verify", "auth", 0, 9],
    ["db.project.list", "database", 10, 18],
  ]),
  buildTrace("tr_2WN90", "api", "POST /api/agents", 154, "success", "14m ago", [
    ["POST /api/agents", "api", 0, 154],
    ["auth.verify", "auth", 0, 10],
    ["db.agent.insert", "database", 12, 32],
    ["audit.log", "database", 48, 11],
  ]),
  buildTrace("tr_2JD83", "gateway", "ws.upgrade", 18, "success", "16m ago", [
    ["ws.upgrade", "gateway", 0, 18],
    ["auth.verify", "auth", 0, 8],
    ["session.register", "redis", 10, 6],
  ]),
  buildTrace("tr_1KB47", "worker", "job.process", 2140, "error", "21m ago", [
    ["job.process", "worker", 0, 2140, "error"],
    ["queue.claim", "redis", 0, 21],
    ["sandbox.resume", "sandbox", 25, 155],
    ["tool.terminal", "sandbox", 190, 1900, "error"],
  ]),
  buildTrace("tr_1AF06", "api", "GET /api/usage", 51, "success", "24m ago", [
    ["GET /api/usage", "api", 0, 51],
    ["auth.verify", "auth", 0, 9],
    ["db.usage.aggregate", "database", 12, 29],
  ]),
];

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

export const ERROR_GROUPS: ErrorGroup[] = [
  {
    id: "err_tool_timeout",
    name: "ToolExecutionTimeout",
    service: "Agent Worker",
    events: 248,
    users: 18,
    status: "unresolved",
    lastSeen: "2m ago",
    firstSeen: "3d ago",
    trend: walk(401, 24, 2, 26, 0.5),
    message: "Command timed out after 120 seconds.",
    sample: {
      id: "evt_01",
      message: "Command timed out after 120 seconds.",
      stack: [
        "ToolExecutionTimeout: Command timed out after 120 seconds.",
        "    at ToolExecutor.run (worker/tools.ts:214:11)",
        "    at processTicksAndRejections (node:internal/process/task_queues:95:5)",
        "    at AgentLoop.step (worker/loop.ts:88:20)",
        "    at async RunSession.execute (worker/session.ts:142:9)",
      ],
      timestamp: "10:40:21.421",
      environment: "production",
      requestId: "req_8FA12C",
      traceId: "tr_5QQ44",
    },
  },
  {
    id: "err_db_conn",
    name: "DatabaseConnectionError",
    service: "API",
    events: 42,
    users: 9,
    status: "unresolved",
    lastSeen: "18m ago",
    firstSeen: "16h ago",
    trend: walk(402, 24, 0, 8, 0.6),
    message: "Connection terminated due to connection pool exhaustion.",
    sample: {
      id: "evt_02",
      message: "Connection terminated due to connection pool exhaustion.",
      stack: [
        "DatabaseConnectionError: Connection terminated due to connection pool exhaustion.",
        "    at Pool.connect (api/db/pool.ts:63:15)",
        "    at async withTransaction (api/db/transaction.ts:22:12)",
        "    at async POST /api/runs (app/api/runs/route.ts:48:3)",
      ],
      timestamp: "10:24:09.882",
      environment: "production",
      requestId: "req_71BD04",
      traceId: "tr_4HT77",
    },
  },
  {
    id: "err_rate_limit",
    name: "ProviderRateLimitError",
    service: "OpenAI",
    events: 31,
    users: 7,
    status: "unresolved",
    lastSeen: "1h ago",
    firstSeen: "2d ago",
    trend: walk(403, 24, 0, 6, 0.6),
    message: "Anthropic rate limit exceeded (retry 5/5).",
    sample: {
      id: "evt_03",
      message: "Anthropic rate limit exceeded (retry 5/5).",
      stack: [
        "ProviderRateLimitError: Anthropic rate limit exceeded (retry 5/5).",
        "    at ProviderClient.request (worker/providers/anthropic.ts:98:13)",
        "    at async RetryQueue.flush (worker/providers/retry.ts:41:18)",
      ],
      timestamp: "09:41:55.104",
      environment: "production",
      requestId: "req_5C99A1",
      traceId: "tr_1KB47",
    },
  },
  {
    id: "err_sandbox_prov",
    name: "SandboxProvisionError",
    service: "Sandbox Service",
    events: 12,
    users: 4,
    status: "unresolved",
    lastSeen: "3h ago",
    firstSeen: "1d ago",
    trend: walk(404, 24, 0, 4, 0.7),
    message: "Sandbox image pull failed (registry timeout).",
    sample: {
      id: "evt_04",
      message: "Sandbox image pull failed (registry timeout).",
      stack: [
        "SandboxProvisionError: Sandbox image pull failed (registry timeout).",
        "    at SandboxPool.provision (sandbox/pool.ts:120:17)",
        "    at async WorkerPool.dispatch (worker/pool.ts:66:24)",
      ],
      timestamp: "07:58:41.660",
      environment: "production",
      requestId: "req_2AF70B",
      traceId: "tr_1AF06",
    },
  },
  {
    id: "err_ws_disconnect",
    name: "WebSocketDisconnect",
    service: "Gateway",
    events: 96,
    users: 21,
    status: "resolved",
    lastSeen: "1d ago",
    firstSeen: "5d ago",
    trend: walk(405, 24, 0, 3, 0.8),
    message: "Client connection closed before handshake completed.",
    sample: {
      id: "evt_05",
      message: "Client connection closed before handshake completed.",
      stack: [
        "WebSocketDisconnect: Client connection closed before handshake completed.",
        "    at Gateway.onClose (gateway/session.ts:77:9)",
      ],
      timestamp: "Yesterday 22:14:03.512",
      environment: "production",
      requestId: "req_0D33E7",
      traceId: "tr_2JD83",
    },
  },
];

// ---------------------------------------------------------------------------
// Platform — users, providers, usage, status page
// ---------------------------------------------------------------------------

export const PROVIDERS: Provider[] = [
  {
    id: "prov_openai",
    name: "OpenAI",
    status: "healthy",
    latency: "142 ms",
    requestsToday: "48.2k",
    uptime: "99.98%",
    models: ["GPT-5.6", "GPT-5.6 mini"],
  },
  {
    id: "prov_anthropic",
    name: "Anthropic",
    status: "healthy",
    latency: "168 ms",
    requestsToday: "21.4k",
    uptime: "99.99%",
    models: ["Claude Sonnet 4.5", "Claude Haiku 4.5"],
  },
  {
    id: "prov_google",
    name: "Google",
    status: "healthy",
    latency: "203 ms",
    requestsToday: "6.8k",
    uptime: "99.95%",
    models: ["Gemini 3 Pro"],
  },
];

export const MODEL_USAGE: ModelUsage[] = [
  {
    id: "mu_01",
    model: "GPT-5.6",
    provider: "OpenAI",
    requests: "31.4k",
    tokens: "48.6M",
    avgLatency: "182 ms",
    errorRate: "0.31%",
    costToday: "$246.10",
  },
  {
    id: "mu_02",
    model: "GPT-5.6 mini",
    provider: "OpenAI",
    requests: "16.8k",
    tokens: "11.2M",
    avgLatency: "96 ms",
    errorRate: "0.12%",
    costToday: "$38.40",
  },
  {
    id: "mu_03",
    model: "Claude Sonnet 4.5",
    provider: "Anthropic",
    requests: "14.1k",
    tokens: "17.8M",
    avgLatency: "214 ms",
    errorRate: "0.44%",
    costToday: "$94.70",
  },
  {
    id: "mu_04",
    model: "Claude Haiku 4.5",
    provider: "Anthropic",
    requests: "7.3k",
    tokens: "4.1M",
    avgLatency: "88 ms",
    errorRate: "0.09%",
    costToday: "$12.20",
  },
  {
    id: "mu_05",
    model: "Gemini 3 Pro",
    provider: "Google",
    requests: "6.8k",
    tokens: "6.4M",
    avgLatency: "203 ms",
    errorRate: "0.51%",
    costToday: "$21.40",
  },
];

export const USAGE_STATS: PlatformStat[] = [
  { id: "usage-runs", label: "Agent Runs · 7d", value: "1,284" },
  { id: "usage-tokens", label: "Tokens · 7d", value: "84.2M" },
  { id: "usage-sandbox", label: "Sandbox Hours · 7d", value: "612 h" },
  { id: "usage-spend", label: "Spend · 7d", value: "$412.80", tone: "neutral" },
];

export const USAGE_LABELS = [
  "Aug 18",
  "Aug 19",
  "Aug 20",
  "Aug 21",
  "Aug 22",
  "Aug 23",
  "Aug 24",
  "Aug 25",
  "Aug 26",
  "Aug 27",
  "Aug 28",
  "Aug 29",
  "Aug 30",
  "Aug 31",
];

export const USAGE_RUNS_SERIES: MetricPoint[] = toPoints(
  walk(501, 14, 110, 210, 0.22),
  USAGE_LABELS,
).map((point) => ({
  ...point,
  value: Math.round(point.value),
}));

export const USAGE_TOKENS_SERIES: MetricPoint[] = toPoints(
  walk(502, 14, 6.4, 13.8, 0.25),
  USAGE_LABELS,
).map((point) => ({
  ...point,
  value: round(point.value, 1),
}));

/** Per-day uptime percent per service for the status page bars. */
export const STATUS_HISTORY_DAYS = 45;

export const STATUS_SERVICES = SERVICES.map((service, index) => {
  const rngService = mulberry32(600 + index * 13);
  const hadBadDay = index === 4; // Sandbox Service — matches its degraded state
  const history = Array.from({ length: STATUS_HISTORY_DAYS }, (_, day) => {
    const isBadDay = hadBadDay && day >= STATUS_HISTORY_DAYS - 3;
    if (isBadDay) return round(97.4 + rngService() * 1.8, 2);
    if (rngService() > 0.94) return round(99.1 + rngService() * 0.6, 2);
    return 100;
  });
  const uptime = round(
    history.reduce((sum, value) => sum + value, 0) / history.length,
    2,
  );
  return {
    id: service.id,
    name: service.name,
    status: service.status,
    history,
    uptime: `${uptime.toFixed(2)}%`,
  };
});
