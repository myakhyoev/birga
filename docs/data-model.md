# Data model

PostgreSQL 17. The schema is defined only by the SQL files in `migrations/`, applied with
[golang-migrate](https://github.com/golang-migrate/migrate). The `schema_migrations` table
that golang-migrate creates records the current version.

## Tables

### `activities`

The curated catalogue of short offline activities. Created by
`000001_create_activities_table`; `deleted_at` added by `000007_create_activity_completions`;
`goal` replaced by `goal_ids` in `000011_link_activities_to_goals`.
Rows are soft-deleted by setting `deleted_at`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `title_uz` | `TEXT` | no | | Uzbek title |
| `title_ru` | `TEXT` | no | | Russian title |
| `title_en` | `TEXT` | no | `''` | English title (added by `000015_add_activity_english`; the starter activities get theirs from `000026`) |
| `description_uz` | `TEXT` | no | | Uzbek instructions |
| `description_ru` | `TEXT` | no | | Russian instructions |
| `description_en` | `TEXT` | no | `''` | English instructions (added by `000015_add_activity_english`) |
| `main_idea_uz` | `TEXT` | no | `''` | Uzbek "Katta g'oya" text, optional (added by `000018_add_activity_main_idea`) |
| `main_idea_ru` | `TEXT` | no | `''` | Russian main idea, optional (`000018`) |
| `main_idea_en` | `TEXT` | no | `''` | English main idea, optional (`000018`) |
| `easier_uz`, `easier_ru`, `easier_en` | `TEXT` | no | `''` | the "Osonroq" variant (Birga Pro), optional (added by `000021_activity_variants_today_extras`) |
| `harder_uz`, `harder_ru`, `harder_en` | `TEXT` | no | `''` | the "Qiyinroq" variant (Birga Pro), optional (`000021`) |
| `difficulty` | `TEXT` | no | `'normal'` | `easy`, `normal` or `difficult` (added by `000025_activity_difficulty`). Plans and goal picks take normal ones first; the "Osonroq / Odatdagi / Qiyinroq" switch swaps an activity for another of the chosen level |
| `photo_id` | `UUID` | yes | | the activity's picture, references `media(id)` `ON DELETE SET NULL` (added by `000027_content_photos`); `NULL` means none |
| `goal_ids` | `UUID[]` | no | `'{}'` | ids of the [`goals`](#goals) the activity serves; no foreign key, see below |
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
- `activities_difficulty_chk`: `difficulty IN ('easy', 'normal', 'difficult')` (`000025`)

Indexes:

- primary key on `id`
- `activities_published_goal_ids_idx`, GIN on `(goal_ids) WHERE is_published AND deleted_at IS NULL`,
  for the goal filter and the goal preference of the plan (`000011`; it replaces
  `activities_published_goal_idx` on the old `goal` column)

Like `children.goal_ids`, `goal_ids` cannot carry a foreign key. The admin API requires 1 to 5
active goal ids (`domain.MaxActivityGoals`, checked with `goalRepo.CountActive`), and deleting a
goal removes its id from every activity (see [`goals`](#goals)). The application additionally
caps `duration_minutes` at 60, which the database does not enforce.

Until `000011` an activity had one fixed text key (`language`, `motor`, `cognitive`, `social`,
`emotional`). The migration maps each key to a goal: it reuses an active goal that already has
one of the names below in any language (or whose English name equals the key), otherwise it
creates the goal, but only for keys some activity uses. The down migration restores the key
from the first goal by the same names, falling back to `cognitive`; goals it created stay.

| Old key | `name_uz` | `name_ru` | `name_en` |
|---|---|---|---|
| `language` | Nutq | Речь | Language |
| `motor` | Harakat | Моторика | Motor skills |
| `cognitive` | Fikrlash | Мышление | Thinking |
| `social` | Muloqot | Общение | Social skills |
| `emotional` | Hissiyotlar | Эмоции | Emotions |

Soft delete rather than a hard delete keeps `activity_completions` rows pointing at a real
activity, so history and streaks survive. Every repository read filters `deleted_at IS NULL`.

Repository (`store.Activity()`): `Create`, `Get`, `List` (filters and paging), `Update`
(partial; a failed CHECK becomes `ErrValidation`), `Delete` (soft) and `Recommend` (see
[api.md](api.md#recommendation)).

A starter set of the five goals above and 20 published activities (Uzbek and Russian, at least
seven for every age 2 to 6) is in `seeds/activities.sql`; load it with `make seed`
([setup.md](setup.md#starter-activities)). It is data, not a migration, and skips goals whose
English name and activities whose title already exist. It also fills the English texts and the
main idea (`main_idea_*`) of those activities where they are still empty.

Migration `000019_fill_activity_main_idea` writes the same main idea texts into databases that
were seeded before `000018` added the columns (the server's), matched by `title_uz`. It only
changes rows whose three `main_idea_*` fields are all empty, so texts an admin wrote are kept, and
it does not touch activities added through the dashboard. Its down migration does nothing.

Migration `000026_fill_activity_english` does the same for the English title and description
(`title_en`, `description_en`) of those 20 activities: the server's database was seeded before
`000015` added English, and the seed file only runs on an empty table, so they stayed `''`.
Matched by `title_uz`, it fills only the English fields that are still empty and leaves the
Uzbek and Russian texts, an admin's own English and dashboard-made activities alone. Its down
migration does nothing.

### `users`

App users (parents). Created by `000002_create_users_tables`. Rows are soft-deleted by
setting `deleted_at`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `name` | `TEXT` | yes | | display name |
| `username` | `TEXT` | yes | | unique among non-deleted users |
| `phone_number` | `VARCHAR(15)` | yes | | E.164 length limit; unique among non-deleted users |
| `photo_id` | `UUID` | yes | | id of the profile photo, references `media(id)` `ON DELETE SET NULL` (constraint `users_photo_id_fkey`, added by `000003_create_media_table`); NULL after sign-up, set through `PATCH /v1/me` |
| `relationship` | `user_relationship` | yes | | enum `father`, `mother`, `educator`, `nanny` (added by `000010_add_relationship_and_goals`); required by sign-up, NULL for older users and users created by an admin |
| `agreed_on_conditions` | `BOOLEAN` | no | `FALSE` | consent to the terms, the tick on the sign-up screen (added by `000012_add_user_consent_and_reminder`); FALSE for older users and users created by an admin |
| `reminder_enabled` | `BOOLEAN` | no | `FALSE` | daily reminder on or off (`000012`) |
| `reminder_time` | `VARCHAR(5)` | no | `'18:00'` | daily reminder time, `HH:MM` 24-hour, Uzbekistan time (`000012`); check `users_reminder_time_chk` |
| `pro_until` | `TIMESTAMPTZ` | yes | | Birga Pro given by date ends at this time (added by `000020_pro_topics_tips`); NULL means never given. Set by `PUT /v1/admin/users/{id}/pro`, which also turns `user_auth.role` `paid_user` into `user` when it ends Pro; read with the auth row on every request (`authRepo.Get`) |
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

### `goals`

Development goals a parent picks for each child, named in three languages. Created by
`000010_add_relationship_and_goals`. Rows are soft-deleted by setting `deleted_at`. Activities
point at goals through `activities.goal_ids`, children through `children.goal_ids`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `name_uz` | `TEXT` | no | | Uzbek name |
| `name_ru` | `TEXT` | no | | Russian name |
| `name_en` | `TEXT` | no | | English name |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | set by the application on update and soft delete (no trigger) |
| `deleted_at` | `TIMESTAMPTZ` | yes | | set on soft delete; `NULL` means active |

Indexes: `goals_name_uz_uniq`, `goals_name_ru_uniq`, `goals_name_en_uniq`, unique on
`(LOWER(name_xx)) WHERE deleted_at IS NULL`. A name is unique per language among active goals,
ignoring case; deleting a goal frees its names. `internal/dbstore/goal.go` maps a violation to
409 (`errs.ErrGoalNameTaken`).

`children.goal_ids` and `activities.goal_ids` point at goals, but Postgres cannot put a foreign
key on array elements, so:

- the child endpoints (add, edit) and the activity admin endpoints check that every id is an
  active goal (`goalRepo.CountActive`) before writing;
- `goalRepo.Delete` soft-deletes the goal and removes its id from every `children.goal_ids` and
  `activities.goal_ids` (`array_remove`) in one statement, so nothing keeps a deleted goal.
  `child_plan_days.goal_id` keeps it: a plan already made does not change.

Until `000014_child_goals_and_plans` the goals were the parent's (`users.goal_ids`, picked at
sign-up). That migration copied each child's first parent's goals to the child and dropped the
column.

Repository (`store.Goal()`): `Create`, `Get`, `List` (all active, oldest first), `CountActive`,
`Update` (partial), `Delete` (soft, see above).

### `user_auth`

Authentication state for a user, one row per user. Created by
`000002_create_users_tables`; `username` and `password` added by
`000004_add_user_auth_credentials`; `role` added by `000005_add_user_auth_role`, value `unverified_user` by `000009_add_unverified_user_role`. `POST /v1/auth/signup` inserts the row;
`PUT /v1/me/password` and `POST /v1/auth/forgot-password` replace `password`, `access_token` and `refresh_token`
together (`authRepo.SetCredentials`). `POST /v1/auth/login` replaces both token hashes and `POST /v1/auth/logout`
sets them to `NULL` (`authRepo.SetTokens`); login looks the row up by `username` (`authRepo.GetByUsername`).

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | | primary key and foreign key to `users(id)` |
| `fcm_token` | `TEXT[]` | no | `'{}'` | Firebase Cloud Messaging tokens, one per device |
| `access_token` | `TEXT` | yes | | SHA-256 (hex) of the newest access token (`domain.HashToken`) |
| `refresh_token` | `TEXT` | yes | | SHA-256 (hex) of the current refresh token; `POST /v1/auth/refresh` accepts only this one |
| `username` | `TEXT` | yes | | sign-in username, same value as `users.username` (kept in sync by `users_sync_auth_username_trg`) |
| `role` | `user_role` | no | `'user'` | enum `user`, `admin`, `paid_user`, `unverified_user`. Sign-up always sets `unverified_user` (the phone is not verified); the other roles are set only in the database for now. Copied into the tokens' `role` claim |
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
- `users_soft_delete_children_trg` and `users_delete_children_trg` on `users`: delete the
  user's children with them; see [`user_children`](#user_children).
- `users_sync_auth_username_trg` on `users` (function `users_sync_auth_username()`): when
  `users.username` changes (for example through `PATCH /v1/admin/users/:id`), the same value
  is written to `user_auth.username`.

### `media`

Files uploaded through `POST /v1/media` (profile photos) and `POST /v1/admin/media` (also daily videos). The bytes live in S3; this
table records what was uploaded. Created by `000003_create_media_table`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key. The API generates it before uploading, because it is part of the object key |
| `object_key` | `TEXT` | no | | S3 key, `<S3_KEY_PREFIX>media/<id>.<jpg\|png\|webp\|mp4\|webm>`; unique |
| `content_type` | `TEXT` | no | | `image/jpeg`, `image/png` or `image/webp`, or (admin uploads) `video/mp4` or `video/webm`, detected from the bytes |
| `size_bytes` | `BIGINT` | no | | file size; `CHECK (size_bytes > 0)` |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |

The table has no `updated_at` or `deleted_at`: a media row never changes, and a new photo
is a new row. The public URL is not stored; it is built from `object_key` and
`S3_PUBLIC_BASE_URL`, so the CDN or bucket address can change without a migration. The
object is uploaded first and the row inserted after; if the insert fails the object is
deleted again. Replaced photos are not cleaned up yet (no cleanup job).

### `children`

Child profiles. Created by `000006_create_children_tables`. Rows are soft-deleted by setting
`deleted_at`. A child belongs to its parents through `user_children`, not through a column.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `name` | `TEXT` | no | | display name |
| `age` | `DOUBLE PRECISION` | no | | age in years as a fraction (1 month = 1/12, so 18 months is 1.5), as entered; not advanced automatically. `SMALLINT` whole years until `000023_child_age_years_float`. The API speaks months; `dbstore/child.go` converts (`domain.ChildAgeToDB`/`ChildAgeFromDB`) |
| `gender` | `TEXT` | no | | `male` or `female` |
| `photo_id` | `UUID` | yes | | references `media(id)` `ON DELETE SET NULL` (constraint `children_photo_id_fkey`), same as `users.photo_id` |
| `goal_ids` | `UUID[]` | no | `'{}'` | ids of the [`goals`](#goals) picked for this child, in the order given (added by `000014_child_goals_and_plans`); no foreign key. The API requires 1 to 20 (`domain.MaxChildGoals`); empty only for a child whose goals were all deleted |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | set by the application on update and soft delete (no trigger) |
| `deleted_at` | `TIMESTAMPTZ` | yes | | set on soft delete; `NULL` means active |

Constraints:

- `children_age_chk`: `age BETWEEN 0 AND 18`, kept from `000006` for older rows. The API allows
  less: `domain.IsValidChildAgeMonths` takes 1 to 11 months, half years to 7, whole years to 12
  (`domain.MaxChildAgeMonths` = 144). Content matching uses whole years clamped to the 2 to 6
  activity range (`domain.MinChildAge`/`MaxChildAge`).
- `children_gender_chk`: `gender IN ('male', 'female')` (`domain.IsKnownGender`).

`internal/dbstore/child.go` reads only rows with `deleted_at IS NULL`, maps a failed check to
`errs.ErrValidation` and a `photo_id` that was never uploaded to `errs.ErrPhotoNotFound`.

### `user_children`

Many-to-many link between parents and children: a user can have several children and a
child several parents. Created by `000006_create_children_tables`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `user_id` | `UUID` | no | | references `users(id)` `ON DELETE CASCADE` |
| `child_id` | `UUID` | no | | references `children(id)` `ON DELETE CASCADE` |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | when the link was made |

Indexes:

- primary key on `(user_id, child_id)`, which also serves "children of a user"
- `user_children_child_id_idx` on `(child_id)`, for "parents of a child"

Links are not soft-deleted. Soft-deleting a child leaves its links in place; every read joins
through active rows only, so a deleted child is not listed for its parents.

Deleting a user deletes their children too, unless another parent still has the child
(migration `000008_cascade_user_children`):

- Soft delete (`users.deleted_at` set, `DELETE /v1/admin/users/{id}`): trigger
  `users_soft_delete_children_trg` (function `users_soft_delete_children()`) deletes the user's
  `user_children` rows, then soft-deletes every child they were linked to that has no link
  left. Those children's `activity_completions` stay, like the rest of a soft-deleted child.
  A shared child stays with its other parents.
- Hard delete (`DELETE FROM users`): trigger `users_delete_children_trg` (function
  `users_delete_children()`, `BEFORE DELETE`) deletes the children only this user is linked
  to; their completions go through `activity_completions.child_id ON DELETE CASCADE`. The
  user's remaining links go through `user_children.user_id ON DELETE CASCADE`.
- The migration applies the soft-delete rule once to users deleted before it ran.
- Restoring a user (`deleted_at` back to `NULL`) does not bring links or children back. The
  down migration drops the triggers and leaves deleted rows as they are.

Repository (`store.Child()`): `Create`, `Get`, `List` (optionally by `ParentID`, paged),
`Update` (partial, `PhotoID` `""` clears it), `Delete` (soft), `AddParent` (idempotent;
`ErrChildNotFound` or `ErrUserNotFound` when either side is missing or deleted),
`RemoveParent` (`ErrChildNotFound` when the link does not exist) and `Parents`. Creating a
child and linking it to its first parent are two calls; the `child_creator` use case
(`POST /v1/children`) runs them in `DBStore.InTx`.

### `activity_completions`

One row per activity a child did on a day, with the caregiver's optional reflection. Created
by `000007_create_activity_completions`. Rows are never updated except for the note, and
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

Repository (`store.Completion()`): `Create` (returns whether a new row was inserted) and `List`
(one child, paged, newest day first). A completion is history; only the plan days it marks done
(`child_plan_days.completed_on`) count toward the streak.

`childRepo.GetForParent(id, userID)` returns a child only when it is active and linked to that
user; every per-child use case calls it first.

### `child_plans`

One row per 7-day plan of a child. Created by `000014_child_goals_and_plans`, which also dropped
the fixed-theme plan of `000013_create_plans` (`plan_themes` and its `child_plan_days`); those
plans were not carried over, every child gets a new plan the next time it is asked for.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `child_id` | `UUID` | no | | references `children(id)` `ON DELETE CASCADE` |
| `number` | `INT` | no | | the child's plans counted from 1 (`child_plans_number_chk`); the highest is the current plan |
| `started_on` | `DATE` | no | | local day (Uzbekistan) the plan was made |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |

Unique `(child_id, number)` (`child_plans_child_number_uniq`). Plans are never deleted, so the
done days of earlier plans keep counting in the streak.

### `child_plan_days`

The seven days of a plan. Created by `000014_child_goals_and_plans`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `plan_id` | `UUID` | no | | references `child_plans(id)` `ON DELETE CASCADE` |
| `day` | `SMALLINT` | no | | 1 to 7 (`child_plan_days_day_chk`) |
| `goal_id` | `UUID` | yes | | the child's goal the day serves; NULL when the child had no goals. No foreign key, kept when the goal is deleted |
| `activity_id` | `UUID` | no | | references `activities(id)` `ON DELETE CASCADE` (only manual hard deletes) |
| `completed_on` | `DATE` | yes | | local day the day was done as part of the plan; NULL while not done |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | |

Primary key `(plan_id, day)`; partial index `child_plan_days_done_idx` on
`(plan_id, completed_on) WHERE completed_on IS NOT NULL` for the streak.

Rules kept by the application (`planRepo.MarkDone`, called by `POST .../completions` in the same
transaction as the completion, with the child row locked): only the first undone day of the
child's latest plan can be marked, only with that day's activity, and only when no plan day of
the child has `completed_on` on that day or later, so at most one plan day per calendar day. The
streak and the streak calendar are the distinct `completed_on` days of all the child's plans
(`domain.ComputeStreak`, `domain.StreakDays`). See
[api.md](api.md#children-7-day-plan-signed-in) for the statuses (`domain.BuildPlanDays`).

Repository (`store.Plan()`): `LockChild` (`SELECT ... FOR UPDATE` on the child, so a plan is made
once), `Current` (the latest plan with each day and whether its activity is still published),
`Create`, `Pick`, `SetActivity`, `MarkDone` and `DoneDays` (done days newest first, their count
and the first plan's start).

### `articles`

Short texts for parents (the "Kun maqolasi" card and screen). Created by
`000016_create_articles`. Rows are soft-deleted by setting `deleted_at`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `title_uz`, `title_ru`, `title_en` | `TEXT` | no | | required by the API |
| `article_body_uz`, `article_body_ru`, `article_body_en` | `TEXT` | no | | required by the API |
| `tip_uz`, `tip_ru`, `tip_en` | `TEXT` | no | `''` | optional "try it today" box; `''` means none |
| `photo_id` | `UUID` | yes | | references `media(id)` `ON DELETE SET NULL` |
| `expected_read_time` | `SMALLINT` | no | | minutes, `> 0` (`articles_read_time_chk`); the API caps it at 60 |
| `min_age`, `max_age` | `SMALLINT` | no | | `2 <= min_age <= max_age <= 6` (`articles_age_range_chk`) |
| `goal_id` | `UUID` | no | | references `goals(id)`; goals are only soft-deleted, so the row stays |
| `status` | `TEXT` | no | `'in_review'` | `published`, `in_review` or `unpublished` (`articles_status_chk`); only `published` reaches the app |
| `created_at`, `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `deleted_at` | `TIMESTAMPTZ` | yes | | soft delete |

Partial index `articles_published_goal_idx` on `goal_id WHERE status = 'published' AND deleted_at IS NULL`.

### `article_reads`

Articles a user marked read ("O‘qib chiqdim"). Created by `000016_create_articles`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `user_id` | `UUID` | no | | references `users(id)` `ON DELETE CASCADE` |
| `article_id` | `UUID` | no | | references `articles(id)` `ON DELETE CASCADE` |
| `read_at` | `TIMESTAMPTZ` | no | `NOW()` | first time it was marked |

Primary key `(user_id, article_id)`; marking it again keeps the first `read_at`.

### `user_daily_articles`

The daily article of each user, one row per local day (Uzbekistan). Created by
`000016_create_articles`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `user_id` | `UUID` | no | | references `users(id)` `ON DELETE CASCADE` |
| `day` | `DATE` | no | | local day the article is for |
| `article_id` | `UUID` | no | | references `articles(id)` `ON DELETE CASCADE` (only manual hard deletes) |
| `created_at` | `TIMESTAMPTZ` | no | `NOW()` | when it was picked |

Primary key `(user_id, day)`; index `user_daily_articles_article_idx` on `(user_id, article_id)`.
`GET /v1/me/articles/daily` writes today's row and tomorrow's the first time they are asked for,
under a per-user advisory lock (`articleRepo.LockDaily`), and replaces a row whose article is no
longer published. Rows up to today are the user's article history (`GET /v1/me/articles`). The
pick order is in [api.md](api.md#daily-article).

Repository (`store.Article()`): `Create`, `Get`, `Update`, `Delete`, `List`, `MarkRead`,
`LockDaily`, `DailyArticleID`, `PickDaily`, `SetDailyArticle`, `DailyHistory`.

### `posts`

Daily videos ("Kun videosi"). Created by `000017_create_posts`. Rows are soft-deleted by
setting `deleted_at`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `title_uz`, `title_ru`, `title_en` | `TEXT` | no | | required by the API |
| `video_id` | `UUID` | no | | references `media(id)`; the API accepts only `video/*` media |
| `photo_id` | `UUID` | yes | | cover picture, references `media(id)` `ON DELETE SET NULL` (`000027_content_photos`); `NULL` means none |
| `goal_id` | `UUID` | no | | references `goals(id)` |
| `min_age`, `max_age` | `SMALLINT` | no | | `2 <= min_age <= max_age <= 6` (`posts_age_range_chk`) |
| `duration` | `NUMERIC(6,2)` | no | | minutes, `> 0` (`posts_duration_chk`); the API caps it at 600 |
| `suggestion_uz`, `suggestion_ru`, `suggestion_en` | `TEXT` | no | `''` | optional "Tavsiya" line |
| `questions_uz`, `questions_ru`, `questions_en` | `TEXT` | no | `''` | optional questions to ask after watching, one per line |
| `status` | `TEXT` | no | `'in_review'` | `published`, `in_review` or `unpublished` (`posts_status_chk`) |
| `created_at`, `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `deleted_at` | `TIMESTAMPTZ` | yes | | soft delete |

Partial index `posts_published_goal_idx` on `goal_id WHERE status = 'published' AND deleted_at IS NULL`.

### `user_daily_posts`

The daily video of each user, one row per local day. Created by `000017_create_posts`. Same
shape and rules as [`user_daily_articles`](#user_daily_articles), with `post_id` referencing
`posts(id)` `ON DELETE CASCADE`; index `user_daily_posts_post_idx` on `(user_id, post_id)`. Only
today's row is written (the next videos are a preview). The rows are the user's video history
and decide which posts a free-plan user may open (`postRepo.WasDaily`).

The daily pick of articles and posts shares one query (`dbstore/daily.go`, `dailyPicks`).

Repository (`store.Post()`): `Create`, `Get`, `Update`, `Delete`, `List`, `WasDaily`,
`LockDaily`, `DailyPostID`, `PickDaily`, `SetDailyPost`, `DailyHistory`.

### `topics`

Tags of tips, articles and videos ("Ovqat", "Uyqu"). Created by `000020_pro_topics_tips`, which
also adds seven starter topics. Same columns and rules as [`goals`](#goals): `id`, `name_uz`,
`name_ru`, `name_en`, `created_at`, `updated_at`, `deleted_at`; partial unique indexes
`topics_name_{uz,ru,en}_uniq` on `LOWER(name_*) WHERE deleted_at IS NULL`. Soft delete only, so
foreign keys from tips, articles and posts stay valid.

`articles.topic_id` and `posts.topic_id` (`UUID`, nullable, references `topics(id)`) are the
optional topic tag, added by the same migration.

Repository (`store.Topic()`): `Create`, `Get`, `List`, `CountActive`, `Update`, `Delete`.

### `tips`

"Kun maslahati" tips. Created by `000020_pro_topics_tips`. Rows are soft-deleted by setting
`deleted_at`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `text_uz`, `text_ru`, `text_en` | `TEXT` | no | | required by the API, at most 500 characters |
| `topic_id` | `UUID` | no | | references `topics(id)` |
| `min_age`, `max_age` | `SMALLINT` | no | | `2 <= min_age <= max_age <= 6` (`tips_age_range_chk`) |
| `status` | `TEXT` | no | `'in_review'` | `published`, `in_review` or `unpublished` (`tips_status_chk`) |
| `created_at`, `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `deleted_at` | `TIMESTAMPTZ` | yes | | soft delete |

Partial index `tips_published_topic_idx` on `topic_id WHERE status = 'published' AND deleted_at IS NULL`.

### `user_daily_tips`

The daily tip of each user, one row per local day. Created by `000020_pro_topics_tips`. Same
shape as [`user_daily_articles`](#user_daily_articles) with `tip_id` referencing `tips(id)`
`ON DELETE CASCADE`; index `user_daily_tips_tip_idx` on `(user_id, tip_id)`. The rows also decide
which tips a free-plan user sees unlocked in `GET /v1/tips`. The pick uses `dailyPicks` with
`topicColumn = "topic_id"`: the days rotate through topics, and the children's goals are not
matched.

Repository (`store.Tip()`): `Create`, `Get`, `Update`, `Delete`, `List`, `LockDaily`,
`DailyTipID`, `PickDaily`, `SetDailyTip`.

### `milestones`

Development milestones ("Ko‘nikmalar"). Created by `000022_milestones`, which also seeds 57
published milestones (2 to 5 years, after the CDC list, in all three languages, advice empty).
Rows are soft-deleted by setting `deleted_at`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `UUID` | no | `gen_random_uuid()` | primary key |
| `domain` | `TEXT` | no | | `communication`, `social`, `thinking` or `movement` (`milestones_domain_chk`) |
| `age_months` | `SMALLINT` | no | | the stage, 2 to 72 (`milestones_age_chk`) |
| `sort_order` | `INTEGER` | no | `0` | order within a domain and stage |
| `text_uz`, `text_ru`, `text_en` | `TEXT` | no | | the milestone |
| `meaning_uz`, `meaning_ru`, `meaning_en` | `TEXT` | no | `''` | advice: "Bu nimani anglatadi" |
| `how_to_uz`, `how_to_ru`, `how_to_en` | `TEXT` | no | `''` | advice: "Qanday rivojlantirish", one step per line |
| `specialist_uz`, `specialist_ru`, `specialist_en` | `TEXT` | no | `''` | advice: when to see a specialist |
| `activity_ids` | `UUID[]` | no | `'{}'` | related activities; no foreign key, checked by the use case, only published ones are shown |
| `status` | `TEXT` | no | `'in_review'` | `published`, `in_review` or `unpublished` (`milestones_status_chk`) |
| `created_at`, `updated_at` | `TIMESTAMPTZ` | no | `NOW()` | |
| `deleted_at` | `TIMESTAMPTZ` | yes | | soft delete |

Index `milestones_published_age_idx` on `(age_months, domain, sort_order)` for published,
not-deleted rows.

### `child_milestones`

The milestones a parent marked for a child ("O‘zlashtirdi"). Created by `000022_milestones`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `child_id` | `UUID` | no | | `REFERENCES children(id) ON DELETE CASCADE` |
| `milestone_id` | `UUID` | no | | `REFERENCES milestones(id)` |
| `user_id` | `UUID` | yes | | the parent who marked it, `REFERENCES users(id) ON DELETE SET NULL` |
| `achieved_at` | `TIMESTAMPTZ` | no | `NOW()` | when it was marked; the monthly report counts by it |

Primary key `(child_id, milestone_id)`: marking again keeps the first row. Index
`child_milestones_child_achieved_idx` on `(child_id, achieved_at)`.

Repository (`store.Milestone()`): `Create`, `Get`, `Update`, `Delete`, `List`, `Stages`,
`ForChild`, `GetForChild`, `Mark`, `Unmark`, `CountAchieved`. `store.Activity().ListByIDs` loads
the related activities; `store.Completion().CountBetween` counts a month's completions.

### `child_day_extras`

Activities a Pro parent added to a child's list for one local day, next to the plan activity
("Bugungi reja", Figma 40a and 42). Created by `000021_activity_variants_today_extras`.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `child_id` | `UUID` | no | | `REFERENCES children(id) ON DELETE CASCADE` |
| `day` | `DATE` | no | | the local day (Uzbekistan time) |
| `activity_id` | `UUID` | no | | `REFERENCES activities(id)`; activities are only soft-deleted |
| `added_at` | `TIMESTAMPTZ` | no | `clock_timestamp()` | orders the list; `clock_timestamp()` so rows added in one transaction keep their order |

Primary key `(child_id, day, activity_id)`: an activity is on a day's list once. Whether an extra
is done comes from [`activity_completions`](#activity_completions) on the same day; extras never
mark a plan day done, so they do not count toward the streak. At most 10 a day, checked by the
use case.

Repository (`store.Extra()`): `ListDay`, `Add`, `Replace` (swaps one activity of the day for
another, used by the difficulty switch), `Pick` (a published activity for the child's age serving
one of the given goals, not in today's list; only one difficulty when asked, otherwise normal
first; then never done or done longest ago first).
`store.Activity().Similar` serves `GET /v1/activities/{id}/similar`.

### Sample content (`000024_sample_content`)

Not a table: the migration adds 10 published tips, 10 articles and 10 videos (posts) in Uzbek,
Russian and English (user request, 2026-10-25), so the library and the daily cards have
something to show. Real content is made in the dashboard; edit or delete the samples there.

- Ids are fixed (`5a1e0000-...` tips, `5a2e0000-...` articles, `5a3e0000-...` posts, ending in
  1 to 10), so running it twice adds nothing and the down migration deletes exactly these rows
  (their daily picks and reads go with them by cascade).
- Tips take their topic by English name (the topics of `000020`), else the oldest active topic.
- Articles and videos take their goal by English name (Language, Motor skills, Thinking, Social
  skills, Emotions), else the oldest active goal; with no goals they are skipped. Their topic is
  set by English name when such a topic exists.
- Videos need a video file, and a migration cannot upload one: all 10 reuse the oldest uploaded
  `media` row with a `video/*` content type. With no uploaded video they are skipped; upload one
  in the dashboard and run the migration again by hand
  (`psql "$POSTGRES_URL" -f migrations/000024_sample_content.up.sql`), or make the videos there.

### Seed photos (`seeds/photos`)

Not a migration: after the migrations, every deploy runs `birga_backend seed-photos seeds/photos`,
which uploads the starter pictures to S3 (one `media` row each) and sets `photo_id` on the
starter activities (matched by `title_uz`), the sample articles and the sample videos (matched
by id) that have no picture yet. A picture an admin chose is never replaced, and a row that
already has one is skipped, so running it again adds nothing. See
[operations.md, "Seed photos"](operations.md#seed-photos).

## Conventions

- Primary keys are UUIDs generated by PostgreSQL.
- Every table has `created_at` and `updated_at` as `TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
  except append-only tables such as `media`, which only have `created_at`.
- Soft-deletable tables use a nullable `deleted_at TIMESTAMPTZ`; uniqueness on them is
  enforced with partial indexes `WHERE deleted_at IS NULL`.
- User-facing text is stored per language in `_uz`, `_ru` and `_en` columns.
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
| `birga:otp:limit:client:<sha256>` | counter | 1 hour from the first code | codes requested with one `User-Agent` value; the key holds the SHA-256 hex of the header |
| `birga:otp:verified:<purpose>:<phone>` | string `1` | `OTP_VERIFIED_TTL` (10 min) | set by the verify script when a code matches, or by `MarkVerified` for `OTP_DEFAULT_CODE`; `PATCH /v1/me` requires the `update_user` one for a new phone number, `PUT /v1/me/password` and `POST /v1/auth/forgot-password` the `reset_password` one for the user's number; each deletes it after its write |

`<phone>` is E.164 (`+998901234567`), `<purpose>` is `sign_up`, `update_user` or `reset_password`.

## Database roles (servers)

On a fresh server volume the devops stack creates extra roles: `birga_readonly` (read-only,
used by Metabase), `metabase` (its own database) and `exporter` (monitoring), and enables
`pg_stat_statements`. See [operations.md](operations.md).
