# Data model

PostgreSQL 17. The schema is defined only by the SQL files in `migrations/`, applied with
[golang-migrate](https://github.com/golang-migrate/migrate). The `schema_migrations` table
that golang-migrate creates records the current version.

## Tables

### `activities`

The curated catalogue of short offline activities. Created by
`000001_create_activities_table`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `title_uz` | `TEXT` | no | | Uzbek title |
| `title_ru` | `TEXT` | no | | Russian title |
| `description_uz` | `TEXT` | no | | Uzbek instructions |
| `description_ru` | `TEXT` | no | | Russian instructions |
| `goal` | `TEXT` | no | | development goal, see below |
| `min_age` | `SMALLINT` | no | | youngest suitable age, years |
| `max_age` | `SMALLINT` | no | | oldest suitable age, years |
| `duration_minutes` | `SMALLINT` | no | | expected length |
| `is_published` | `BOOLEAN` | no | `FALSE` | only published rows are visible to the app |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | not yet updated automatically (no update endpoint exists) |

Constraints:

- `activities_age_range_chk`: `min_age >= 2 AND max_age <= 6 AND min_age <= max_age`
- `activities_duration_chk`: `duration_minutes > 0`

Indexes:

- primary key on `id`
- `activities_published_goal_idx` on `(goal) WHERE is_published`, for the public list
  filtered by goal

Goals are plain text, not an enum type or lookup table. The allowed values (`language`,
`motor`, `cognitive`, `social`, `emotional`) are checked in Go
(`domain.IsKnownGoal`). The application additionally caps `duration_minutes` at 60, which
the database does not enforce.

### `users`

App users (parents). Created by `000002_create_users_tables`. Rows are soft-deleted by
setting `deleted_at`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `name` | `TEXT` | yes | | display name |
| `username` | `TEXT` | yes | | unique among non-deleted users |
| `phone_number` | `VARCHAR(15)` | yes | | E.164 length limit; unique among non-deleted users |
| `photo_id` | `UUID` | yes | | id of the profile photo, references `media(id)` `ON DELETE SET NULL` (constraint `users_photo_id_fkey`, added by `000003_create_media_table`) |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | set by the application on update and soft delete (no trigger) |
| `deleted_at` | `TIMESTAMPTZ` | yes | | set on soft delete; `NULL` means active |

Indexes:

- primary key on `id`
- `users_username_uniq`: unique on `(username) WHERE deleted_at IS NULL`
- `users_phone_number_uniq`: unique on `(phone_number) WHERE deleted_at IS NULL`

The partial unique indexes let a soft-deleted user's username or phone number be reused.

Application rules not enforced by the database (see [api.md](api.md#users-admin)):
`phone_number` is required on create and E.164; `username` is stored lowercase and matches
`[a-z0-9_.]{3,32}`; `name` is at most 100 characters. `internal/dbstore/user.go` reads only
rows with `deleted_at IS NULL` and maps violations of `users_username_uniq` and
`users_phone_number_uniq` to 409 conflicts, and a violation of `users_photo_id_fkey` (a
`photo_id` that was never uploaded) to 422.

### `user_auth`

Authentication state for a user, one row per user. Created by
`000002_create_users_tables`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | | primary key and foreign key to `users(id)` |
| `fcm_token` | `TEXT[]` | no | `'{}'` | Firebase Cloud Messaging tokens, one per device |
| `access_token` | `TEXT` | yes | | current access token |
| `refresh_token` | `TEXT` | yes | | current refresh token |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | not updated automatically |

Constraints:

- `id` references `users(id)` `ON DELETE CASCADE`: hard-deleting a user removes their
  auth row.

Triggers:

- `users_soft_delete_auth_trg` on `users` (function `users_soft_delete_auth()`): when a
  user's `deleted_at` changes from `NULL` to a value, their `user_auth` row is deleted, so
  a soft delete also drops the user's tokens. Restoring the user (`deleted_at` back to
  `NULL`) does not bring the row back; they sign in again.

### `media`

Files uploaded through `POST /v1/media` (today: profile photos). The bytes live in S3; this
table records what was uploaded. Created by `000003_create_media_table`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key. The API generates it before uploading, because it is part of the object key |
| `object_key` | `TEXT` | no | | S3 key, `<S3_KEY_PREFIX>media/<id>.<jpg\|png\|webp>`; unique |
| `content_type` | `TEXT` | no | | `image/jpeg`, `image/png` or `image/webp`, detected from the bytes |
| `size_bytes` | `BIGINT` | no | | file size; `CHECK (size_bytes > 0)` |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |

The table has no `updated_at` or `deleted_at`: a media row never changes, and a new photo
is a new row. The public URL is not stored; it is built from `object_key` and
`S3_PUBLIC_BASE_URL`, so the CDN or bucket address can change without a migration. The
object is uploaded first and the row inserted after; if the insert fails the object is
deleted again. Replaced photos are not cleaned up yet (no cleanup job).

## Conventions

- Primary keys are UUIDs generated by PostgreSQL.
- Every table has `created_at` and `updated_at` as `TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
  except append-only tables such as `media`, which only have `created_at`.
- Soft-deletable tables use a nullable `deleted_at TIMESTAMPTZ`; uniqueness on them is
  enforced with partial indexes `WHERE deleted_at IS NULL`.
- User-facing text is stored per language in `_uz` and `_ru` columns.
- Business invariants that are cheap to state in SQL also get a `CHECK` constraint, so bad
  data cannot enter even through manual SQL.
- Each migration has an `.up.sql` and a `.down.sql`; numbering is sequential with six digits.

## Redis keys

Short-lived state lives in Redis, not PostgreSQL (`internal/redisstore`). Every key starts
with `birga:` so the instance can be shared. Nothing in Redis needs a backup: losing it only
means users request a new code.

| Key | Type | TTL | Holds |
|---|---|---|---|
| `birga:otp:code:<purpose>:<phone>` | hash `{hash, attempts}` | `OTP_TTL` (2 min) | SHA-256 of `<phone>:<purpose>:<code>` (`domain.HashOTPCode`) and wrong attempts so far; deleted on a match or after `OTP_MAX_VERIFY_ATTEMPTS` wrong codes |
| `birga:otp:cooldown:<purpose>:<phone>` | string | `OTP_RESEND_COOLDOWN` (1 min) | present while a new code may not be sent |
| `birga:otp:limit:phone:<phone>` | counter | 1 hour from the first code | codes sent to the phone, any purpose |
| `birga:otp:limit:ip:<ip>` | counter | 1 hour from the first code | codes requested from the IP (canonical form, e.g. `2001:db8::1`) |

`<phone>` is E.164 (`+998901234567`), `<purpose>` is `sign_up` or `update_user`.

## Database roles (servers)

On a fresh server volume the devops stack creates extra roles: `birga_readonly` (read-only,
used by Metabase), `metabase` (its own database) and `exporter` (monitoring), and enables
`pg_stat_statements`. See [operations.md](operations.md).
