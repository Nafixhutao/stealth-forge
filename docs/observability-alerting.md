# Observability alerts and failure runbook

Stealth exposes bounded-cardinality Prometheus metrics from the API and the
trusted worker. This document is the operator baseline: a minimal alert set, a
suggested dashboard, and the first response for the most common failures. It is
guidance for an operator's own monitoring stack, not a bundled alerting service.

## Scrape targets

| Target | Endpoint | Auth |
| --- | --- | --- |
| API | `http://api:8080/metrics` on the private network | `X-Metrics-Token: $METRICS_TOKEN` |
| Worker | `http://worker:9091/metrics` on the private network | `X-Metrics-Token: $METRICS_TOKEN` |

`/metrics` returns `404` unless `METRICS_TOKEN` is set. Scrape both with the
token from your own monitoring stack. Do not publish either listener to the
host.

## Minimal alert set

Thresholds are starting points; tune them to the installation.

| Alert | Signal | Suggested condition | Why |
| --- | --- | --- | --- |
| API error rate | `stealth_api_http_requests_total{status=~"5.."}` | ratio > 5% for 10m | User-visible failures |
| API latency | `stealth_api_http_request_duration_seconds` | p99 > 2s for 10m | Saturation or slow queries |
| Readiness | `up{job="stealth-api"}` or a `/readyz` probe | down for 2m | Database/storage/Redis loss |
| Database pool | `stealth_db_pool_connections{state="acquired"}` vs `state="max"` | > 90% for 10m | Pool exhaustion |
| Empty acquires | `rate(stealth_db_pool_empty_acquires_total[5m])` | > 0 sustained | Callers waiting on the pool |
| Worker down | `up{job="stealth-worker"}` | down for 2m | Queues stop draining |
| Queue backlog | `stealth_webhook_worker_polls_total` stops increasing | no poll increase for 5m | Stalled loop |
| Delivery failures | `rate(stealth_webhook_worker_jobs_completed_total{result="failed"}[15m])` | sustained > 0 | Provider or payload problem |
| Messaging failures | `rate(stealth_messaging_worker_jobs_completed_total{result="failed"}[15m])` | sustained > 0 | Provider problem |
| App builds | `rate(stealth_apps_worker_builds_completed_total{result="failed"}[30m])` | sustained > 0 | Build plane problem |
| Runtime image cache | `stealth_apps_worker_runtime_image_cache_pressure` | == 1 for 30m | Cache cannot converge |

## Suggested dashboard panels

1. API request rate and 5xx ratio by route template.
2. API p50/p95/p99 latency.
3. Database pool: acquired, idle, max, empty-acquire rate.
4. Per-queue polls and completed `{result}` for function, App build, App
   runtime, webhook, messaging, monitor, notification, and artifact cleanup.
5. Realtime: active SSE connections and slow disconnects.

## Failure runbook

**API 5xx spike.** Check `stealth_api_http_requests_total` by route, then API
logs filtered by `request_id`. If the database pool is near `max`, look for a
slow query or a lock wait; restarting is a last resort.

**`/readyz` failing.** `readyz` checks PostgreSQL, the required storage
implementation, function/site stores, and the rate limiter. Inspect
`docker compose logs api postgres redis`. Redis unavailability fails readiness
by design.

**Worker down or queues not draining.** Check `docker compose ps worker` and
worker logs. A failed worker loop exits so the container supervisor restarts it.
Verify BuildKit and the Docker socket if App builds or the runtime reconciler
stalled.

**Delivery backlog.** Webhook and messaging deliveries retry with bounded
backoff and become terminal after their attempt cap. A sustained failure rate
usually means the provider endpoint is rejecting requests; check the delivery
`last_error` through the Console.

**Telemetry unavailable.** This release keeps no telemetry store. Host resource
metrics are sampled in-process by the API, and App logs are read from Docker
json-file logs through the bounded project-scoped API. There is no separate
collector to restart.

**Migration failure on upgrade.** The migration runner is advisory-locked and
fails loudly. See [Upgrade and rollback](upgrade.md); roll back the application
images rather than editing the schema.
