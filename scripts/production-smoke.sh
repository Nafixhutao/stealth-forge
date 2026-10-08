#!/usr/bin/env bash
set -Eeuo pipefail

api_url="${API_URL:-http://127.0.0.1:18080}"
console_url="${CONSOLE_URL:-http://127.0.0.1:13000}"
proxy_url="${PROXY_URL:-http://127.0.0.1:${PROXY_HTTP_PORT:-8080}}"
attempts="${SMOKE_ATTEMPTS:-60}"
interval="${SMOKE_INTERVAL_SECONDS:-2}"
compose_file="${COMPOSE_FILE:-}"
env_file="${ENV_FILE:-.env.production}"
compose_services="${SMOKE_COMPOSE_SERVICES:-worker buildkit traefik}"

if ! [[ "$attempts" =~ ^[1-9][0-9]*$ && "$interval" =~ ^[1-9][0-9]*$ ]]; then
	printf 'SMOKE_ATTEMPTS and SMOKE_INTERVAL_SECONDS must be positive integers\n' >&2
	exit 2
fi

wait_for() {
	local name="$1"
	local url="$2"
	local response
	for attempt in $(seq 1 "$attempts"); do
		if response="$(curl --fail --silent --show-error --max-time 5 "$url")"; then
			printf '%s: %s\n' "$name" "$url"
			printf '%s\n' "$response" | head -c 300
			printf '\n'
			return 0
		fi
		if [ "$attempt" -lt "$attempts" ]; then
			sleep "$interval"
		fi
	done
	printf 'smoke check timed out after %s attempts: %s\n' "$attempts" "$url" >&2
	return 1
}

expect_status() {
	local name="$1"
	local url="$2"
	local expected="$3"
	local actual
	actual="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 5 "$url" || true)"
	if [ "$actual" != "$expected" ]; then
		printf '%s: expected HTTP %s, got %s (%s)\n' "$name" "$expected" "$actual" "$url" >&2
		return 1
	fi
	printf '%s: %s (HTTP %s)\n' "$name" "$url" "$actual"
}

wait_for "API liveness" "${api_url%/}/healthz"
wait_for "API readiness" "${api_url%/}/readyz"
wait_for "API build metadata" "${api_url%/}/version"
wait_for "Console" "${console_url%/}/"

if [ -n "$proxy_url" ]; then
	wait_for "Reverse proxy" "${proxy_url%/}/"
	expect_status "Reverse proxy API route" "${proxy_url%/}/v1/account" "401"
fi

# A passing HTTP check does not prove the worker, BuildKit, or Traefik are
# healthy. Assert the Compose health status for those services
# when a Compose file is supplied, so a stalled queue or build plane fails the
# documented post-upgrade smoke instead of reporting a false success.
if [ -n "$compose_file" ] && [ -f "$compose_file" ] && command -v docker >/dev/null 2>&1; then
	compose_args=(docker compose --env-file "$env_file" -f "$compose_file")
	for service in $compose_services; do
		container="$("${compose_args[@]}" ps -q "$service" 2>/dev/null || true)"
		if [ -z "$container" ]; then
			printf 'compose service %s is not running\n' "$service" >&2
			exit 1
		fi
		status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container" 2>/dev/null || true)"
		if [ "$status" != "healthy" ] && [ "$status" != "running" ]; then
			printf 'compose service %s is not healthy: %s\n' "$service" "$status" >&2
			exit 1
		fi
		printf '%s: %s\n' "$service" "$status"
	done
fi

printf 'HTTP production smoke checks passed\n'
