# Architecture

The API is a single Go binary (`cmd/server`) built with
[gin](https://github.com/gin-gonic/gin) for HTTP and [pgx](https://github.com/jackc/pgx)
for PostgreSQL. It follows a layered "clean architecture" layout: business rules in the
middle, transport and storage at the edges.

## Layers

```
            HTTP request
                 │
   ┌─────────────▼──────────────┐
   │ gateways/rest              │  routing, auth, parsing, response envelope
   └─────────────┬──────────────┘
                 │ small interfaces declared by the gateway
   ┌─────────────▼──────────────┐
   │ usecases/<action>          │  one package per business action, validation
   └──────┬───────────────┬─────┘
          │               │ interfaces declared by each use case
   ┌──────▼──────────────┐   ┌─────▼──────┐
   │ dbstore, redisstore │   │ drivers    │  storage / external services
   └──────┬──────────────┘   └─────┬──────┘
          ▼                        ▼
   PostgreSQL, Redis          Play Mobile (SMS), AWS S3, ...

   domain/  plain types shared by every layer (no dependencies)
   errs/    application errors, mapped to HTTP codes by the gateway
```

Dependencies point inward. A use case declares the interface it needs (for example
`activityRepo` with a single `Create` method) and `dbstore` satisfies it implicitly. The REST
server likewise depends on interfaces, not on use case structs. This keeps every layer unit
testable with small fakes.

## Directory map

| Path | Purpose |
|---|---|
| `cmd/server/` | entrypoint: reads env config, handles SIGINT/SIGTERM, top-level swagger annotations |
| `internal/config/` | `config.Application`, loaded from environment variables ([setup.md](setup.md#configuration)) |
| `internal/bootstrap/` | wires everything: logger, DB pool, `dbstore`, drivers, use cases, REST server; runs teardown on shutdown |
| `internal/domain/` | business types (`Activity`, `ActivityFilter`, `User`, `UserUpdate`, `UserFilter`, `OTPPurpose`, `OTPSendRequest`, `OTPVerifyRequest`, `OTPLimits`, `OTPVerifyOutcome`, `Media`, `MediaUpload`), constants and format checks (goals, age range, username, phone, Uzbek phone, OTP code), OTP hashing |
| `internal/errs/` | `errs.Error` type and sentinel errors (`list.go`) |
| `internal/dbstore/` | PostgreSQL repositories and the transaction helper |
| `internal/redisstore/` | Redis state: one-time codes and the send-OTP rate limiter (`store.OTP()`) |
| `internal/drivers/` | clients for external services, one package each (`playmobile`; `smslog` is the fake SMS sender; `s3storage` for files) |
| `internal/usecases/` | `activity_creator`, `activity_getter`, `activity_lister`, `user_creator`, `user_getter`, `user_lister`, `user_updater`, `user_deleter`, `otp_sender`, `otp_verifier`, `media_uploader` |
| `internal/gateways/rest/` | gin server, middleware, routes, handlers with swagger comments, response envelope |
| `pkg/logger/` | zap wrapper carrying request-scoped fields through `context.Context`; `ginlog` (request id, access log, recovery), `httplog` (outgoing call logging) |
| `pkg/metrics/` | Prometheus collectors for HTTP server, HTTP client, pgx queries and pool; `/metrics` server |
| `pkg/remote/` | JSON-over-HTTP client used by drivers; non-2xx responses become `*remote.StatusError` |
| `api/docs/` | generated swagger (`make swag-init`); never edit by hand |
| `migrations/` | golang-migrate SQL files ([data-model.md](data-model.md)) |

`pkg/` holds project-independent code written for Birga. Birga does not depend on any
company-internal libraries.

## Startup and shutdown

`bootstrap.New` builds the app in this order, registering a teardown function for each
resource that needs closing:

1. Logger (`pkg/logger`), JSON on stdout, installed as zap's global logger.
2. PostgreSQL pool (`pgxpool`) with the metrics tracer; the process exits if the database
   cannot be pinged within `POSTGRES_CONNECT_TIMEOUT`.
3. `dbstore.New(pool)`, then the Redis client (`REDIS_URL`); the process exits if Redis cannot
   be pinged within `REDIS_CONNECT_TIMEOUT`. `redisstore.New(rdb)`.
4. Drivers: the SMS sender chosen by `SMS_PROVIDER` (`playmobile` or `log`). Startup fails on
   an unknown provider, on `playmobile` without credentials, and on `log` in production.
5. Use cases.
6. REST server.

`App.Run` starts the metrics server on `METRICS_PORT` and the API on `HTTP_PORT`, then waits
for a signal or a server error. Teardown runs in reverse order: the HTTP server gets 10 seconds
to finish in-flight requests, then the Redis client and the DB pool close, then the logger
flushes. `/health` pings both PostgreSQL and Redis (`bootstrap.healthCheck`).

## Request flow

Middleware order on every request (`gateways/rest/server.go`):

1. `ginlog.RequestID` reads `X-Request-Id` or generates one, stores it in the context and
   echoes it in the response.
2. `ginlog.Recovery` turns panics into a 500 and logs the stack.
3. `ginlog.LogExcept` writes one access log line per request (skips `/ping`, `/health`,
   `/swagger/*`).
4. `metrics.Gin` records duration and in-flight counters, labelled with the application
   error code.
5. CORS: allows any origin, answers `OPTIONS` with 204.

`/v1/admin/*` additionally runs `adminAuth` (see [api.md](api.md#admin-authentication)).

A handler parses input, calls one use case, and passes the result to `rest.Return`, which
writes the response envelope.

`rest.New` takes each activity use case as its own argument; the user use cases come grouped
in `rest.UserUseCases` (creator, lister, getter, updater, deleter) and the OTP ones in
`rest.OTPUseCases` (sender, verifier). Group the use cases of
new resources the same way rather than growing the argument list.

## Errors

All errors that should reach a client are `*errs.Error` values that wrap a sentinel from
`internal/errs/list.go` (`ErrValidation`, `ErrBadRequest`, `ErrNotFound`, ...).

- Create one with `errs.Errf(errs.ErrValidation, "unknown goal %q", goal)`; it still matches
  `errors.Is(err, errs.ErrValidation)` but carries a specific message.
- `errs.Wrap(err)` classifies any unknown error (driver, SQL) as `ErrInternal`.
- `rest.Return` maps the sentinel to an HTTP status and `error_code`. For internal errors it
  returns only `"internal error"` to the client; the real message goes to the access log.

The full mapping table is in [api.md](api.md#errors).

## Database access

- Repositories hang off `DBStore` (`store.Activity()`, `store.User()`, `store.OTP()`).
- `DBStore.InTx(ctx, fn)` runs `fn` in a read-committed transaction. Repository calls made
  with the `ctx` that `fn` receives join the transaction automatically
  (`sqlClientByCtx` picks the `pgx.Tx` from the context, or the pool otherwise).
- Queries use positional parameters only; dynamic filters and partial updates build the
  `WHERE` / `SET` clause from fixed fragments (`activityWhere`, `userSet`), so values never
  enter the SQL text.
- Soft-deletable tables (`users`) are filtered with `deleted_at IS NULL` in every
  repository query, and delete is an `UPDATE ... SET deleted_at = NOW()`.
- A unique-index violation (`23505`) is mapped to a specific `errs.ErrConflict` error by
  constraint name (`userConflict`), which the gateway turns into a 409.
- Rows scan into `db*` structs with `db:"..."` tags and convert to domain types with
  `toDomain()`.

## Redis

`internal/redisstore` holds state that expires: one-time codes and rate-limit counters (key
layout in [data-model.md](data-model.md#redis-keys)). It uses
[go-redis](https://github.com/redis/go-redis) v9.

- Logic that must not race runs as a Lua script (`redis.NewScript`), which Redis executes
  atomically: `acquireScript` checks the cooldown and both hourly counters and only then counts
  the send; `verifyScript` compares the hash and deletes the code on a match, or counts the
  attempt and deletes the code after the last one. Several API instances can share one Redis.
- Scripts return small integer tuples; the Go side maps them to `errs` values
  (`ErrRateLimited` with the wait in seconds) or `domain.OTPVerifyOutcome`.
- Use cases declare the methods they need (`otpStore`), exactly as with `dbstore`.

## Drivers

A driver wraps one external service using `pkg/remote`, with `pkg/metrics.RoundTripper` and
`pkg/logger/httplog` in the transport so every outgoing call is timed and logged. It maps
provider failures to `errs` values (4xx to `ErrBadRequest`, transport failures to
`ErrConnection`).

SMS drivers implement `Send(ctx, messageID, phone, text) error`; the use case passes its own
message id (a fresh UUID per send).

- `internal/drivers/playmobile` calls Play Mobile (smsxabar.uz): `POST
  <PLAYMOBILE_BASE_URL>/broker-api/send` with HTTP Basic auth and
  `{"messages": [{"recipient": "998901234567", "message-id": "<id>", "sms": {"originator":
  "<PLAYMOBILE_ORIGINATOR>", "content": {"text": "..."}}}]}`. Any 2xx means accepted (the body
  is plain text). A 400 carries `{"error-code", "error-description"}` (for example 102 account
  locked, 401 empty originator); since input is validated first, 4xx is treated as our
  misconfiguration (`ErrInternal`), 5xx and network errors as `ErrConnection`. Delivery
  reports (webhook) are not handled yet.
- `internal/drivers/smslog` logs the message instead of sending it, for local development.

- `internal/drivers/s3storage` stores files in S3 with the AWS SDK for Go v2 (the SDK, not
  `pkg/remote`, because S3 needs SigV4 signing). Its HTTP client still goes through
  `metrics.RoundTripper` (service `s3`). It offers `Put`, `Delete` and `URL(key)`; failures
  become `ErrConnection`. Objects are written with `Cache-Control: public, max-age=31536000,
  immutable`, since a key is never reused. `S3_ENDPOINT` points it at MinIO or LocalStack.
  With `S3_BUCKET` empty, bootstrap builds no S3 client and no `media_uploader`, and the REST
  server answers `POST /v1/media` with 503.

Request and response bodies of outgoing calls are not logged (`httplog` without
`WithBodies`), so OTP codes never reach the logs when Play Mobile is used.

## Adding a feature

1. Migration: `make migrate-create name=create_things_table`, then `make migrate-up`.
2. Domain type in `internal/domain`.
3. Repository in `internal/dbstore`, plus an accessor on `DBStore`.
4. Use case package in `internal/usecases/<action>` with its own repo/driver interfaces and
   tests.
5. Handler with swagger comments in `internal/gateways/rest`, route in `routes.go`.
6. Wire it in `internal/bootstrap/init.go`, then run `make swag-init`.
7. Update the docs: [api.md](api.md) for new endpoints, [data-model.md](data-model.md) for
   new tables, this file for new layers or patterns, [setup.md](setup.md) for new config.
