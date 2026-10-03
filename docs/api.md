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
| -10 | 422 | validation error | business rule broken, e.g. `unknown goal "flying"`, wrong OTP code, unsupported or too large upload |
| -11 | 400 | malformed request | bad JSON, non-integer `limit`, invalid UUID |
| -20 | 401 | unauthorized | missing or wrong `X-Admin-Key`; missing, invalid, expired or replaced access token; invalid, expired or revoked refresh token |
| -21 | 403 | forbidden | admin API disabled (no `ADMIN_API_KEY` configured); sign-up without a verified phone number |
| -30 | 404 | not found | unknown id, unpublished or soft-deleted activity on a public endpoint, soft-deleted user, a child that is not the caller's, no activity to recommend, OTP expired or not requested |
| -40 | 409 | conflict | uniqueness conflict, e.g. `username is already taken`, `phone number is already taken` |
| -50 | 500 | internal error | anything unexpected; details only in server logs |
| -60 | 503 | dependency unavailable | `/health` when the database or Redis is unreachable; `POST /v1/media` when S3 is not configured |
| -70 | 429 | rate limited | OTP requested again too soon, too many OTPs for a phone or IP, too many wrong OTP codes |

For 500s the client only sees `"internal error"`. Find the real message in the logs by the
request id.

## Common headers

| Header | Direction | Meaning |
|---|---|---|
| `X-Request-Id` | request (optional) and response | correlates a call with server log lines; generated if absent |
| `X-Admin-Key` | request | admin key for `/v1/admin/*` |
| `Authorization: Bearer <access_token>` | request | signed-in user for `/v1/children/*` |
| `Content-Type: application/json` | request | for bodies |

CORS allows any origin.

## Admin authentication

`/v1/admin/*` is protected by a single shared key, a stop-gap until real admin accounts exist.

- Send `X-Admin-Key: <ADMIN_API_KEY>`. The comparison is constant-time.
- Wrong or missing key: 401, code -20.
- `ADMIN_API_KEY` not set on the server: every admin call returns 403, code -21.

## User authentication

Endpoints for a signed-in parent (today `/v1/children/{id}/...`) need the access token from
`/v1/auth/signup` or `/v1/auth/refresh`:

- Send `Authorization: Bearer <access_token>`.
- The token must be validly signed, of type `access`, not expired, and still the latest access
  token issued to the user (each refresh replaces the stored one, and deleting the user removes
  it). Otherwise: 401, code -20. Refresh the token and retry.
- A child id the user is not linked to (through `user_children`) answers 404, the same as a
  child that does not exist, so other families' ids cannot be probed.

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
| GET | `/health` | readiness; pings the database and Redis, `data: "OK"` or 503 |
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
| PATCH | `/v1/admin/activities/{id}` | change some fields, including publishing or unpublishing |
| DELETE | `/v1/admin/activities/{id}` | soft-delete an activity |

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

Update (`PATCH`) takes the same fields, all optional. Omitted or `null` fields are kept. Each
given field follows the create rules (texts cannot be emptied); an age that is valid alone but
not together with the stored one, such as `min_age` above the stored `max_age`, is rejected
by the database check, also with 422. An empty body is 422 (`nothing to update`).
`updated_at` is set on every update.

```bash
# publish
curl -X PATCH localhost:8080/v1/admin/activities/$ID \
  -H 'X-Admin-Key: change-me' -d '{"is_published": true}'
```

Delete sets `deleted_at`. The activity then disappears from every list, get and
recommendation, and a second delete is 404. Completions that point at it are kept, so a
child's history and streak do not change. There is no undelete endpoint.

### Children: recommendation, completions, streak (signed in)

All of these need [user authentication](#user-authentication) and a child linked to the
caller. Days are calendar days in Uzbekistan time (UTC+5, `domain.Location`).

| Method | Path | Description |
|---|---|---|
| GET | `/v1/children/{id}/recommendation` | today's activity for the child |
| POST | `/v1/children/{id}/completions` | mark an activity done today, with an optional reflection note |
| GET | `/v1/children/{id}/completions` | the child's completions, newest first (paged) |
| GET | `/v1/children/{id}/streak` | streak and progress counters |

Child profiles have no endpoints yet, so a client cannot create or link a child through the
API. Until it can, create them with the `store.Child()` repository or SQL.

#### Recommendation

Query parameters, both optional:

| Param | Type | Rules |
|---|---|---|
| `goal` | string | one of the goals; 422 otherwise |
| `minutes` | int | time available; keeps activities with `duration_minutes <= minutes`; 400 if not an integer, 422 if negative |

Rules (`activityRepo.Recommend`), all in one query:

1. Candidates are published, non-deleted activities whose age range contains the child's age.
   The age is clamped to 2..6 first, so a 1-year-old gets 2-year activities and a 7-year-old
   gets 6-year ones.
2. Activities the child has never completed come first, then the one completed longest ago.
3. Ties are broken by `md5(activity id || child id || date)`, so the pick stays the same all
   day, changes the next day, and differs between children. Completing it moves it to the
   back, so asking again after a completion suggests the next activity.

No candidate: 404, `no published activity matches this child's age and the filters`.
Response: an [activity object](#activity-object).

```bash
curl -H "Authorization: Bearer $TOKEN" \
  'localhost:8080/v1/children/'$CHILD'/recommendation?goal=emotional&minutes=10'
```

#### Complete

```json
{"activity_id": "7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11", "note": "Qizil rangni birinchi topdi"}
```

- `activity_id` must be a UUID (422) of a published, non-deleted activity (404).
- `note` is the caregiver's optional reflection: trimmed, blank means none, at most 1000
  characters (422).
- The completion is for today. Sending the same activity again on the same day returns the
  existing completion, replacing its note when a new one is sent; it is never counted twice.

Response: a [completion object](#completion-object).

#### Streak

Response:

```json
{"current": 3, "longest": 7, "completed_today": true, "this_week": 2, "total": 12, "last_completed_on": "2026-10-03"}
```

| Field | Meaning |
|---|---|
| `current` | consecutive days with at least one completion, ending today; while today has none yet it ends yesterday, so the streak is only lost after a whole day is missed |
| `longest` | the longest such run ever |
| `completed_today` | at least one completion today |
| `this_week` | days with a completion since Monday (the concept favours weekly goals of 2 to 5 activities over daily perfection) |
| `total` | all completions |
| `last_completed_on` | date of the latest completion, `null` before the first |

Pausing or repairing a streak is not built yet.

### OTP (public)

One-time codes sent by SMS through Play Mobile, for registration and for changing account
data. The code lives in Redis between send and verify.

| Method | Path | Description |
|---|---|---|
| POST | `/v1/otp/send` | generate a 6-digit code and send it to a phone |
| POST | `/v1/otp/verify` | check a code; a matching code is deleted |

#### Send

Body (all fields required):

```json
{
  "phone_number": "+998901234567",
  "purpose": "sign_up",
  "ip_address": "203.0.113.7"
}
```

| Field | Rules |
|---|---|
| `phone_number` | Uzbek mobile number in E.164: `+998` and 9 digits (422). Play Mobile only delivers in Uzbekistan |
| `purpose` | `sign_up` (registration) or `update_user` (changing account data) (422) |
| `ip_address` | the end user's IPv4 or IPv6 address, as seen by the app or proxy in front of the API (422) |

Response `data`:

```json
{"expires_in": 120, "resend_in": 60}
```

`expires_in` is how many seconds the code stays valid (`OTP_TTL`, 2 minutes); `resend_in` is
how many seconds the client must wait before asking for another code (`OTP_RESEND_COOLDOWN`).

Behaviour (`usecases/otp_sender`):

- `sign_up`: the number must not belong to an active user, otherwise 409
  `phone number is already registered`. `update_user` does not check the users table.
- Rate limiter, checked before anything is sent, all returning 429 (code -70) with the
  number of seconds to wait in `error_note`:
  - one code per phone and purpose every `OTP_RESEND_COOLDOWN` (60 s): `a code was sent
    recently, request a new one in N seconds`;
  - at most `OTP_MAX_PER_PHONE_HOUR` (5) codes per phone per hour, both purposes together;
  - at most `OTP_MAX_PER_IP_HOUR` (20) codes per `ip_address` per hour.

  The hour is a fixed window that starts with the first code. The check and the counting are
  one atomic Redis script, so parallel requests and several API instances cannot slip past it.
- The code is 6 random digits (`crypto/rand`). Redis keeps only a SHA-256 hash for
  `OTP_TTL`; a new code for the same phone and purpose replaces the previous one and resets its
  attempts. The plain code exists only in the SMS: `Birga: tasdiqlash kodingiz 123456. Kodni
  hech kimga bermang.`
- If the SMS provider fails, the code is deleted and the send is not counted against the
  limits; the client gets 500.
- With `SMS_PROVIDER=log` (local development) nothing is sent; the SMS text, including the
  code, is written to the server log.

#### Verify

Body (all fields required):

```json
{"phone_number": "+998901234567", "purpose": "sign_up", "code": "480569"}
```

`phone_number` and `purpose` must be the ones the code was sent for; `code` is 6 digits.
A malformed field returns 422 before the code is looked up.

| Result | HTTP | error_code | error_note |
|---|---:|---:|---|
| code matches; it is deleted and cannot be used again, and the phone is marked verified for the purpose for `OTP_VERIFIED_TTL` (10 min) | 200 | 0 | `data` is `null` |
| wrong code | 422 | -10 | `wrong code, N attempts left` |
| wrong code and no attempts left (`OTP_MAX_VERIFY_ATTEMPTS`, 5); the code is deleted | 429 | -70 | `too many wrong codes, request a new one` |
| no code: never sent, expired, already used, or deleted after too many attempts | 404 | -30 | `code expired or was not requested, request a new one` |

The comparison runs as one Redis script, so two parallel requests cannot both use a code.
The verified mark is a Redis key (see [data-model.md](data-model.md#redis-keys)) rather than a
token in the response: `POST /v1/auth/signup` checks it for `sign_up`; nothing reads the
`update_user` mark yet.

Examples:

```bash
curl -X POST localhost:8080/v1/otp/send -H 'Content-Type: application/json' \
  -d '{"phone_number": "+998901234567", "purpose": "sign_up", "ip_address": "203.0.113.7"}'

curl -X POST localhost:8080/v1/otp/verify -H 'Content-Type: application/json' \
  -d '{"phone_number": "+998901234567", "purpose": "sign_up", "code": "480569"}'
```

### Auth (public)

Sign-up and access token refresh. Tokens are HS256 JWTs signed with `JWT_SECRET`.

| Method | Path | Description |
|---|---|---|
| POST | `/v1/auth/signup` | create a user whose phone number was verified, return access and refresh tokens |
| POST | `/v1/auth/refresh` | trade a refresh token for a new access token |

Token claims: `sub` is the user id, `typ` is `access` or `refresh`, plus `iss`
(`JWT_ISSUER`), `iat`, `exp` and a unique `jti`. An access token lives `JWT_ACCESS_TTL`
(15 min), a refresh token `JWT_REFRESH_TTL` (30 days). A token of one type is rejected where
the other is expected. Endpoints for signed-in users take the access token as
`Authorization: Bearer <access_token>`; see [User authentication](#user-authentication).

#### Sign up

Flow: `POST /v1/otp/send` and `POST /v1/otp/verify` with `purpose: sign_up`, then within
`OTP_VERIFIED_TTL` (10 min):

```json
{"name": "Dilnoza", "username": "dilnoza_k", "password": "s3cret-pass", "phone_number": "+998901234567"}
```

| Field | Rules |
|---|---|
| `name` | required, at most 100 characters, trimmed |
| `username` | 3 to 32 of `a-z`, `0-9`, `_`, `.`; trimmed and lowercased |
| `password` | 8 to 72 bytes (bcrypt reads at most 72); stored only as a bcrypt hash |
| `phone_number` | Uzbek number, `+998` and 9 digits; must be the verified one |

Response `data`:

```json
{"access_token": "eyJ...", "refresh_token": "eyJ...", "expires_in": 900}
```

`expires_in` is the access token lifetime in seconds.

Behaviour (`usecases/user_signup`):

| Result | HTTP | error_code | error_note |
|---|---:|---:|---|
| invalid field | 422 | -10 | which field and why |
| no `sign_up` verification for the phone (never verified, expired, or already used by a sign-up) | 403 | -21 | `phone number is not verified, verify a sign_up code with /v1/otp/verify first` |
| username or phone number belongs to an active user | 409 | -40 | `username is already taken` / `phone number is already taken` |

- The `users` row (name, username, phone) and the `user_auth` row (username, bcrypt password,
  SHA-256 hashes of both tokens) are written in one transaction.
- The verified mark is deleted only after the user is stored, so a sign-up that fails (for
  example a taken username) can be retried with another username without a new code.

#### Refresh

```json
{"refresh_token": "eyJ..."}
```

Response `data`: `{"access_token": "eyJ...", "expires_in": 900}`.

The refresh token must be signed with `JWT_SECRET`, unexpired, of type `refresh`, and its
SHA-256 must equal `user_auth.refresh_token` for the user in `sub`. Anything else, including a
soft-deleted user (their `user_auth` row is gone), is 401 (-20) `refresh token is invalid or
expired, sign in again`. The new access token's hash replaces `user_auth.access_token`. The
refresh token is not rotated; the client keeps it until it expires.

```bash
curl -X POST localhost:8080/v1/auth/signup -H 'Content-Type: application/json' \
  -d '{"name": "Dilnoza", "username": "dilnoza_k", "password": "s3cret-pass", "phone_number": "+998901234567"}'

curl -X POST localhost:8080/v1/auth/refresh -H 'Content-Type: application/json' \
  -d '{"refresh_token": "eyJ..."}'
```

### Media (public)

Uploads an image to AWS S3 and returns its id and URL. The app uploads a profile photo
here first, then saves the returned `id` as the user's `photo_id`.

| Method | Path | Description |
|---|---|---|
| POST | `/v1/media` | upload one image |

The body is form data with one field, `file`, in any of these forms:

- `multipart/form-data` with `file` as a file part (a normal file upload);
- `multipart/form-data` or `application/x-www-form-urlencoded` with `file` as a base64
  string. Standard or URL-safe base64, with or without padding, and an optional data URL
  prefix such as `data:image/jpeg;base64,` are all accepted. In a urlencoded body the value
  must be URL-encoded (every HTTP client does this for form data), otherwise `+` turns into
  a space.

Rules (`usecases/media_uploader`):

- the type is detected from the bytes, not from the file name or `Content-Type`: JPEG, PNG
  or WebP only, otherwise 422 `unsupported file type, upload a JPEG, PNG or WebP image`.
  HEIC from iPhones is not accepted; the app should convert to JPEG before upload;
- at most `MEDIA_MAX_SIZE` bytes after decoding (default 5 MiB), otherwise 422; an empty file
  is 422 too;
- a missing `file` field or invalid base64 is 400;
- when S3 is not configured (`S3_BUCKET` empty) the endpoint answers 503 `media uploads are
  disabled`; a failed S3 call is 500.

The object is stored as `<S3_KEY_PREFIX>media/<id>.<ext>`. Response `data`:

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | the media id; put it in `photo_id` |
| `url` | string | where clients load the image, `<S3_PUBLIC_BASE_URL>/<key>` |
| `content_type` | string | `image/jpeg`, `image/png` or `image/webp` |
| `size` | integer | bytes |

The endpoint has no user authentication yet (there are no user tokens); it should move
behind user auth once sign-in exists.

Examples:

```bash
# a file
curl -X POST localhost:8080/v1/media -F file=@photo.jpg

# base64 in form data
curl -X POST localhost:8080/v1/media --data-urlencode "file=$(base64 -i photo.jpg)"
```

```json
{
  "status": "Success",
  "error_code": 0,
  "error_note": "",
  "data": {
    "id": "3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f",
    "url": "https://birga-media.s3.eu-central-1.amazonaws.com/media/3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f.jpg",
    "content_type": "image/jpeg",
    "size": 183244
  }
}
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
- `photo_id`: a UUID (422) of an image uploaded with `POST /v1/media`; an id with no
  `media` row is 422 `photo_id is not an uploaded media id, ...`;
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
| `photo_id` | UUID string or `null` | profile photo, a media id from `POST /v1/media` |
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

Soft-deleted activities are never returned, so `deleted_at` is not part of the object.

### Completion object

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | |
| `child_id` | UUID string | |
| `activity_id` | UUID string | may point at an activity deleted since |
| `user_id` | UUID string or `null` | the parent who marked it; `null` if that user was hard-deleted |
| `completed_on` | date `YYYY-MM-DD` | day in Uzbekistan time |
| `note` | string or `null` | caregiver's reflection |
| `created_at` | RFC 3339 timestamp | first time it was marked that day |
