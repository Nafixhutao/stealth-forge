# Managed MySQL (free tier)

The hosted MySQL feature gives every project one real MySQL database and one
MySQL user on a shared tenant cluster. It is intentionally separate from the
built-in JSONB "Databases" product: that surface stores tenant rows inside the
control-plane PostgreSQL, while this one provisions a customer database that an
application can connect to with ordinary MySQL credentials.

## What the free tier includes

- MySQL 8.4 on the shared `tenant-mysql` service.
- One database per project, enforced by a unique constraint.
- One generated user, granted `ALL PRIVILEGES` on that database only.
- Ten concurrent connections, `utf8mb4_0900_ai_ci`.
- No internet-facing MySQL port. Customers use the shared phpMyAdmin at `db.<console-domain>` (or
  `/db/` when no database hostname is configured), or a project App reaches
  `tenant-mysql:3306` from the app runtime network.
- No automatic backups, no replicas, no point-in-time recovery.

Paid tiers can later move a project to a dedicated container or server without
changing the Console contract.

## Provisioning flow

1. The Console calls `POST /v1/projects/{projectID}/managed-databases`.
2. The API generates the MySQL database (`mdb_<24 hex>`), user
   (`u_<24 hex>`), and a 43-character password.
3. The password is sealed with `MANAGED_DATABASE_SECRET_KEY` and authenticated
   to the project and database ids.
4. A metadata row is inserted with status `provisioning`.
5. The API executes `CREATE DATABASE`, `CREATE USER`, the connection limit, and
   the database-scoped `GRANT` on the tenant cluster.
6. The row becomes `ready`. On any failure the MySQL objects and the metadata
   row are removed.

`DELETE` drops the database and user, then the metadata row. The Console shows
a typed-name confirmation because the operation is irreversible.

## Configuration

The two Compose services carry the `managed-databases` profile. Enable them
with `COMPOSE_PROFILES=managed-databases` in the installation `config.env` (the
installer writes this on fresh installations).

Required when `MANAGED_DATABASES_ENABLED=true`:

- `TENANT_MYSQL_ADMIN_DSN` — admin DSN, for example
  `root:<password>@tcp(tenant-mysql:3306)/`.
- `MANAGED_DATABASE_SECRET_KEY` — base64-encoded 32 bytes.
- `TENANT_MYSQL_HOST`, `TENANT_MYSQL_PORT`, `TENANT_PHPMYADMIN_URL`.
- `MANAGED_DATABASE_HOSTNAME` (optional) — public phpMyAdmin hostname. The
  tunnel reconciler adds its ingress rule and proxied CNAME.
- `TENANT_MYSQL_ROOT_PASSWORD` for the Compose service.
- `TENANT_PHPMYADMIN_BLOWFISH_SECRET`.

The API keeps running when the tenant cluster is unreachable; the managed
database routes answer `503 feature_unavailable` until the API restarts and the
cluster answers again.

## Security boundaries

- The tenant cluster is never the control-plane PostgreSQL. Customer users
  cannot see `projects`, `accounts`, sessions, or any other tenant.
- The API is the only component that uses the MySQL admin credential.
- phpMyAdmin is fixed to `tenant-mysql`, disables root login, and stores no
  credentials. Session cookies are scoped to `/db/`.
- The app runtime worker joins the hardened `tenant-mysql` container to the app
  bridge with the `tenant-mysql` alias. It is the only non-app peer allowed on
  that bridge besides the worker and Traefik.
- Customer users receive `USAGE` globally and `ALL PRIVILEGES` only on their
  own schema.
- Revealing a password writes an audit event `managed_database.password.reveal`.

## Known limitations

- Shared cluster: tenants share CPU, memory, and disk. There is no hard disk
  quota yet; monitor database sizes and warn before the fair-use limit.
- phpMyAdmin is internet-facing. Passwords are strong and root is blocked, but
  an operator should add a Cloudflare rate-limit/WAF rule for `/db/`.
- No backup/restore UI for the free tier.
- Functions do not join the app runtime network; only Apps can connect
  directly to `tenant-mysql` today.
- The Compose service and nginx route are release-managed assets. Deploying a
  change requires rebuilding the installation assets, not only the Console
  image.
