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

`docker-compose.yml` starts four services:

| Service | What it does |
|---|---|
| `postgres` | PostgreSQL 17, user/password/db `birga`, data in the `pgdata` volume |
| `redis` | Redis 7 for one-time codes and rate limits, no persistence |
| `migrate` | applies `migrations/` once, then exits |
| `api` | builds the Dockerfile; starts after migrations succeed |

If ports 5432, 6379, 8080 or 9090 are taken on your machine, override the host ports:

```bash
POSTGRES_HOST_PORT=55432 REDIS_HOST_PORT=56379 API_HOST_PORT=18080 METRICS_HOST_PORT=19090 make up
```

`make down` stops the stack (the database volume is kept).

The fuller stack with Grafana, Prometheus, Adminer and Metabase lives in the separate
`devops` folder; see [operations.md](operations.md).

## Run the API from source

```bash
docker compose up -d postgres redis migrate   # database and Redis only
make run                                # go run, loads .env if present
```

If you changed `POSTGRES_HOST_PORT` or `REDIS_HOST_PORT`, change the port in `POSTGRES_URL`
or `REDIS_URL` in `.env` as well.

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
| `REDIS_URL` | `redis://localhost:6379/0` | Redis for one-time codes and rate limits; `redis://:password@host:6379/0`, or `rediss://` for TLS. The process exits if Redis cannot be pinged at startup |
| `REDIS_CONNECT_TIMEOUT` | `5s` | startup ping timeout |
| `SMS_PROVIDER` | `log` | `playmobile` sends real SMS; `log` writes them (and OTP codes) to the log. `log` is refused when `ENVIRONMENT=production` |
| `PLAYMOBILE_BASE_URL` | `https://send.smsxabar.uz` | Play Mobile API host; requests go to `<base>/broker-api/send` |
| `PLAYMOBILE_USERNAME` | empty | Play Mobile login (HTTP Basic auth); required when `SMS_PROVIDER=playmobile` |
| `PLAYMOBILE_PASSWORD` | empty | Play Mobile password; required when `SMS_PROVIDER=playmobile`. Keep it in the server's secret store, never in git |
| `PLAYMOBILE_ORIGINATOR` | `3700` | sender name or short number registered with Play Mobile |
| `PLAYMOBILE_TIMEOUT` | `10s` | Play Mobile request timeout |
| `OTP_TTL` | `2m` | how long a code is valid (Redis key TTL) |
| `OTP_RESEND_COOLDOWN` | `1m` | minimum gap between codes for one phone and purpose |
| `OTP_MAX_PER_PHONE_HOUR` | `5` | codes per phone per hour |
| `OTP_MAX_PER_IP_HOUR` | `20` | codes per IP address per hour |
| `OTP_MAX_VERIFY_ATTEMPTS` | `5` | wrong codes before the code is deleted |
| `OTP_DEFAULT_CODE` | empty | 6 digits that `POST /v1/otp/verify` accepts for any phone without checking (development, testing, app review). Empty turns it off; the process exits if it is set with `ENVIRONMENT=production` or is not 6 digits. `.env.example` sets `654321` |
| `OTP_VERIFIED_TTL` | `10m` | how long a matched code marks the phone as verified; `POST /v1/auth/signup` needs a `sign_up` mark |
| `JWT_SECRET` | **required** | HS256 key for access and refresh tokens, at least 32 bytes (the process exits otherwise). Generate with `openssl rand -hex 32`, keep it in the server's secret store. Changing it invalidates every issued token. `.env.example` has a development-only value |
| `JWT_ISSUER` | `birga` | `iss` claim written into and required from every token |
| `JWT_ACCESS_TTL` | `24h` | access token lifetime |
| `JWT_REFRESH_TTL` | `0` | refresh token lifetime; `0` issues refresh tokens without an expiry (they work until the user is deleted or `JWT_SECRET` changes) |
| `S3_BUCKET` | empty | S3 bucket for uploaded media (profile photos). Empty disables `POST /v1/media` (it answers 503) |
| `S3_REGION` | `eu-central-1` | AWS region of the bucket |
| `S3_ACCESS_KEY_ID` | empty | IAM access key; set it together with `S3_SECRET_ACCESS_KEY`. Both empty means the AWS default credential chain (`AWS_*` variables, `~/.aws`, an instance or task role) |
| `S3_SECRET_ACCESS_KEY` | empty | IAM secret key. Keep it in the server's secret store, never in git |
| `S3_ENDPOINT` | empty | S3-compatible endpoint (MinIO, LocalStack), e.g. `http://localhost:9000`; also switches to path-style URLs. Empty means AWS |
| `S3_PUBLIC_BASE_URL` | empty | prefix of the photo URLs returned to clients, e.g. a CloudFront domain. Empty means `https://<bucket>.s3.<region>.amazonaws.com` (or `<endpoint>/<bucket>`) |
| `S3_KEY_PREFIX` | empty | prefix for every object key, e.g. `staging/`, to share one bucket between environments |
| `S3_TIMEOUT` | `30s` | timeout of one S3 request |
| `MEDIA_MAX_SIZE` | `5242880` | largest accepted upload in bytes (5 MiB), after base64 decoding |

When you add a setting, add it to `config.go`, `.env.example` and this table.

### SMS locally

With `.env.example` as is, `OTP_DEFAULT_CODE=654321`: after (or even without) `POST /v1/otp/send`,
verify with code `654321`.

Keep `SMS_PROVIDER=log` while developing: `POST /v1/otp/send` then logs a line
`sms not sent (SMS_PROVIDER=log)` with the code in `text`, which you pass to
`POST /v1/otp/verify`. To clear rate limits while testing, delete the keys:
`redis-cli --scan --pattern 'birga:otp:*' | xargs redis-cli del`. To send real SMS, get a Play Mobile
(smsxabar.uz) account and set `SMS_PROVIDER=playmobile`, `PLAYMOBILE_USERNAME`,
`PLAYMOBILE_PASSWORD` and `PLAYMOBILE_ORIGINATOR` from the contract. The API refuses to start
if `playmobile` is chosen without credentials.

### Media (S3) locally

Without `S3_BUCKET` the API starts normally and `POST /v1/media` answers 503. To try uploads,
either point it at a real bucket or run MinIO:

```bash
docker run -d --name minio -p 9000:9000 -e MINIO_ROOT_USER=minio -e MINIO_ROOT_PASSWORD=minio123 \
  minio/minio server /data
docker exec minio mc alias set local http://localhost:9000 minio minio123
docker exec minio mc mb local/birga-media && docker exec minio mc anonymous set download local/birga-media
```

and set `S3_BUCKET=birga-media S3_ENDPOINT=http://localhost:9000 S3_ACCESS_KEY_ID=minio
S3_SECRET_ACCESS_KEY=minio123`.

For AWS, the IAM user or role needs `s3:PutObject` and `s3:DeleteObject` on
`arn:aws:s3:::<bucket>/<S3_KEY_PREFIX>media/*`. The returned `url` only opens if clients can
read the objects: either a bucket policy allowing public `s3:GetObject` on that prefix, or a
CloudFront distribution in front of the bucket (set `S3_PUBLIC_BASE_URL` to its domain).

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

### Starter activities

A fresh database has no activities. Load the 12 starter activities (published, Uzbek and
Russian, two or three per goal) into the Docker Compose database:

```bash
make seed
```

It runs `seeds/activities.sql` through `psql` inside the `postgres` container and skips
titles that already exist, so it is safe to repeat. For another database, run the same file
with `psql "$POSTGRES_URL" -v ON_ERROR_STOP=1 -f seeds/activities.sql`. The texts are drafts
for development; the content team should review them before they reach real users.

## Tests and lint

```bash
make test               # unit tests (go test -short); DB tests are skipped
make test-integration   # also runs dbstore tests against POSTGRES_URL (must be migrated)
make race               # unit tests with the race detector
make coverage           # total coverage
make lint-go            # golangci-lint with .golangci.yml
```

`internal/redisstore` tests run the Redis scripts against an in-process
[miniredis](https://github.com/alicebob/miniredis), so they need no Redis server.

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
