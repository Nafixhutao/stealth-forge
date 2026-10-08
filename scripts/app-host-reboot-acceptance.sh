#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="${ENV_FILE:-$repo_root/.env.production}"
compose_file="${COMPOSE_FILE:-$repo_root/compose.production.yaml}"
state_file="${APP_REBOOT_EVIDENCE_PATH:-}"
result_file="${APP_REBOOT_RESULT_PATH:-}"
action="${1:-}"
if [ -z "$action" ]; then printf 'usage: %s prepare|verify|cleanup\n' "$(basename -- "$0")" >&2; exit 2; fi
case "$action" in
	prepare|verify|cleanup) ;;
	*) printf 'unsupported host reboot acceptance action: %s\n' "$action" >&2; exit 2 ;;
esac
if [ ! -f "$env_file" ] || [ -L "$env_file" ] || [ ! -f "$compose_file" ] || [ -L "$compose_file" ]; then
	printf '%s\n' 'host reboot acceptance requires regular Compose and environment files' >&2
	exit 2
fi
if [ "$action" != cleanup ] && [ "$(stat -c '%a' "$env_file")" != 600 ]; then
	printf 'host reboot acceptance environment file must remain mode 0600: %s\n' "$env_file" >&2
	exit 2
fi
compose=(docker compose --env-file "$env_file" -f "$compose_file")

cleanup_acceptance_resources() {
	local app_id="${1:-}" containers container network image labels deployment
	local container_args=(--filter 'label=stealth.managed=true' --filter 'label=stealth.resource_type=app')
	if [ -n "$app_id" ]; then
		container_args+=(--filter "label=stealth.app_id=${app_id}")
	fi
	containers="$(docker ps -aq "${container_args[@]}")"
	if [ -n "$containers" ]; then
		while IFS= read -r container; do
			[ -z "$container" ] || docker rm -f "$container" >/dev/null
		done <<<"$containers"
	fi
	"${compose[@]}" down --volumes --rmi local --remove-orphans
	if [ -n "$app_id" ]; then
		shift
		for deployment in "$@"; do
			if [[ "$deployment" =~ ^[0-9a-f-]{36}$ ]] && docker image inspect "stealth-app/${deployment}:runtime" >/dev/null 2>&1; then
				docker image rm "stealth-app/${deployment}:runtime" >/dev/null
			fi
		done
	else
		while IFS= read -r image; do
			if [[ "$image" =~ ^stealth-app/[0-9a-f-]{36}:runtime$ ]]; then
				docker image rm "$image" >/dev/null
			fi
		done < <(docker image ls --format '{{.Repository}}:{{.Tag}}')
	fi
	network="$(sed -n 's/^APPS_RUNTIME_NETWORK_NAME=//p' "$env_file" | tail -n 1 | tr -d "\"'")"
	network="${network:-stealth_app_runtime}"
	if [[ ! "$network" =~ ^[A-Za-z0-9_.-]{1,128}$ ]]; then
		printf 'invalid App runtime network name in environment file: %s\n' "$network" >&2
		return 1
	fi
	if docker network inspect "$network" >/dev/null 2>&1; then
		labels="$(docker network inspect --format '{{index .Labels "stealth.managed"}}|{{index .Labels "stealth.resource_type"}}' "$network")"
		if [ "$labels" = 'true|app_runtime_network' ]; then
			docker network rm "$network" >/dev/null
		fi
	fi
	[ -z "$state_file" ] || rm -f -- "$state_file"
	[ -z "$result_file" ] || rm -f -- "$result_file"
}

restore_buildkit_for_host_reboot() {
	local container status
	"${compose[@]}" start buildkit >/dev/null
	container="$("${compose[@]}" ps -q buildkit 2>/dev/null || true)"
	if [ -z "$container" ]; then
		printf '%s\n' 'BuildKit container is missing after the App OCI reimport smoke' >&2
		return 1
	fi
	for attempt in $(seq 1 60); do
		status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container" 2>/dev/null || true)"
		if [ "$status" = healthy ]; then
			printf '%s\n' 'BuildKit was restored and healthy before the host reboot baseline'
			return 0
		fi
		sleep 5
	done
	printf 'BuildKit did not become healthy before the host reboot baseline: status=%s\n' "${status:-unknown}" >&2
	return 1
}

case "$action" in
	prepare)
		if [ -z "$state_file" ] || [[ "$state_file" != /* ]]; then
			printf '%s\n' 'APP_REBOOT_EVIDENCE_PATH must be absolute' >&2
			exit 2
		fi
		APP_HOST_REBOOT_ACCEPTANCE=true \
		APP_REBOOT_EVIDENCE_PATH="$state_file" \
		APP_RUNTIME_SOAK_SECONDS="${APP_RUNTIME_SOAK_SECONDS:-0}" \
		SMOKE_REMOVE_VOLUMES=false ENV_FILE="$env_file" COMPOSE_FILE="$compose_file" \
		"$repo_root/scripts/compose-production-smoke.sh"
		restore_buildkit_for_host_reboot
		printf 'Reboot baseline prepared at %s. Reboot the host, then run verify.\n' "$state_file"
		;;
	verify|cleanup)
		if [ "$action" = cleanup ] && { [ -z "$state_file" ] || [ ! -e "$state_file" ]; }; then
			clean_marker="${APP_REBOOT_CLEAN_HOST_MARKER:-}"
			if [ -n "$clean_marker" ] && [ -f "$clean_marker" ] && [ ! -L "$clean_marker" ]; then
				project_name="$(sed -n 's/^COMPOSE_PROJECT_NAME=//p' "$env_file" | tail -n 1 | tr -d "\"'")"
				if [[ ! "$project_name" =~ ^[A-Za-z0-9_-]{1,63}$ ]] || [ "$(cat "$clean_marker")" != "clean:${project_name}" ]; then
					printf '%s\n' 'clean-host marker does not match this Compose project; refusing broad App cleanup' >&2
					exit 1
				fi
				cleanup_acceptance_resources
				printf '%s\n' 'removed the isolated Compose acceptance project and managed App runtime resources after a clean-host preflight'
			else
				"${compose[@]}" down --volumes --rmi local --remove-orphans
				[ -z "$state_file" ] || rm -f -- "$state_file"
				[ -z "$result_file" ] || rm -f -- "$result_file"
				printf '%s\n' 'removed only the isolated Compose project; no clean-host marker authorized App runtime cleanup'
			fi
			exit 0
		fi
		if [ -z "$state_file" ] || [ ! -f "$state_file" ] || [ -L "$state_file" ] || [ "$(stat -c '%a' "$state_file")" != 600 ]; then
			printf 'host reboot state must be a regular 0600 file: %s\n' "$state_file" >&2
			exit 2
		fi
		fields_file="$(mktemp "${TMPDIR:-/tmp}/stealth-app-reboot-state.XXXXXX")"
		chmod 0600 "$fields_file"
		python3 - "$state_file" >"$fields_file" <<'PY'
import json
import re
import sys
from urllib.parse import urlparse

with open(sys.argv[1], encoding="utf-8") as source:
    state = json.load(source)
keys = ("boot_id", "source_sha", "api_url", "email", "password", "project_id", "app_id", "app_host", "deployment_id", "generation", "workload_spec_sha256", "marker", "stdout_log_id", "stderr_log_id", "stdout_log_count", "stderr_log_count", "prior_deployment_id")
if any(key not in state for key in keys): raise SystemExit("host reboot state is missing fields")
for key in ("boot_id", "project_id", "app_id", "deployment_id", "prior_deployment_id"):
    if not isinstance(state[key], str) or not re.fullmatch(r"[0-9a-f-]{36}", state[key]): raise SystemExit(f"invalid state field: {key}")
parsed = urlparse(str(state["api_url"]))
if parsed.scheme != "http" or parsed.hostname not in {"127.0.0.1", "localhost"} or not parsed.port: raise SystemExit("API URL must use loopback and an explicit port")
if not re.fullmatch(r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+", str(state["email"])): raise SystemExit("invalid smoke account email")
if not isinstance(state["password"], str) or not state["password"] or "\n" in state["password"]: raise SystemExit("invalid smoke account password")
if not re.fullmatch(r"[A-Za-z0-9.-]+", str(state["app_host"])): raise SystemExit("invalid App hostname")
if not isinstance(state["generation"], int) or state["generation"] < 1: raise SystemExit("invalid App generation")
if not re.fullmatch(r"[0-9a-f]{64}", str(state["workload_spec_sha256"])): raise SystemExit("invalid WorkloadSpec checksum")
if not re.fullmatch(r"[A-Za-z0-9._-]{1,200}", str(state["marker"])): raise SystemExit("invalid App marker")
for key in keys:
    value = state[key]
    if isinstance(value, bool): raise SystemExit(f"invalid state field: {key}")
    print(str(value))
PY
		mapfile -t state <"$fields_file"
		rm -f -- "$fields_file"
		if [ "${#state[@]}" -ne 17 ]; then printf 'invalid host reboot state fields: %s\n' "${#state[@]}" >&2; exit 1; fi
		boot_before="${state[0]}"; source_sha="${state[1]}"; api_url="${state[2]}"; email="${state[3]}"; password="${state[4]}"
		project_id="${state[5]}"; app_id="${state[6]}"; app_host="${state[7]}"; deployment_id="${state[8]}"; generation="${state[9]}"
		spec_sha="${state[10]}"; marker="${state[11]}"; stdout_id="${state[12]}"; stderr_id="${state[13]}"
		stdout_count="${state[14]}"; stderr_count="${state[15]}"; prior_deployment_id="${state[16]}"
		if [ "$action" = verify ] && [ -n "${COMMIT_SHA:-}" ] && [ "$source_sha" != "$COMMIT_SHA" ]; then
			printf 'state source %s does not match expected head %s\n' "$source_sha" "$COMMIT_SHA" >&2
			exit 1
		fi
		if [ "$action" = cleanup ]; then
			cleanup_acceptance_resources "$app_id" "$deployment_id" "$prior_deployment_id"
			printf '%s\n' 'removed this Compose acceptance project and its labeled App runtime resources'
			exit 0
		fi
		boot_after="$(cat /proc/sys/kernel/random/boot_id)"
		if [ "$boot_after" = "$boot_before" ]; then printf '%s\n' 'kernel boot ID is unchanged; real reboot was not observed' >&2; exit 1; fi
		printf '%s\n' 'Kernel boot ID changed; checking production service recovery.'
		for service in postgres redis api worker buildkit console proxy traefik; do
			printf 'Waiting for service recovery: %s\n' "$service"
			container="$("${compose[@]}" ps -q "$service" 2>/dev/null || true)"; healthy=false
			for attempt in $(seq 1 120); do
				if [ -n "$container" ] && [ "$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container" 2>/dev/null || true)" = healthy ]; then healthy=true; break; fi
				sleep 5
			done
			if [ "$healthy" != true ]; then printf 'service did not recover after reboot: %s\n' "$service" >&2; exit 1; fi
		done
		ready="$(curl --silent --show-error --max-time 10 --output /dev/null --write-out '%{http_code}' "${api_url%/}/readyz")"
		if [ "$ready" != 200 ]; then printf 'API readiness returned HTTP %s after reboot\n' "$ready" >&2; exit 1; fi
		printf '%s\n' 'API readiness passed; authenticating the acceptance account.'
		cookie_file="$(mktemp "${TMPDIR:-/tmp}/stealth-app-reboot-cookie.XXXXXX")"; response_file="$(mktemp "${TMPDIR:-/tmp}/stealth-app-reboot-response.XXXXXX")"
		login_payload="$(mktemp "${TMPDIR:-/tmp}/stealth-app-reboot-login.XXXXXX")"; chmod 0600 "$cookie_file" "$response_file" "$login_payload"
		trap 'rm -f -- "$cookie_file" "$response_file" "$login_payload"' EXIT
		python3 - "$login_payload" "$email" "$password" <<'PY'
import json
import sys
with open(sys.argv[1], "w", encoding="utf-8") as output: json.dump({"email": sys.argv[2], "password": sys.argv[3]}, output)
PY
		login_status="$(curl --silent --show-error --max-time 15 --cookie-jar "$cookie_file" --header 'Content-Type: application/json' --data-binary "@$login_payload" --output "$response_file" --write-out '%{http_code}' "${api_url%/}/v1/sessions/email-password")"
		if [ "$login_status" != 204 ]; then printf 'smoke account login returned HTTP %s\n' "$login_status" >&2; exit 1; fi
		cookie="$(awk 'BEGIN { s = "" } { d=$1; if (d ~ /^#HttpOnly_/) sub(/^#HttpOnly_/,"",d); else if (d ~ /^#/) next; if (NF>=7) { printf "%s%s=%s",s,$6,$7; s="; " } }' "$cookie_file")"
		app_url="${api_url%/}/v1/projects/${project_id}/apps/${app_id}"
		diagnostics_url="${app_url}/diagnostics"
		boot_epoch="$(date -d "$(uptime -s)" +%s)"
		printf '%s\n' 'Waiting for the App desired generation, fresh health, and route state to recover.'
		for attempt in $(seq 1 180); do
			app_status="$(curl --silent --show-error --max-time 10 --header "Cookie: $cookie" --output "$response_file" --write-out '%{http_code}' "$app_url" 2>/dev/null || true)"
			app_projection_ready=0
			if [ "$app_status" = 200 ] && python3 - "$response_file" "$deployment_id" "$generation" "$spec_sha" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as source: app=json.load(source).get("app", {})
if app.get("runtime_status") != "running" or app.get("desired_generation") != int(sys.argv[3]) or app.get("observed_generation") != int(sys.argv[3]) or app.get("desired_deployment_id") != sys.argv[2] or app.get("workload_spec_sha256") != sys.argv[4] or app.get("health_status") != "healthy" or app.get("route_status") != "active": raise SystemExit(1)
PY
			then app_projection_ready=1; fi
			diagnostics_status=''
			diagnostics_ready=0
			if [ "$app_projection_ready" -eq 1 ]; then
				diagnostics_status="$(curl --silent --show-error --max-time 10 --header "Cookie: $cookie" --output "$response_file" --write-out '%{http_code}' "$diagnostics_url" 2>/dev/null || true)"
				if [ "$diagnostics_status" = 200 ] && python3 - "$response_file" "$deployment_id" "$generation" "$boot_epoch" <<'PY'
import json
import sys
from datetime import datetime
with open(sys.argv[1], encoding="utf-8") as source: diagnostics=json.load(source)
desired=diagnostics.get("desired_deployment") or {}
applied=diagnostics.get("applied_deployment") or {}
checked_at=diagnostics.get("health_checked_at")
if not checked_at: raise SystemExit(1)
checked_epoch=datetime.fromisoformat(checked_at.replace("Z", "+00:00")).timestamp()
if diagnostics.get("convergence_status") != "converged" or diagnostics.get("desired_generation") != int(sys.argv[3]) or diagnostics.get("observed_generation") != int(sys.argv[3]) or diagnostics.get("applied_generation") != int(sys.argv[3]) or desired.get("id") != sys.argv[2] or applied.get("id") != sys.argv[2] or diagnostics.get("runtime_status") != "running" or diagnostics.get("health_status") != "healthy" or diagnostics.get("route_status") != "active" or checked_epoch < int(sys.argv[4]): raise SystemExit(1)
PY
				then diagnostics_ready=1; fi
			fi
			containers="$(docker ps -aq --filter "label=stealth.app_id=${app_id}" --filter 'label=stealth.resource_type=app' 2>/dev/null || true)"
			container_count="$(printf '%s\n' "$containers" | sed '/^$/d' | wc -l | tr -d ' ')"
			runtime_ready=0
			runtime_container=''
			container_started=''
			container_running=''
			if [ "$container_count" = 1 ]; then
				runtime_container="$containers"
				container_running="$(docker inspect --format '{{.State.Running}}' "$runtime_container" 2>/dev/null || true)"
				if [ "$container_running" = true ]; then
					container_started="$(docker inspect --format '{{.State.StartedAt}}' "$runtime_container" 2>/dev/null || true)"
					if [ -n "$container_started" ] && python3 - "$container_started" "$boot_epoch" <<'PY'
from datetime import datetime
import sys
if datetime.fromisoformat(sys.argv[1].replace("Z", "+00:00")).timestamp() < int(sys.argv[2]): raise SystemExit(1)
PY
					then runtime_ready=1; fi
				fi
			fi
			if [ "$app_projection_ready" -eq 1 ] && [ "$diagnostics_ready" -eq 1 ] && [ "$runtime_ready" -eq 1 ]; then break; fi
			if [ "$attempt" = 180 ]; then
				printf 'App failed to converge after reboot: app_http=%s diagnostics_http=%s managed_containers=%s running=%s started_after_boot=%s\n' \
					"${app_status:-unavailable}" "${diagnostics_status:-unavailable}" "$container_count" \
					"${container_running:-unknown}" "$runtime_ready" >&2
				exit 1
			fi
			sleep 5
		done
		if ! docker exec "$runtime_container" /buildkit-secret-probe verify-runtime-secret-v2 >/dev/null 2>&1; then
			printf '%s\n' 'App runtime secret verification failed after reboot' >&2
			exit 1
		fi
		printf '%s\n' 'App container and current secret passed; checking routed App responses.'
		for check in configuration healthz version; do
			case "$check" in
				configuration) expected_body=app-config-v2 ;;
				healthz) expected_body=app-runtime-smoke-ok ;;
				version) expected_body="$marker" ;;
			esac
			printf 'Waiting for App route after reboot: %s\n' "$check"
			body=''
			for attempt in $(seq 1 60); do
				if body="$("${compose[@]}" exec -T api sh -ec 'wget -qO- --timeout=8 --header "Host: $1" "http://traefik:8080/$2"' sh "$app_host" "$check")" && [ "$body" = "$expected_body" ]; then
					break
				fi
				if [ "$attempt" -eq 60 ]; then
					route_response="$("${compose[@]}" exec -T api sh -ec 'wget -S -O /dev/null --timeout=8 --header "Host: $1" "http://traefik:8080/$2"' sh "$app_host" "$check" 2>&1 || true)"
					route_status="$(printf '%s\n' "$route_response" | sed -n 's/.*HTTP\/[^ ]* \([0-9][0-9][0-9]\).*/\1/p' | tail -n 1)"
					printf 'App route failed to recover after reboot: path=%s HTTP=%s expected=%q body_bytes=%s\n' "$check" "${route_status:-unavailable}" "$expected_body" "${#body}" >&2
					exit 1
				fi
				if [ "$attempt" -eq 1 ] || [ "$((attempt % 10))" -eq 0 ]; then
					printf 'App route is not ready after reboot: path=%s attempt=%s/60\n' "$check" "$attempt"
				fi
				sleep 2
			done
		done
		printf '%s\n' 'App route checks passed; verifying persisted artifact and retained logs.'
		artifact_row="$("${compose[@]}" exec -T postgres sh -ec 'd="$1"; a="$2"; case "$d$a" in *[!0-9a-f-]*) exit 2;; esac; psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set ON_ERROR_STOP=1 --tuples-only --no-align --field-separator="|" --command "SELECT image_path,image_archive_sha256,image_digest,image_size_bytes FROM app_deployments WHERE id = '\''$d'\'' AND app_id = '\''$a'\''"' sh "$deployment_id" "$app_id" | tr -d '\r')"
		IFS='|' read -r image_path archive_sha digest image_size <<<"$artifact_row"
		if ! [[ "$image_path" =~ ^[0-9a-f-]{36}/[0-9a-f-]{36}/[0-9a-f-]{36}$ && "$archive_sha" =~ ^[0-9a-f]{64}$ && "$digest" =~ ^sha256:[0-9a-f]{64}$ && "$image_size" =~ ^[1-9][0-9]*$ ]]; then printf '%s\n' 'invalid persisted App artifact metadata after reboot' >&2; exit 1; fi
		actual_sha="$("${compose[@]}" exec -T worker sh -ec 'p="$1"; s="$2"; f="/var/lib/stealth/storage/app-images/$p"; test -s "$f"; a="$(sha256sum "$f" | cut -d " " -f 1)"; test "$a" = "$s"; printf "%s" "$a"' sh "$image_path" "$archive_sha")"
		if [ "$actual_sha" != "$archive_sha" ]; then printf '%s\n' 'OCI archive checksum changed across host reboot' >&2; exit 1; fi
		logs_url="${api_url%/}/v1/projects/${project_id}/apps/${app_id}/logs"
		for attempt in $(seq 1 90); do
			logs_status="$(curl --silent --show-error --max-time 15 --header "Cookie: $cookie" --get --data-urlencode 'limit=250' --output "$response_file" --write-out '%{http_code}' "$logs_url" 2>/dev/null || true)"
			if [ "$logs_status" = 200 ] && python3 - "$response_file" "$marker" "${state[12]}" "${state[13]}" "${state[14]}" "${state[15]}" <<'PY'
import json
import sys
path, marker, old_out, old_err, count_out, count_err = sys.argv[1:]
with open(path, encoding="utf-8") as source: logs=json.load(source).get("logs", [])
prefix={"out": f"STEALTH_APP_RUNTIME_LOG_STDOUT_{marker}_", "err": f"STEALTH_APP_RUNTIME_LOG_STDERR_{marker}_"}
found={key:set() for key in prefix}
for entry in logs:
    message=str(entry.get("message", ""))
    for key, value in prefix.items():
        if message.startswith(value): found[key].add(str(entry.get("id", "")))
if old_out not in found["out"] or old_err not in found["err"] or len(found["out"]) <= int(count_out) or len(found["err"]) <= int(count_err) or len(logs)>250: raise SystemExit(1)
PY
			then break; fi
			if [ "$attempt" = 90 ]; then printf 'runtime logs failed retained-history/post-reboot checks: HTTP %s\n' "$logs_status" >&2; exit 1; fi
			sleep 2
		done
		if grep -Fq fake-smoke-secret-not-real-v1 "$response_file" || grep -Fq fake-smoke-secret-not-real-v2 "$response_file"; then printf '%s\n' 'runtime logs exposed an environment secret' >&2; exit 1; fi
		if grep -Fq -- "$runtime_container" "$response_file"; then printf '%s\n' 'runtime logs exposed a Docker container ID' >&2; exit 1; fi
		diagnostics_status="$(curl --silent --show-error --max-time 15 --header "Cookie: $cookie" --output "$response_file" --write-out '%{http_code}' "${api_url%/}/v1/projects/${project_id}/apps/${app_id}/diagnostics")"
		if [ "$diagnostics_status" != 200 ]; then printf 'App diagnostics returned HTTP %s after reboot\n' "$diagnostics_status" >&2; exit 1; fi
		python3 - "$response_file" "$deployment_id" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as source: d=json.load(source)
if d.get("convergence_status")!="converged" or (d.get("desired_deployment") or {}).get("id")!=sys.argv[2] or (d.get("applied_deployment") or {}).get("id")!=sys.argv[2]: raise SystemExit("App diagnostics did not converge after reboot")
forbidden={"container_id","container_name","image_id","image_path","source_path","value_ciphertext","ciphertext","nonce","APPS_SECRET_KEY"}
def walk(x):
    if isinstance(x,dict):
        if forbidden.intersection(x): raise SystemExit("App diagnostics exposed forbidden fields")
        for child in x.values(): walk(child)
    elif isinstance(x,list):
        for child in x: walk(child)
walk(d)
PY
		if [ -z "$result_file" ] || [[ "$result_file" != /* ]] || [ -e "$result_file" ] || [ -L "$result_file" ]; then printf 'APP_REBOOT_RESULT_PATH must be an unused absolute path: %s\n' "$result_file" >&2; exit 2; fi
		mkdir -p -- "$(dirname -- "$result_file")"
		host_kind="${APP_REBOOT_HOST_KIND:-external-ssh-vm}"
		case "$host_kind" in external-ssh-vm|github-actions-kvm-vm) ;; *) printf 'invalid App reboot host kind: %s\n' "$host_kind" >&2; exit 2 ;; esac
		python3 - "$result_file" "$source_sha" "$boot_before" "$boot_after" "$app_id" "$deployment_id" "$generation" "$container_started" "$archive_sha" "$actual_sha" "$host_kind" <<'PY'
import json, os, sys
from datetime import datetime, timezone
path, source_sha, old, new, app, deployment, generation, container_started, expected, actual, host_kind = sys.argv[1:]
data={"status":"passed","source_sha":source_sha,"host_kind":host_kind,"previous_boot_id":old,"current_boot_id":new,"app_id":app,"deployment_id":deployment,"desired_generation":int(generation),"managed_app_container_count":1,"app_container_started_at":container_started,"environment_file_mode":"0600","artifact_archive_sha256":expected,"artifact_checksum_after_reboot":actual,"verified_at":datetime.now(timezone.utc).isoformat()}
fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
with os.fdopen(fd,"w",encoding="utf-8") as out: json.dump(data,out,sort_keys=True); out.write("\n")
PY
		printf 'App host reboot acceptance passed: source=%s boot=%s->%s generation=%s managed_containers=1 environment_file_mode=0600\n' "$source_sha" "$boot_before" "$boot_after" "$generation"
		;;
	*) printf 'unsupported host reboot action: %s\n' "$action" >&2; exit 2 ;;
esac
