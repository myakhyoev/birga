# API

Base URL locally: `http://localhost:8080`. The interactive spec is at
`/swagger/index.html`; this page explains the conventions behind it.

## Response envelope

Every response, success or failure, has the same shape:

```json
{"status": "Success", "error_code": 0, "error_note": "", "data": {}}
```

| Field | Meaning |
|---|---|
| `status` | `Success` or `Failure` |
| `error_code` | `0` on success, a negative code otherwise (table below) |
| `error_note` | human-readable reason; empty on success |
| `data` | the payload; `null` on failure |

## Errors

| error_code | HTTP | Meaning | Typical cause |
|---:|---:|---|---|
| 0 | 200 | success | |
| -10 | 422 | validation error | business rule broken, e.g. `unknown goal "flying"` |
| -11 | 400 | malformed request | bad JSON, non-integer `limit`, invalid UUID |
| -20 | 401 | unauthorized | missing or wrong `X-Admin-Key` |
| -21 | 403 | forbidden | admin API disabled (no `ADMIN_API_KEY` configured) |
| -30 | 404 | not found | unknown id, unpublished activity on a public endpoint, soft-deleted user |
| -40 | 409 | conflict | uniqueness conflict, e.g. `username is already taken` |
| -50 | 500 | internal error | anything unexpected; details only in server logs |
| -60 | 503 | dependency unavailable | `/health` when the database is unreachable |

For 500s the client only sees `"internal error"`. Find the real message in the logs by the
request id.

## Common headers

| Header | Direction | Meaning |
|---|---|---|
| `X-Request-Id` | request (optional) and response | correlates a call with server log lines; generated if absent |
| `X-Admin-Key` | request | admin key for `/v1/admin/*` |
| `Content-Type: application/json` | request | for bodies |

CORS allows any origin.

## Admin authentication

`/v1/admin/*` is protected by a single shared key, a stop-gap until real admin accounts exist.

- Send `X-Admin-Key: <ADMIN_API_KEY>`. The comparison is constant-time.
- Wrong or missing key: 401, code -20.
- `ADMIN_API_KEY` not set on the server: every admin call returns 403, code -21.

## Pagination

List endpoints take `limit` (default 20, capped at 100, must be positive) and `offset`
(default 0, must be non-negative), and return:

```json
{"items": [ ... ], "total": 42, "limit": 20, "offset": 0}
```

`total` is the number of matches before paging. Items are ordered newest first
(`created_at DESC, id`).

## Endpoints

### System

| Method | Path | Description |
|---|---|---|
| GET | `/ping` | liveness; always `data: "Pong"` |
| GET | `/health` | readiness; pings the database, `data: "OK"` or 503 |
| GET | `/swagger/*any` | Swagger UI and spec |

Metrics are on a separate port (`METRICS_PORT`, default 9090) at `/metrics`.

### Activities (public)

Only published activities are visible here.

| Method | Path | Description |
|---|---|---|
| GET | `/v1/activities` | list published activities |
| GET | `/v1/activities/{id}` | one published activity; unpublished ones return 404 |

Query parameters for the list:

| Param | Type | Rules |
|---|---|---|
| `age` | int | child age in years, 2 to 6; matches activities where `min_age <= age <= max_age` |
| `goal` | string | one of `language`, `motor`, `cognitive`, `social`, `emotional` |
| `limit`, `offset` | int | see [Pagination](#pagination) |

An `age` or `goal` outside the allowed values returns 422; a non-integer `age` returns 400.

Example:

```bash
curl 'localhost:8080/v1/activities?age=4&goal=cognitive&limit=10'
```

### Activities (admin)

| Method | Path | Description |
|---|---|---|
| POST | `/v1/admin/activities` | create an activity |
| GET | `/v1/admin/activities` | list all activities, including unpublished (same filters) |
| GET | `/v1/admin/activities/{id}` | one activity, including unpublished |

Create body:

```json
{
  "title_uz": "Rangli toshlar",
  "title_ru": "Цветные камни",
  "description_uz": "Toshlarni rangi bo'yicha saralang",
  "description_ru": "Сортируйте камни по цвету",
  "goal": "cognitive",
  "min_age": 3,
  "max_age": 5,
  "duration_minutes": 10,
  "is_published": true
}
```

Validation (`usecases/activity_creator`), all returning 422:

- titles and descriptions are required in both languages (whitespace is trimmed first);
- `goal` must be a known goal;
- `2 <= min_age <= max_age <= 6`;
- `1 <= duration_minutes <= 60`.

Example:

```bash
curl -X POST localhost:8080/v1/admin/activities \
  -H 'X-Admin-Key: change-me' -H 'Content-Type: application/json' \
  -d @activity.json
```

### Users (admin)

Users are app accounts (parents and caregivers). There is no user-facing auth yet, so these
endpoints are admin-only. Deleting is a soft delete; deleted users behave as if they do not
exist on every endpoint below.

| Method | Path | Description |
|---|---|---|
| POST | `/v1/admin/users` | create a user |
| GET | `/v1/admin/users` | list users, newest first ([Pagination](#pagination)) |
| GET | `/v1/admin/users/{id}` | one user |
| PATCH | `/v1/admin/users/{id}` | change some fields of a user |
| DELETE | `/v1/admin/users/{id}` | soft-delete a user; `data` is `null` |

Create body (only `phone_number` is required):

```json
{
  "name": "Dilnoza",
  "username": "dilnoza_95",
  "phone_number": "+998901234567",
  "photo_id": "3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"
}
```

Update body: the same fields, all optional.

- A field that is omitted or `null` is left unchanged.
- `""` clears `name`, `username` or `photo_id` (stored as `NULL`).
- `phone_number` cannot be cleared.
- An empty body (`{}`) returns 422 `nothing to update`.

Normalization and validation (`usecases/user_creator`, `usecases/user_updater`):

- every field is trimmed; `username` is lowercased, so usernames are effectively
  case-insensitive. On create, a blank optional field is stored as `NULL`;
- `phone_number`: E.164, a `+` and 8 to 15 characters in total, e.g. `+998901234567` (422);
- `username`: 3 to 32 characters from `a-z`, `0-9`, `_`, `.` (422);
- `name`: at most 100 characters (422);
- `photo_id`: a UUID (422). It is not checked against any files table yet;
- `username` and `phone_number` must be unique among non-deleted users: 409 with
  `username is already taken` or `phone number is already taken`;
- an id in the path that is not a UUID returns 400; an unknown or deleted user returns 404.

Soft delete (`DELETE`) sets `deleted_at`. In the same statement the database removes the
user's `user_auth` row (their tokens), and the username and phone number become free for a
new user. Deleting an already deleted user returns 404. There is no restore endpoint.

Example:

```bash
curl -X PATCH localhost:8080/v1/admin/users/7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11 \
  -H 'X-Admin-Key: change-me' -H 'Content-Type: application/json' \
  -d '{"name": "Dilnoza", "username": ""}'
```

### User object

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | generated by the database |
| `name` | string or `null` | display name |
| `username` | string or `null` | lowercase |
| `phone_number` | string or `null` | E.164; always set for users created through the API |
| `photo_id` | UUID string or `null` | profile photo id |
| `created_at`, `updated_at` | RFC 3339 timestamp | `updated_at` changes on every update and on delete |

`deleted_at` is never returned: deleted users are not returned at all.

### Activity object

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | generated by the database |
| `title_uz`, `title_ru` | string | |
| `description_uz`, `description_ru` | string | |
| `goal` | string | development goal |
| `min_age`, `max_age` | int | years, inclusive |
| `duration_minutes` | int | |
| `is_published` | bool | hidden from public endpoints when false |
| `created_at`, `updated_at` | RFC 3339 timestamp | |
