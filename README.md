# Birga backend

Go API for the Birga caregiver app: curated offline activities for children aged 2 to 6.

## Quick start

```bash
cp .env.example .env
make up                 # postgres + migrations + api in docker compose
open http://localhost:8080/swagger/index.html
```

If ports 5432/8080/9090 are taken on your machine, override the host ports:
`POSTGRES_HOST_PORT=55432 API_HOST_PORT=18080 make up`.

To run the API from source against the compose database:

```bash
docker compose up -d postgres migrate
make run
```

## Layout

```
cmd/server/            entrypoint: reads env config, handles SIGINT/SIGTERM
api/docs/              generated swagger (make swag-init) - do not edit by hand
migrations/            golang-migrate SQL files (make migrate-create name=...)
internal/
  config/              env configuration (sethvargo/go-envconfig)
  bootstrap/           wiring: db -> dbstore -> drivers -> usecases -> gateways; graceful teardown
  domain/              plain business types, no dependencies
  errs/                application errors; the REST layer maps them to HTTP codes
  dbstore/             PostgreSQL repositories (pgx), transactions via InTx(ctx, fn)
  drivers/             integrations with external services (one package per service)
  usecases/            one package per business action, depends only on small interfaces
  gateways/rest/       gin server, routes, handlers + swagger annotations, response envelope
pkg/                   project-independent libraries
  logger/              zap wrapper with context fields (request id)
    ginlog/            request id, access log and panic recovery middlewares
    httplog/           logging http.RoundTripper for drivers
  metrics/             Prometheus: HTTP server/client, pgx queries, pool stats, /metrics server
  remote/              JSON-over-HTTP client used by drivers
```

Dependencies point inward: `gateways` and `bootstrap` know about `usecases`, `usecases`
declare the interfaces they need, and `dbstore` / `drivers` satisfy them.

## Adding a feature

1. Migration: `make migrate-create name=create_things_table`, then `make migrate-up`.
2. Domain type in `internal/domain`.
3. Repository in `internal/dbstore` (+ accessor on `DBStore`).
4. Use case package in `internal/usecases/<action>` with its own repo/driver interfaces and tests.
5. Handler + swagger comments in `internal/gateways/rest`, route in `routes.go`.
6. Wire it in `internal/bootstrap/init.go`, then `make swag-init`.

## API conventions

Every response uses one envelope:

```json
{"status": "Success", "error_code": 0, "error_note": "", "data": {}}
```

| error_code | HTTP | meaning                       |
|-----------:|-----:|-------------------------------|
| 0          | 200  | success                       |
| -10        | 422  | validation error              |
| -11        | 400  | malformed request             |
| -20        | 401  | unauthorized                  |
| -21        | 403  | forbidden                     |
| -30        | 404  | not found                     |
| -40        | 409  | conflict                      |
| -50        | 500  | internal error (details only in logs) |
| -60        | 503  | dependency unavailable        |

`/v1/admin/*` requires the `X-Admin-Key` header (`ADMIN_API_KEY`); with no key configured
the admin API is disabled. Send `X-Request-Id` to correlate your request with server logs.

## Observability

- Logs: JSON on stdout, one access line per request, every line carries `request_id`.
- Metrics: `http://localhost:9090/metrics` (separate port; do not expose publicly).
  - `birga_http_server_request_duration_seconds{method,route,status,code}`
  - `birga_http_server_requests_in_flight{method}`
  - `birga_http_client_request_duration_seconds{service,method,status}`
  - `birga_db_query_duration_seconds{operation,status}`
  - `birga_db_pool_*` connection pool gauges/counters
  - Go runtime and process metrics
- Probes: `GET /ping` (liveness), `GET /health` (readiness, pings the database).

## Development

```bash
make test               # unit tests
make test-integration   # + repository tests against POSTGRES_URL (migrated DB)
make lint-go            # golangci-lint v2
make swag-init          # regenerate swagger after changing handler comments
make help               # all targets
```
