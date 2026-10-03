# Data model

PostgreSQL 17. The schema is defined only by the SQL files in `migrations/`, applied with
[golang-migrate](https://github.com/golang-migrate/migrate). The `schema_migrations` table
that golang-migrate creates records the current version.

## Tables

### `activities`

The curated catalogue of short offline activities. Created by
`000001_create_activities_table`; `deleted_at` added by `000006_create_activity_completions`.
Rows are soft-deleted by setting `deleted_at`.

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
| `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | set by the application on update and soft delete (no trigger) |
| `deleted_at` | `TIMESTAMPTZ` | yes | | set on soft delete; `NULL` means active |

Constraints:

- `activities_age_range_chk`: `min_age >= 2 AND max_age <= 6 AND min_age <= max_age`
- `activities_duration_chk`: `duration_minutes > 0`

Indexes:

- primary key on `id`
- `activities_published_goal_idx` on `(goal) WHERE is_published AND deleted_at IS NULL`, for
  the public list filtered by goal (recreated with the `deleted_at` condition by `000006`)

Goals are plain text, not an enum type or lookup table. The allowed values (`language`,
`motor`, `cognitive`, `social`, `emotional`) are checked in Go
(`domain.IsKnownGoal`). The application additionally caps `duration_minutes` at 60, which
the database does not enforce.

Soft delete rather than a hard delete keeps `activity_completions` rows pointing at a real
activity, so history and streaks survive. Every repository read filters `deleted_at IS NULL`.

Repository (`store.Activity()`): `Create`, `Get`, `List` (filters and paging), `Update`
(partial; a failed CHECK becomes `ErrValidation`), `Delete` (soft) and `Recommend` (see
[api.md](api.md#recommendation)).

A starter set of 12 published activities (two or three per goal, Uzbek and Russian) is in
`seeds/activities.sql`; load it with `make seed` ([setup.md](setup.md#starter-activities)).
It is data, not a migration, and skips titles that already exist.

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
`000002_create_users_tables`; `username` and `password` added by
`000004_add_user_auth_credentials`; `role` added by `000005_add_user_auth_role`. `POST /v1/auth/signup` inserts the row.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | | primary key and foreign key to `users(id)` |
| `fcm_token` | `TEXT[]` | no | `'{}'` | Firebase Cloud Messaging tokens, one per device |
| `access_token` | `TEXT` | yes | | SHA-256 (hex) of the newest access token (`domain.HashToken`) |
| `refresh_token` | `TEXT` | yes | | SHA-256 (hex) of the current refresh token; `POST /v1/auth/refresh` accepts only this one |
| `username` | `TEXT` | yes | | sign-in username, same value as `users.username` (kept in sync by `users_sync_auth_username_trg`) |
| `role` | `user_role` | no | `'user'` | enum `user`, `admin`, `paid_user`. Sign-up accepts `user` and `paid_user`; `admin` is set only in the database for now. Copied into the tokens' `role` claim |
| `password` | `TEXT` | yes | | bcrypt hash (`$2a$10$...`) of the password; the plain password is never stored |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | not updated automatically |

Constraints:

- `id` references `users(id)` `ON DELETE CASCADE`: hard-deleting a user removes their
  auth row.
- `user_auth_username_uniq`: unique on `(username)`. Rows of soft-deleted users are removed,
  so their usernames are free again, matching `users_username_uniq`.

Tokens are stored as hashes so a leaked table cannot be replayed as tokens.

Triggers:

- `users_soft_delete_auth_trg` on `users` (function `users_soft_delete_auth()`): when a
  user's `deleted_at` changes from `NULL` to a value, their `user_auth` row is deleted, so
  a soft delete also drops the user's tokens. Restoring the user (`deleted_at` back to
  `NULL`) does not bring the row back; they sign in again.
- `users_sync_auth_username_trg` on `users` (function `users_sync_auth_username()`): when
  `users.username` changes (for example through `PATCH /v1/admin/users/:id`), the same value
  is written to `user_auth.username`.

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

### `children`

Child profiles. Created by `000005_create_children_tables`. Rows are soft-deleted by setting
`deleted_at`. A child belongs to its parents through `user_children`, not through a column.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `name` | `TEXT` | no | | display name |
| `age` | `SMALLINT` | no | | age in years, as entered; not advanced automatically |
| `gender` | `TEXT` | no | | `male` or `female` |
| `photo_id` | `UUID` | yes | | references `media(id)` `ON DELETE SET NULL` (constraint `children_photo_id_fkey`), same as `users.photo_id` |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | set by the application on update and soft delete (no trigger) |
| `deleted_at` | `TIMESTAMPTZ` | yes | | set on soft delete; `NULL` means active |

Constraints:

- `children_age_chk`: `age BETWEEN 0 AND 18`. Wider than the 2 to 6 activity range
  (`domain.MinChildAge`/`MaxChildAge`) so a profile stays valid as the child grows;
  `domain.IsValidChildAge` checks the same 0 to 18 range.
- `children_gender_chk`: `gender IN ('male', 'female')` (`domain.IsKnownGender`).

`internal/dbstore/child.go` reads only rows with `deleted_at IS NULL`, maps a failed check to
`errs.ErrValidation` and a `photo_id` that was never uploaded to `errs.ErrPhotoNotFound`.

### `user_children`

Many-to-many link between parents and children: a user can have several children and a
child several parents. Created by `000005_create_children_tables`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `user_id` | `UUID` | no | | references `users(id)` `ON DELETE CASCADE` |
| `child_id` | `UUID` | no | | references `children(id)` `ON DELETE CASCADE` |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | when the link was made |

Indexes:

- primary key on `(user_id, child_id)`, which also serves "children of a user"
- `user_children_child_id_idx` on `(child_id)`, for "parents of a child"

Links are not soft-deleted. Soft-deleting a user or a child leaves its links in place;
every read joins through active rows only, so the deleted side simply disappears (a
deleted parent is not listed by `childRepo.Parents`, a deleted child is not listed for its
parents). Hard deletes remove the links through the cascades.

Repository (`store.Child()`): `Create`, `Get`, `List` (optionally by `ParentID`, paged),
`Update` (partial, `PhotoID` `""` clears it), `Delete` (soft), `AddParent` (idempotent;
`ErrChildNotFound` or `ErrUserNotFound` when either side is missing or deleted),
`RemoveParent` (`ErrChildNotFound` when the link does not exist) and `Parents`. Creating a
child and linking it to its first parent are two calls; a use case runs them in
`DBStore.InTx`.

### `activity_completions`

One row per activity a child did on a day, with the caregiver's optional reflection. Created
by `000006_create_activity_completions`. Rows are never updated except for the note, and
never soft-deleted.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `child_id` | `UUID` | no | | references `children(id)` `ON DELETE CASCADE` |
| `activity_id` | `UUID` | no | | references `activities(id)` `ON DELETE CASCADE` (activities are only soft-deleted, so this fires only on manual hard deletes) |
| `user_id` | `UUID` | yes | | parent who marked it, references `users(id)` `ON DELETE SET NULL` |
| `completed_on` | `DATE` | no | | calendar day in Uzbekistan time (UTC+5), computed by the application |
| `note` | `TEXT` | yes | | reflection, at most 1000 characters |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | first time it was marked that day |

Constraints:

- `activity_completions_note_len_chk`: `char_length(note) <= 1000`

Indexes:

- primary key on `id`
- `activity_completions_child_activity_day_uniq`, unique on `(child_id, activity_id,
  completed_on)`: an activity counts once per child per day. Inserts use
  `ON CONFLICT ... DO UPDATE` to return the existing row and replace the note.
- `activity_completions_child_day_idx` on `(child_id, completed_on DESC)`, for the streak and
  the history list

Repository (`store.Completion()`): `Create` (returns whether a new row was inserted), `List`
(one child, paged, newest day first) and `Days` (distinct days with a completion, newest
first, plus the total count). The streak itself is computed in Go by `domain.ComputeStreak`.

`childRepo.GetForParent(id, userID)` returns a child only when it is active and linked to that
user; every per-child use case calls it first.

## Conventions

- Primary keys are UUIDs generated by PostgreSQL.
- Every table has `created_at` and `updated_at` as `TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
  except append-only tables such as `media`, which only have `created_at`.
- Soft-deletable tables use a nullable `deleted_at TIMESTAMPTZ`; uniqueness on them is
  enforced with partial indexes `WHERE deleted_at IS NULL`.
- User-facing text is stored per language in `_uz` and `_ru` columns.
- Business invariants that are cheap to state in SQL also get a `CHECK` constraint, so bad
  data cannot enter even through manual SQL.
- Many-to-many relations use a join table named `<a>_<b>` with a composite primary key,
  `ON DELETE CASCADE` on both foreign keys and an index on the second column.
- Each migration has an `.up.sql` and a `.down.sql`; numbering is sequential with six digits.

## Redis keys

Short-lived state lives in Redis, not PostgreSQL (`internal/redisstore`). Every key starts
with `birga:` so the instance can be shared. Nothing in Redis needs a backup: losing it only
means users request a new code (and verify again before signing up).

| Key | Type | TTL | Holds |
|---|---|---|---|
| `birga:otp:code:<purpose>:<phone>` | hash `{hash, attempts}` | `OTP_TTL` (2 min) | SHA-256 of `<phone>:<purpose>:<code>` (`domain.HashOTPCode`) and wrong attempts so far; deleted on a match or after `OTP_MAX_VERIFY_ATTEMPTS` wrong codes |
| `birga:otp:cooldown:<purpose>:<phone>` | string | `OTP_RESEND_COOLDOWN` (1 min) | present while a new code may not be sent |
| `birga:otp:limit:phone:<phone>` | counter | 1 hour from the first code | codes sent to the phone, any purpose |
| `birga:otp:limit:ip:<ip>` | counter | 1 hour from the first code | codes requested from the IP (canonical form, e.g. `2001:db8::1`) |
| `birga:otp:verified:<purpose>:<phone>` | string `1` | `OTP_VERIFIED_TTL` (10 min) | set by the verify script when a code matches, or by `MarkVerified` for `OTP_DEFAULT_CODE`; `POST /v1/auth/signup` requires the `sign_up` one and deletes it after creating the user |

`<phone>` is E.164 (`+998901234567`), `<purpose>` is `sign_up` or `update_user`.

## Database roles (servers)

On a fresh server volume the devops stack creates extra roles: `birga_readonly` (read-only,
used by Metabase), `metabase` (its own database) and `exporter` (monitoring), and enables
`pg_stat_statements`. See [operations.md](operations.md).
