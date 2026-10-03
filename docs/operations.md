# Operations

How the API is built, deployed and watched. The server-side Docker Compose stack lives in
the separate `birga/devops` folder (not yet pushed to GitLab); its `README.md` has the full
step-by-step guide. This page summarises what a backend developer needs to know.

## Images

The `Dockerfile` has three stages:

| Target | Image | Contents |
|---|---|---|
| `builder` | `golang:1.27.1-alpine` | compiles a static binary |
| `migrations` | `migrate/migrate:v4.20.1` | `migrations/` baked in; entrypoint `migrate -path /migrations` |
| `runtime` (default) | `alpine:3.22` | the binary, CA certs, tzdata, `TZ=Asia/Tashkent`, non-root user `app`; exposes 8080 and 9090 |

CI pushes both on the default branch:

- `registry.gitlab.com/loyihalar/birga/backend:<short-sha>` and `:latest`
- `registry.gitlab.com/loyihalar/birga/backend/migrations:<short-sha>` and `:latest`

Servers only pull images; they never need the source tree.

## Deploying

In the devops folder on the server, set `BACKEND_TAG` in `.env` (a commit SHA from CI, or
`latest`) and run `make deploy`. It pulls the images, runs migrations, then restarts only the
API. Migrations must therefore be backward compatible with the API version still running
during the rollout.

## Compose stack (devops)

`COMPOSE_FILE` in the devops `.env` picks which layers run:

| File | Services |
|---|---|
| `compose.yaml` | `postgres`, `migrate`, `api` |
| `compose.dev.yaml` | builds from `../backend`, publishes ports, Adminer |
| `compose.proxy.yaml` | Caddy with automatic HTTPS |
| `compose.monitoring.yaml` | Prometheus, Alertmanager (Telegram), Grafana, Loki, Alloy, node/cAdvisor/postgres exporters |
| `compose.analytics.yaml` | Metabase |
| `compose.backup.yaml` | scheduled `pg_dump` with daily/weekly/monthly rotation |

## Observability

**Logs.** JSON on stdout. One access line per request; every line carries `request_id`.
On servers, Alloy ships container logs to Loki. In Grafana Explore:
`{service="api"} | json | request_id="<id>"`.

**Metrics.** `METRICS_PORT` (default 9090) serves `/metrics`, kept off the public port:

| Metric | Labels |
|---|---|
| `birga_http_server_request_duration_seconds` | `method`, `route`, `status`, `code` (application error code) |
| `birga_http_server_requests_in_flight` | `method` |
| `birga_http_client_request_duration_seconds` | `service`, `method`, `status` |
| `birga_db_query_duration_seconds` | `operation`, `status` |
| `birga_db_pool_*` | pool gauges and counters |

Go runtime and process metrics are included. `/ping`, `/health` and `/swagger/*` are excluded
from access logs and HTTP metrics.

**Dashboards and alerts.** The provisioned Grafana dashboard *Birga API* covers traffic,
error codes, latency, database, runtime and host. Alert rules (`devops/config/prometheus/rules/birga.yml`)
fire on API down, 5xx above 5%, p95 above 1s, DB pool and connection pressure, disk, memory
and container restarts, and go to Telegram when a bot token and chat id are configured.

**Probes.** `GET /ping` for liveness, `GET /health` for readiness (fails with 503 when the
database or Redis is unreachable).

**Redis.** The API needs Redis (`REDIS_URL`) for one-time codes and rate limits. The backend
`docker-compose.yml` runs one; the devops stack does not include it yet, so add a Redis
service there (no persistence or backups needed) before deploying this version.

**S3.** Profile photos go to the bucket in `S3_BUCKET` (see
[setup.md](setup.md#media-s3-locally) for the IAM permissions and public read access). Without
it, uploads answer 503 and the rest of the API works. S3 calls show up in
`birga_http_client_request_duration_seconds{service="s3"}`. Turn on bucket versioning or
a lifecycle rule if old photos should be kept or expired; the API never deletes a photo
that a media row points to.

## Backups

The backup service dumps the `birga` and `metabase` databases on a schedule into
`devops/backups/` (`last/`, `daily/`, `weekly/`, `monthly/`). `make backup` runs one now,
`make restore f=...` restores (the API is stopped meanwhile). Copy backups off the server
regularly and test restores.
