#!/usr/bin/env bash
set -Eeuo pipefail

# This is intentionally separate from the fresh-install Compose smoke. It
# materializes a v0.2.5-era installation layout, runs the current target
# release's install-engine configuration transaction against it, then runs the
# existing full HTTP smoke using that migrated installation root.

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
source_env="${ENV_FILE:-$repo_root/.env.production}"
target_version="${STEALTH_UPGRADE_SMOKE_VERSION:-v0.2.6}"

if ! command -v docker >/dev/null 2>&1; then
	printf '%s\n' 'managed asset upgrade smoke requires Docker' >&2
	exit 2
fi
if [ ! -f "$source_env" ]; then
	printf 'environment file not found: %s\n' "$source_env" >&2
	exit 2
fi

smoke_root="$(mktemp -d "${TMPDIR:-/tmp}/stealth-managed-upgrade.XXXXXX")"
asset_root="$(mktemp -d "${TMPDIR:-/tmp}/stealth-managed-assets.XXXXXX")"
asset_pid=""
cleanup() {
	local status=$?
	if [ -n "$asset_pid" ]; then
		kill "$asset_pid" >/dev/null 2>&1 || true
		wait "$asset_pid" >/dev/null 2>&1 || true
	fi
	rm -rf "$smoke_root" "$asset_root"
	exit "$status"
}
trap cleanup EXIT

mkdir -p "$smoke_root/console/deploy" "$asset_root/$target_version"
while IFS= read -r managed_asset; do
	[ -n "$managed_asset" ] || continue
	mkdir -p "$asset_root/$target_version/$(dirname -- "$managed_asset")"
	cp "$repo_root/$managed_asset" "$asset_root/$target_version/$managed_asset"
done < "$repo_root/release-managed-assets.txt"
(
	cd "$asset_root/$target_version"
	: > checksums.txt
	while IFS= read -r managed_asset; do
		[ -n "$managed_asset" ] || continue
		sha256sum -- "$managed_asset" >> checksums.txt
	done < "$repo_root/release-managed-assets.txt"
)
cp "$source_env" "$smoke_root/config.env"
chmod 600 "$smoke_root/config.env"

config_value() {
	local key="$1"
	sed -n "s/^${key}=//p" "$source_env" | tail -n 1
}

# The fixture removes a managed image key to prove config migration restores
# it. Tag the locally built workflow image with the canonical target name so the
# subsequent Compose smoke remains hermetic and never pulls a release image from
# a registry.
console_image="$(config_value STEALTH_CONSOLE_IMAGE)"
if [ -z "$console_image" ] || ! docker image inspect "$console_image" >/dev/null 2>&1; then
	printf 'required locally-built smoke image is unavailable: %s\n' "$console_image" >&2
	exit 1
fi
docker tag "$console_image" "ghcr.io/nafixhutao/stealth-console:$target_version"

# Model an installation created before managed-asset migration. The
# configuration remains operator state; only release-owned topology files are
# deliberately old.
sed -i \
	-e '/^STEALTH_CONSOLE_IMAGE=/d' \
	"$smoke_root/config.env"
printf '%s\n' 'UPGRADE_SECRET_MARKER=upgrade-secret-must-survive' >>"$smoke_root/config.env"
printf '%s\n' 'v0.2.5' >"$smoke_root/VERSION"
printf '%s\n' \
	'services:' \
	'  api:' \
	'    image: stealth-api:v0.2.5' \
	'networks:' \
	'  stealth:' >"$smoke_root/compose.production.yaml"
printf '%s\n' 'server {' '  # v0.2.5 managed proxy asset' '}' >"$smoke_root/console/deploy/nginx.conf"

python3 -c '
import functools
import http.server
import sys

directory = sys.argv[1]
handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=directory)
server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
print(server.server_port, flush=True)
server.serve_forever()
' "$asset_root" >"$asset_root/server.log" 2>&1 &
asset_pid=$!
for _ in $(seq 1 30); do
	asset_port="$(head -n 1 "$asset_root/server.log" 2>/dev/null || true)"
	if [ -n "${asset_port:-}" ]; then
		break
	fi
	sleep 0.1
done
if [ -z "${asset_port:-}" ]; then
	printf '%s\n' 'local managed-asset server did not start' >&2
	exit 1
fi

export STEALTH_UPGRADE_SMOKE_ROOT="$smoke_root"
export STEALTH_UPGRADE_SMOKE_ASSET_BASE="http://127.0.0.1:$asset_port"
export STEALTH_UPGRADE_SMOKE_VERSION="$target_version"
go test ./internal/installengine -run '^TestManagedAssetUpgradeSmoke$' -count=1

docker compose --env-file "$smoke_root/config.env" -f "$smoke_root/compose.production.yaml" config --quiet
ENV_FILE="$smoke_root/config.env" \
COMPOSE_FILE="$smoke_root/compose.production.yaml" \
SMOKE_REMOVE_VOLUMES=true \
"$repo_root/scripts/compose-production-smoke.sh"
