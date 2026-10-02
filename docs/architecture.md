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
   ┌──────▼─────┐   ┌─────▼──────┐
   │ dbstore    │   │ drivers    │  PostgreSQL repositories / external services
   └──────┬─────┘   └─────┬──────┘
          ▼               ▼
      PostgreSQL     SMS gateway, ...

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
| `internal/domain/` | business types (`Activity`, `ActivityFilter`) and constants (goals, age range) |
| `internal/errs/` | `errs.Error` type and sentinel errors (`list.go`) |
| `internal/dbstore/` | PostgreSQL repositories and the transaction helper |
| `internal/drivers/` | clients for external services, one package each (`sms_service`) |
| `internal/usecases/` | `activity_creator`, `activity_getter`, `activity_lister` |
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
3. `dbstore.New(pool)`.
4. Drivers (currently the SMS client, not yet used by any use case).
5. Use cases.
6. REST server.

`App.Run` starts the metrics server on `METRICS_PORT` and the API on `HTTP_PORT`, then waits
for a signal or a server error. Teardown runs in reverse order: the HTTP server gets 10 seconds
to finish in-flight requests, then the DB pool closes, then the logger flushes.

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

- Repositories hang off `DBStore` (`store.Activity()`).
- `DBStore.InTx(ctx, fn)` runs `fn` in a read-committed transaction. Repository calls made
  with the `ctx` that `fn` receives join the transaction automatically
  (`sqlClientByCtx` picks the `pgx.Tx` from the context, or the pool otherwise).
- Queries use positional parameters only; dynamic filters build the `WHERE` clause from
  fixed fragments (`activityWhere`), so values never enter the SQL text.
- Rows scan into `db*` structs with `db:"..."` tags and convert to domain types with
  `toDomain()`.

## Drivers

A driver wraps one external service using `pkg/remote`, with `pkg/metrics.RoundTripper` and
`pkg/logger/httplog` in the transport so every outgoing call is timed and logged. It maps
provider failures to `errs` values (4xx to `ErrBadRequest`, transport failures to
`ErrConnection`).

The SMS driver (`internal/drivers/sms_service`) sends a message and returns the provider's
id. Its request contract is generic and must be adapted to the provider chosen (Eskiz, Play
Mobile, ...). It is built at startup for the upcoming phone sign-in (OTP) feature.

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
