# Local setup

## Prerequisites

- Go 1.27.1 (see `go.mod`)
- Docker with the compose plugin (on macOS, Docker Desktop or colima)
- Optional, for running parts outside Docker:
  - [golang-migrate](https://github.com/golang-migrate/migrate) CLI (`migrate`) for `make migrate-*`
  - [golangci-lint](https://golangci-lint.run) v2 for `make lint-go`

## Run everything in Docker

```bash
cp .env.example .env
make up                 # postgres + migrations + api (docker-compose.yml)
open http://localhost:8080/swagger/index.html
```

`docker-compose.yml` starts three services:

| Service | What it does |
|---|---|
| `postgres` | PostgreSQL 17, user/password/db `birga`, data in the `pgdata` volume |
| `migrate` | applies `migrations/` once, then exits |
| `api` | builds the Dockerfile; starts after migrations succeed |

If ports 5432, 8080 or 9090 are taken on your machine, override the host ports:

```bash
POSTGRES_HOST_PORT=55432 API_HOST_PORT=18080 METRICS_HOST_PORT=19090 make up
```

`make down` stops the stack (the database volume is kept).

The fuller stack with Grafana, Prometheus, Adminer and Metabase lives in the separate
`devops` folder; see [operations.md](operations.md).

## Run the API from source

```bash
docker compose up -d postgres migrate   # database only
make run                                # go run, loads .env if present
```

If you changed `POSTGRES_HOST_PORT`, change the port in `POSTGRES_URL` in `.env` as well.

Check it is up:

```bash
curl localhost:8080/ping      # liveness
curl localhost:8080/health    # readiness, pings the database
```

## Configuration

All configuration comes from environment variables (`internal/config/config.go`, loaded
with `sethvargo/go-envconfig`). `.env.example` lists the usual ones.

| Variable | Default | Meaning |
|---|---|---|
| `ENVIRONMENT` | `development` | `development`, `staging` or `production`; `production` puts gin in release mode |
| `LOG_LEVEL` | `info` | zap level: `debug`, `info`, `warn`, `error` |
| `HTTP_PORT` | `:8080` | API listen address |
| `METRICS_PORT` | `:9090` | Prometheus `/metrics` listen address; never expose publicly |
| `ADMIN_API_KEY` | empty | shared key for `/v1/admin/*` (`X-Admin-Key` header); empty disables the admin API |
| `POSTGRES_URL` | **required** | full connection string, e.g. `postgres://birga:birga@localhost:5432/birga?sslmode=disable` |
| `POSTGRES_MAX_CONNS` | `25` | pool max connections |
| `POSTGRES_MIN_CONNS` | `2` | pool min connections |
| `POSTGRES_MAX_CONN_LIFETIME` | `30m` | recycle connections after this |
| `POSTGRES_MAX_CONN_IDLE_TIME` | `5m` | close idle connections after this |
| `POSTGRES_CONNECT_TIMEOUT` | `5s` | startup connect + ping timeout; the process exits if exceeded |
| `SMS_PROVIDER` | `log` | `playmobile` sends real SMS; `log` writes them (and OTP codes) to the log. `log` is refused when `ENVIRONMENT=production` |
| `PLAYMOBILE_BASE_URL` | `https://send.smsxabar.uz` | Play Mobile API host; requests go to `<base>/broker-api/send` |
| `PLAYMOBILE_USERNAME` | empty | Play Mobile login (HTTP Basic auth); required when `SMS_PROVIDER=playmobile` |
| `PLAYMOBILE_PASSWORD` | empty | Play Mobile password; required when `SMS_PROVIDER=playmobile`. Keep it in the server's secret store, never in git |
| `PLAYMOBILE_ORIGINATOR` | `3700` | sender name or short number registered with Play Mobile |
| `PLAYMOBILE_TIMEOUT` | `10s` | Play Mobile request timeout |
| `OTP_TTL` | `3m` | how long a code is valid |
| `OTP_RESEND_COOLDOWN` | `1m` | minimum gap between codes for one phone and purpose |
| `OTP_MAX_PER_PHONE_HOUR` | `5` | codes per phone per hour |
| `OTP_MAX_PER_IP_HOUR` | `20` | codes per IP address per hour |

When you add a setting, add it to `config.go`, `.env.example` and this table.

### SMS locally

Keep `SMS_PROVIDER=log` while developing: `POST /v1/otp/send` then logs a line
`sms not sent (SMS_PROVIDER=log)` with the code in `text`. To send real SMS, get a Play Mobile
(smsxabar.uz) account and set `SMS_PROVIDER=playmobile`, `PLAYMOBILE_USERNAME`,
`PLAYMOBILE_PASSWORD` and `PLAYMOBILE_ORIGINATOR` from the contract. The API refuses to start
if `playmobile` is chosen without credentials.

## Database migrations

Migrations are plain SQL files run by golang-migrate. Details of the schema are in
[data-model.md](data-model.md).

```bash
make migrate-create name=create_children_table   # new up/down pair in migrations/
make migrate-up                                    # apply all (uses POSTGRES_URL)
make migrate-down                                  # roll back the last one
```

The Makefile's `POSTGRES_URL` defaults to `localhost:5432`; pass
`POSTGRES_URL=...` if your port differs.

## Tests and lint

```bash
make test               # unit tests (go test -short); DB tests are skipped
make test-integration   # also runs dbstore tests against POSTGRES_URL (must be migrated)
make race               # unit tests with the race detector
make coverage           # total coverage
make lint-go            # golangci-lint with .golangci.yml
```

Repository tests in `internal/dbstore` run only when `TEST_POSTGRES_URL` is set and `-short`
is not passed; `make test-integration` sets it from `POSTGRES_URL`.

## Swagger

Handler comments in `internal/gateways/rest/handlers.go` and the top-level annotations in
`cmd/server/main.go` generate `api/docs/`. After changing them:

```bash
make swag-init
```

Commit the regenerated files with the change.

## CI

`.gitlab-ci.yml` runs on every push:

| Stage | Job | What |
|---|---|---|
| lint | `lint` | golangci-lint v2 |
| test | `test` | PostgreSQL 17 service, migrations, `go test -race` with coverage |
| build | `build-image` | default branch only: pushes `registry.gitlab.com/loyihalar/birga/backend:<sha>` and `:latest`, plus `.../migrations:<sha>` and `:latest` |
