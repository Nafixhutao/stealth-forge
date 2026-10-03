# Upgrade and rollback

Stealth releases are coordinated application releases. Use the same version
for `stealth-api`, `stealth-worker`, `stealth-ingress-control`,
`stealth-migrate`, `stealth-console`,
`stealth-otel-collector`, `stealth-otel-docker-logs`, and
`stealth-telemetry-docker-proxy`. The host-metrics and Docker-metrics
collectors use the capability-free `stealth-otel-collector` image; the
Docker-log collector uses the dedicated `stealth-otel-docker-logs` image.
The `stealth-setup` image is only needed for a fresh browser installation or
setup repair.
The current deployment does not promise rolling upgrades between incompatible
API/worker/schema versions.

## Go module identity

The canonical Go module path is now `github.com/Nafixhutao/stealth-forge`.
Consumers of the previous `github.com/nazxf/stealth-api` module must update
their imports and `go.mod` requirements; Go does not automatically treat the
two paths as the same module. No compatibility `replace` directive or
forwarding packages are provided.

## Upgrade procedure

Use the installed host CLI for a coordinated update:

```bash
stealth update
```

For releases containing the coordinated updater, the running CLI downloads,
checksum-verifies, extracts, and version-verifies the target binary first. It
then invokes that verified target binary in a narrow internal host-only
migration mode. The target binary owns the target release's managed-asset
manifest, acquires `install.lock`, validates the staged Compose file against
the staged `config.env`, then activates the coordinated platform files. It
revalidates the active Compose configuration before pulling target images,
runs the existing database/telemetry state initialization and migrations,
recreates production services in a deterministic order, and performs the
normal health checks. Only after that succeeds does the original CLI atomically
replace its own executable. On a host without an installation it remains a
CLI-only self-update.

### Transition from v0.2.5

The currently published stable CLI, `v0.2.5`, predates the target-binary
handoff. Its updater can verify and replace a CLI archive, but cannot execute
the release-managed platform migration code that did not exist when it was
published. This is an unavoidable bootstrap boundary, not evidence that an
old stack has been upgraded.

The first release carrying this updater is the **bridge release**. When moving
from `v0.2.5` while that bridge is the latest stable release:

1. Run `stealth update` once. v0.2.5 replaces only the CLI with the bridge
   binary; the running production topology remains unchanged.
2. Run `stealth update` again (or `stealth install --repair`). The bridge
   binary detects the existing installation and performs the documented
   managed-asset migration.

If a later release is already latest, install the bridge executable itself
before running `stealth update`; do not run a fresh-install bootstrap against
an existing root. Use the official bridge archive and its checksum file, for
example:

```bash
release=v0.2.6                 # the bridge named by that release's notes
asset=stealth_Linux_x86_64.tar.gz
workdir="$(mktemp -d)"
curl -fsSLo "$workdir/$asset" "https://github.com/Nafixhutao/stealth-forge/releases/download/$release/$asset"
curl -fsSLo "$workdir/checksums.txt" "https://github.com/Nafixhutao/stealth-forge/releases/download/$release/checksums.txt"
(cd "$workdir" && grep "  $asset$" checksums.txt | sha256sum -c -)
tar -xzf "$workdir/$asset" -C "$workdir"
install -m 0755 "$workdir/stealth" "$(readlink -f "$(command -v stealth)")"
rm -rf "$workdir"
stealth update
```

Use the arm64 archive on arm64 hosts. This binary-only replacement preserves
the existing installation root; the following `stealth update` performs the
coordinated migration. Do not assume a single invocation of the already
shipped v0.2.5 binary can migrate future platform assets. Release notes name
the bridge tag while this transition remains necessary.

`stealth install --repair` performs the same managed-asset preparation for the
currently installed release and is the supported recovery path for a missing
or damaged runtime asset.

The host CLI treats these repository-controlled files as release-managed:

- `compose.production.yaml` and `compose.setup.yaml` when setup assets are present;
- `buildkit/buildkitd.toml` and the managed BuildKit AppArmor profile;
- `traefik/traefik.yaml`, `traefik/dynamic/core.yaml`, and the managed route placeholder;
- `telemetry/otel-collector.yaml`, `telemetry/host-metrics.yaml`,
  `telemetry/docker-logs.yaml`, and `telemetry/docker-stats.yaml`;
- `console/deploy/nginx.conf`.

The complete target set is downloaded and validated before activation. Existing
managed files are replaced atomically, with one bounded previous-release set
under `state/managed-assets.previous` for recovery/debugging. Unknown files in
the installation root and telemetry directory are preserved. Direct edits to
release-managed files are not an override mechanism and may be replaced by a
supported update or repair.

`config.env` is operator state. Its existing values, including generated
secrets and custom image references, are preserved; only missing release keys
are added, and canonical Stealth image references advance with the installed
release. Persistent database, object-storage, and Collector state volumes are
not deleted or recreated by this migration. External PostgreSQL/Redis settings
remain external and are not replaced with bundled services.

App runtime image-cache controls are Compose settings, not database state:
`APPS_RUNTIME_IMAGE_CACHE_MAX_BYTES` (20 GiB default),
`APPS_RUNTIME_IMAGE_CACHE_TARGET_BYTES` (16 GiB default), and
`APPS_RUNTIME_IMAGE_GC_INTERVAL` (15 minute default). An existing valid value
is preserved; a missing setting uses the production Compose default. These
settings add no database migration and do not change persisted App OCI
artifacts or quota.

The automatic update path accepts stable releases only and verifies both the
release archive checksum and the extracted CLI version before target migration.
The installed `VERSION` must agree with the migration plan; an older target is
refused as a platform downgrade. `stealth doctor` reports a CLI/platform
release mismatch and recommends `stealth update` when the CLI was left behind.

For a version-changing upgrade, before migrations run the installer snapshots
the sorted `schema_migrations` ledger using the target release's migration
image and stores its SHA-256 fingerprint beside checksummed previous managed
assets. Compose configuration is first validated from the staged target
Compose file and private staged `config.env`, before release-managed files are
activated.
After activation, Compose is validated again before image pulls or service
recreation; that second validation is the journal's durable forward-recovery
point. The stage and config files are private and are removed on success or
recovery.
The migration journal is durable through these states: `PREPARED`,
`BACKED_UP`, `ASSETS_ACTIVATED`, `CONFIG_ACTIVATED`, `VERSION_ACTIVATED`,
`COMPOSE_VALIDATED`, and `FINALIZED`. Before `COMPOSE_VALIDATED`, a later
update/repair rolls the runtime assets, private `config.env` backup, and
`VERSION` back as one old release. After that durable validation point, it
finishes forward cleanup and retains one private prior-release recovery set.
The journal contains paths and phase metadata only; it never contains secret
values. `config.env` backups are private files under the installation state
directory.

If target assets cannot be downloaded/validated, or Compose rejects them, the
active files, `config.env`, and `VERSION` remain at the previous release. A
later service or migration failure leaves a coherent prepared asset set and can
be retried with `stealth install --repair`; this process does not promise
zero-downtime upgrades or automatic database rollback. The supported legacy
baseline is `v0.2.5`; use the bridge procedure above, then upgrade through
coordinated releases. Database migrations are forward migrations. A binary or
image downgrade is not a database rollback: use a compatible backup restore
when the previous release cannot read the current schema. If target platform
migration succeeds but the final CLI executable replacement fails, the stack
is already at the target coordinated release while the old CLI remains. The
command reports that bounded skew explicitly; rerun `stealth update` to
reconcile the executable. An older CLI intentionally refuses a repair that
would downgrade a newer recorded platform release.

For an operator-managed checkout, the manual platform procedure below remains
available. Installed deployments using the host CLI should use the coordinated
command so managed assets and the runtime are migrated as one lifecycle.

## Manual platform upgrade

1. Read the GitHub Release notes for the target version, especially migration
   and configuration changes.
2. Verify PostgreSQL and object-storage backups and record where the restore
   artifacts are stored. See [`backup-restore.md`](backup-restore.md).
3. Update all production image variables in `.env.production` to the same
   immutable release tag or digest. Keep the setup image on the same release
   when fresh browser setup or repair may be used.
4. Pull the images and validate the rendered Compose file.
5. Stop or coordinate workers if the release notes require a quiet queue.
6. Start PostgreSQL/Redis if needed, then run the one-shot migration command.
7. Recreate API, worker, Console, proxy, the main Collector, host-metrics
   Collector, Docker-log Collector, Docker-metrics Collector, and proxy from
   the same release configuration.
8. Verify `/healthz`, `/readyz`, `/version`, worker health, and the HTTP smoke
   script. Check logs for migration and worker claim errors.

```bash
docker compose --env-file .env.production -f compose.production.yaml pull
docker compose --env-file .env.production -f compose.production.yaml up -d postgres redis clickhouse
docker compose --env-file .env.production -f compose.production.yaml up migrate
docker compose --env-file .env.production -f compose.production.yaml run --rm --no-deps cloudflare-setup-state-init
docker compose --env-file .env.production -f compose.production.yaml run --rm --no-deps cloudflare-state-init
docker compose --env-file .env.production -f compose.production.yaml up -d --force-recreate api worker console proxy otel-collector telemetry-host telemetry-docker-logs telemetry-docker-proxy telemetry-docker
./scripts/production-smoke.sh
```

The migration runner is deterministic and protected by a PostgreSQL advisory
lock. It fails loudly; it does not perform destructive automatic rollback.

When upgrading an installation that already has accounts, migration seals
public first-owner bootstrap as `legacy_installation` without assigning an
arbitrary account the privileged `instance_owner` role. This prevents an
existing deployment from unexpectedly exposing `/setup`. After the API is
healthy, a local operator may review and explicitly adopt an account:

```bash
stealth setup --adopt-owner
```

The command requires the dedicated `BOOTSTRAP_CLI_KEY`, lists existing account
identities locally, and requires the exact `ADOPT <account-id>` confirmation.
The assignment is audited and does not grant organization membership. Add the
dedicated `BOOTSTRAP_CLI_KEY`, `APPS_SECRET_KEY`, and `GITHUB_APP_CLIENT_ID` to
`config.env` before starting an upgraded API; the installer preserves a valid
App key and generates one only when missing. The App key is not interchangeable
with `FUNCTIONS_SECRET_KEY`.
Stop or coordinate the old application processes before applying migrations.

## Coordinated platform rollback

For a managed installation, run:

```bash
stealth rollback
```

The command checks the previous-release metadata and compares the live
`schema_migrations` fingerprint with the snapshot taken before the upgrade.
It proceeds only when the ledger is identical. It then restores the previous
managed files and `VERSION`, retargets canonical image references, preserves
the active operator configuration and rotated secrets, recreates API, Worker,
BuildKit, Console, ingress, and telemetry services, and checks readiness. The
operator UID and outcome are written to the private
`state/platform-rollback.jsonl` audit log. Run `stealth rollback` again after
an interrupted operation; its journal either restores the current release or
continues a Compose-validated rollback before retrying service checks.

This is an application/image rollback only. It never runs reverse migrations
or restores database/object-storage data. If the migration ledger advanced,
the command refuses before changing active files. Restore PostgreSQL and
object storage from verified backups by following
[`backup-restore.md`](backup-restore.md), then start a release compatible with
that backup. Previous-release sets without B3 metadata or a schema snapshot
also fail closed and require the verified backup path. The ledger cannot detect
manual schema edits or data changes outside recorded migrations, so keep and
verify backups before every production upgrade.
