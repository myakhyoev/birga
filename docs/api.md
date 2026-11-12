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
| -10 | 422 | validation error | business rule broken, e.g. `duration_minutes must be between 1 and 60`, wrong OTP code, unsupported or too large upload |
| -11 | 400 | malformed request | bad JSON, non-integer `limit`, invalid UUID |
| -20 | 401 | unauthorized | missing or wrong `X-Admin-Key`; missing, invalid, expired or replaced access token; invalid, expired or revoked refresh token; wrong username or password at login |
| -21 | 403 | forbidden | admin API disabled (no `ADMIN_API_KEY` configured and no admin token sent); access token of a role the endpoint does not allow; profile phone change or password reset without the matching OTP verification; a free-plan user opening a video or article that was not their daily one; a free-plan user calling a Pro feature (`this needs Birga Pro`) |
| -30 | 404 | not found | unknown id, unpublished or soft-deleted activity or article on an app endpoint, no published article or video for the daily article or video, soft-deleted user, a child that is not the caller's, no activity to recommend, no plan day open today, OTP expired or not requested, `reset_password` OTP for a number no user has |
| -40 | 409 | conflict | uniqueness conflict, e.g. `username is already taken`, `phone number is already taken`; starting the next plan before the current one is done |
| -50 | 500 | internal error | anything unexpected; details only in server logs |
| -60 | 503 | dependency unavailable | `/health` when the database or Redis is unreachable; `POST /v1/media` when S3 is not configured |
| -70 | 429 | rate limited | OTP requested again too soon, too many OTPs for a phone or a User-Agent, too many wrong OTP codes |

For 500s the client only sees `"internal error"`. Find the real message in the logs by the
request id.

Every error answers with this JSON envelope (`status`, `error_code`, `error_note`, `data: null`),
including the ones no handler produces:

- an unknown path: 404, `-30`, `no such endpoint: GET /v1/...`;
- a known path with the wrong method: 405, `-11`, `method DELETE is not allowed on /v1/...`;
- a panic in the server: 500, `-50`, `internal error`;
- nginx on the server: a body over 140 MB is 413, `-10`, `request body is too large, ...`; the
  API process down or not answering is 503, `-60`, `the API is not reachable right now, ...`.

The one exception is `GET /v1/media/{id}`, whose success is a 302 redirect.

## Common headers

| Header | Direction | Meaning |
|---|---|---|
| `X-Request-Id` | request (optional) and response | correlates a call with server log lines; generated if absent |
| `X-Admin-Key` | request | admin key for `/v1/admin/*` (when no `Authorization` header is sent) |
| `Authorization: Bearer <access_token>` | request | signed-in user for `/v1/children` and `/v1/children/*`; an admin user for `/v1/admin/*` |
| `Content-Type: application/json` | request | for bodies |
| `User-Agent` | request | sent by the apps on every request; required by `POST /v1/otp/send`, whose rate limit counts per value (see [Send](#send)) |

CORS allows any origin.

## Admin authentication

`/v1/admin/*` accepts either an admin user's access token or the shared admin key.

- With an `Authorization` header, the request is checked as [user authentication](#user-authentication)
  with role `admin` only: an invalid token is 401 (-20), a valid token of any other role
  is 403 (-21). `X-Admin-Key` is ignored then, so a valid key does not rescue a non-admin token.
- Without one, send `X-Admin-Key: <ADMIN_API_KEY>`, a stop-gap for scripts and seeding. The
  comparison is constant-time. Wrong or missing key: 401, code -20. `ADMIN_API_KEY` not set on
  the server: 403, code -21, so then only admin users get in.
- An admin is created by [sign-up](#sign-up) with `"role": "admin"` and the `X-Admin-Key`
  header; without the key sign-up only gives `unverified_user`. An existing user can also be
  made an admin by setting `user_auth.role = 'admin'` in the database. The new role applies at once, because the role
  is read from `user_auth` on every request, not from the token.

## User authentication

Endpoints for a signed-in parent (`/v1/me...`, `/v1/children` and `/v1/children/{id}...`) need the access
token from `/v1/auth/signup`, `/v1/auth/login`, `/v1/auth/forgot-password` or `/v1/auth/refresh`:

- Send `Authorization: Bearer <access_token>`.
- The token must be validly signed, of type `access`, not expired (it lives `JWT_ACCESS_TTL`,
  24 hours by default), and still the latest access token issued to the user (each refresh,
  login or password reset replaces the stored one; logout and deleting the user remove it). Otherwise: 401, code -20.
  Refresh the token and retry.
- A route can be limited to some roles (`unverified_user`, `user`, `paid_user`, `admin`). The role checked is the
  user's current `user_auth.role`, not the token's `role` claim. A role outside the list: 403,
  code -21, `your role cannot use this endpoint`. Today `POST /v1/children` lists all four
  roles, `/v1/me...` and `/v1/children/{id}...` take any role, and `/v1/admin/*` allows only `admin`.
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
| `goal_id` | UUID string | a goal id from [`GET /v1/goals`](#goals); matches activities whose `goal_ids` contain it |
| `q` | string | title search: matches activities whose `title_uz`, `title_ru` or `title_en` contains the text, ignoring case; trimmed, at most 100 characters; `%` and `_` match literally |
| `max_minutes` | int | only activities of at most this many minutes (the "10 daqiqagacha" filter) |
| `difficulty` | string | only activities of this level: `easy`, `normal` or `difficult` |
| `limit`, `offset` | int | see [Pagination](#pagination) |

An `age` outside 2..6, a `goal_id` that is not a UUID, a negative `max_minutes`, a `q` over 100
characters or another `difficulty` returns 422; a non-integer `age` or `max_minutes` returns 400. A UUID that is not a goal simply matches nothing.

Example:

```bash
curl 'localhost:8080/v1/activities?age=4&goal_id='$GOAL'&limit=10'
```

### Activities (admin)

| Method | Path | Description |
|---|---|---|
| POST | `/v1/admin/activities` | create an activity |
| GET | `/v1/admin/activities` | list all activities, including unpublished (same filters, plus `status`) |
| GET | `/v1/admin/activities/{id}` | one activity, including unpublished |
| PATCH | `/v1/admin/activities/{id}` | change some fields, including publishing or unpublishing |
| DELETE | `/v1/admin/activities/{id}` | soft-delete an activity |

The admin list takes the public list's `age`, `goal_id`, `q`, `difficulty`, `limit` and `offset`, and also
`status`: `published` (only published activities), `draft` (only unpublished ones) or omitted
for both. Any other value is 422 `status must be published or draft`. It also takes `source`
(`test`, `admin`, `harvard` or `ai`; see [Content source](#content-source)); another value is 422
`source must be test, admin, harvard or ai`. The public list ignores `status` and `source`.

#### Content source

Activities, articles and videos (posts) each have a `source` says where the content comes from: `test` (sample content made for testing), `admin`
(written in the dashboard), `harvard` (taken from Harvard material) or `ai` (generated with AI). Admin create takes it (case and spaces ignored; omitted or `""` means `admin`),
PATCH changes it, and the admin lists filter on it with `?source=`. Any other value is 422
`source must be test, admin, harvard or ai`. Everything stored before the field existed
(migration `000028`) is `test`. Every activity, article and post object returns `source`; the
app may ignore it.

```bash
curl 'localhost:8080/v1/admin/activities?q=tosh&status=draft' -H "Authorization: Bearer $ADMIN_TOKEN"
```

Create body:

```json
{
  "title_uz": "Rangli toshlar",
  "title_ru": "Цветные камни",
  "title_en": "Colourful stones",
  "description_uz": "Toshlarni rangi bo'yicha saralang",
  "description_ru": "Сортируйте камни по цвету",
  "description_en": "Sort the stones by colour",
  "main_idea_uz": "Bola ranglarni nomlab, ularni farqlashni o'rganadi.",
  "main_idea_ru": "Называя цвета, ребёнок учится их различать.",
  "main_idea_en": "Naming colours helps the child tell them apart.",
  "goal_ids": ["2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"],
  "min_age": 3,
  "max_age": 5,
  "duration_minutes": 10,
  "difficulty": "normal",
  "source": "admin",
  "photo_id": "5d2f8a90-1c3b-4e6f-8a7d-9b0c1d2e3f4a",
  "is_published": true
}
```

Validation (`usecases/activity_creator`), all returning 422:

- titles and descriptions are required in all three languages, Uzbek, Russian and English (whitespace is trimmed first);
- `main_idea_uz`, `main_idea_ru`, `main_idea_en` are optional (trimmed, default `""`);
- `easier_uz/ru/en` and `harder_uz/ru/en` are optional (trimmed, default `""`): the "Osonroq" and
  "Qiyinroq" variants ("Odatdagi" is the description);
- `difficulty` is `easy`, `normal` or `difficult` (case and spaces ignored); omitted or `""` means
  `normal`. Plans give children normal activities; the easy and difficult ones are what
  [the difficulty switch](#difficulty-switch) swaps to, so each goal and age needs some of each;
- `source` is `test`, `admin`, `harvard` or `ai` ([Content source](#content-source)); omitted or `""` means `admin`;
- `goal_ids` must hold 1 to 5 UUIDs (repeats are stored once), and each must be an active goal
  from [`GET /v1/goals`](#goals) (`goals contains ids that are not active goals, ...`);
- `photo_id` is optional: the activity's picture, an image uploaded with
  [`POST /v1/admin/media`](#media-public) (or `POST /v1/media`); not a UUID or not an uploaded
  media id is 422 (`photo_id is not an uploaded media id, ...`);
- `2 <= min_age <= max_age <= 6`;
- `1 <= duration_minutes <= 60`.

Example:

```bash
curl -X POST localhost:8080/v1/admin/activities \
  -H 'X-Admin-Key: change-me' -H 'Content-Type: application/json' \
  -d @activity.json
```

Update (`PATCH`) takes the same fields, all optional. Omitted or `null` fields are kept. Each
given field follows the create rules (titles and descriptions cannot be emptied, `main_idea_*`, `easier_*` and `harder_*` can, `difficulty` must be one of the three levels, `source` one of the four sources; `photo_id` `""` removes the picture; `goal_ids`, when sent, replaces the
whole list and cannot be emptied); an age that is valid alone but
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

### Articles (admin)

Short texts for parents, managed from the dashboard. Same auth as the other admin endpoints.

| Method | Path | Description |
|---|---|---|
| POST | `/v1/admin/articles` | create an article |
| GET | `/v1/admin/articles` | list articles in every status |
| GET | `/v1/admin/articles/{id}` | one article in any status |
| PATCH | `/v1/admin/articles/{id}` | change some fields, including the status |
| DELETE | `/v1/admin/articles/{id}` | soft-delete an article |

List query parameters: `q` (title search in any language, ignoring case, at most 100
characters), `status` (`published`, `in_review` or `unpublished`; omitted lists all),
`source` (`test`, `admin`, `harvard` or `ai`; omitted lists all), `goal_id`, `topic_id`, `age` (2..6, matches `min_age <= age <= max_age`), `limit`, `offset`. Newest first.

Create body:

```json
{
  "title_uz": "Nega bola «yo‘q» deydi?",
  "title_ru": "Почему ребёнок говорит «нет»?",
  "title_en": "Why does a child say \"no\"?",
  "article_body_uz": "2–5 yoshda «yo‘q» deyish rivojlanishning odatiy bosqichi. ...",
  "article_body_ru": "В 2–5 лет «нет» — обычный этап развития. ...",
  "article_body_en": "Saying \"no\" at 2–5 is a normal stage of development. ...",
  "tip_uz": "Bugun kamida 3 marta bolaga ikki variantli tanlov bering.",
  "tip_ru": "Сегодня хотя бы 3 раза дайте ребёнку выбор из двух вариантов.",
  "tip_en": "Today, offer your child a choice of two options at least 3 times.",
  "photo_id": null,
  "expected_read_time": 3,
  "min_age": 2,
  "max_age": 5,
  "goal_id": "2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00",
  "status": "in_review",
  "source": "admin"
}
```

Validation (`usecases/article_creator`), all returning 422:

- `title_*` and `article_body_*` are required in Uzbek, Russian and English (trimmed first);
  `tip_*` is optional, `""` means the app shows no tip box ("Bugun sinab ko‘ring");
- `goal_id` must be an active goal from [`GET /v1/goals`](#goals);
- `photo_id` is optional, an id from [`POST /v1/media`](#media-public);
- `1 <= expected_read_time <= 60` (minutes);
- `2 <= min_age <= max_age <= 6`;
- `status` is `published`, `in_review` or `unpublished`; omitted means `in_review`. Only
  `published` articles reach the app;
- `source` is `test`, `admin`, `harvard` or `ai` ([Content source](#content-source)); omitted means `admin`.

Update (`PATCH`) takes the same fields, all optional; omitted or `null` fields are kept. Titles
and bodies cannot be emptied, `tip_*: ""` removes the tip and `photo_id: ""` removes the photo.
An age valid alone but not with the stored one is 422. An empty body is 422 (`nothing to update`).

```bash
# publish
curl -X PATCH localhost:8080/v1/admin/articles/$ID \
  -H "Authorization: Bearer $ADMIN_TOKEN" -d '{"status": "published"}'
```

Delete sets `deleted_at`: the article disappears from every list, get, daily article and article
history. A second delete is 404.

### Articles (signed in)

All need `Authorization: Bearer <access token>` (any role) and only show `published` articles.
Every article here carries `is_read` for the signed-in user.

| Method | Path | Description |
|---|---|---|
| GET | `/v1/me/articles/daily` | today's article for this user and tomorrow's ("Kun maqolasi" card on the home screen) |
| GET | `/v1/me/articles` | this user's daily articles up to today, newest first (`limit`, `offset`) |
| GET | `/v1/articles` | published articles: `q`, `goal_id`, `topic_id`, `age`, `max_minutes` (at most this `expected_read_time`), `limit`, `offset` |
| GET | `/v1/articles/{id}` | one published article; others are 404; see the free-plan rule below |
| POST | `/v1/articles/{id}/read` | mark it read ("O‘qib chiqdim"); calling it again changes nothing |

#### Daily article

```json
{
  "day": "2026-10-19",
  "article": { "...article object...": "", "is_read": false },
  "tomorrow": { "id": "...", "title_uz": "Uyqudan oldingi tinch tartib", "title_ru": "...", "title_en": "...", "expected_read_time": 4 }
}
```

`day` is today in Uzbekistan time. `tomorrow` is the article that will be today's article
tomorrow ("Ertangi maqola: ..."), or `null` when no other article is published. 404 (`no
published article yet`) when no article is published at all.

How it is picked (`usecases/daily_article`, `articleRepo.PickDaily`): the first time a day is
asked for, one article is chosen and stored in `user_daily_articles`, so it stays the same all
day and the stored rows are the user's history (one user gets many articles over time). The
next day's article is chosen and stored at the same time. Among published articles it prefers,
in this order:

1. an article whose `goal_id` is one of the goals of the user's children;
2. whose age range fits at least one of the user's children;
3. not shown to this user as a daily article before and not marked read;
4. then the one shown longest ago, so once every matching article was shown they repeat;
5. then the goal shown longest ago, so the days rotate through the children's goals;
6. then the oldest article.

So articles that fit a goal and an age always come first, even when they repeat; only when none
exists does it fall back to a goal fit with another age, then to any published article. A stored
pick that was unpublished or deleted since is replaced on the next call. A user with no children
or no goals gets articles in the order of rules 3 to 6.

#### One article a day on the free plan

Users without [Birga Pro](#birga-pro) can open with `GET /v1/articles/{id}` only articles that
were their daily article, today or earlier (tomorrow's opens tomorrow); any other article is 403
(-21) `this article was not your daily article, the free plan opens one article a day`. In
`GET /v1/articles` those other articles come back with `locked: true` and empty
`article_body_*` and `tip_*` (title, photo, tags and read time stay, for the locked card). Pro
users open every published article and always get `locked: false`.

### Daily videos: posts (admin)

"Kun videosi": short videos for parents and children, managed from the dashboard. A post holds
an uploaded video. Same auth as the other admin endpoints.

| Method | Path | Description |
|---|---|---|
| POST | `/v1/admin/media` | upload the video (or an image): same form as [`POST /v1/media`](#media-public), also accepts MP4 and WebM |
| POST | `/v1/admin/posts` | create a post |
| GET | `/v1/admin/posts` | list posts in every status (`q`, `status`, `source`, `goal_id`, `topic_id`, `age`, `limit`, `offset`, like articles) |
| GET | `/v1/admin/posts/{id}` | one post in any status |
| PATCH | `/v1/admin/posts/{id}` | change some fields, including the status |
| DELETE | `/v1/admin/posts/{id}` | soft-delete a post (the video file is kept) |

Create body:

```json
{
  "title_uz": "Ranglar qo‘shig‘i",
  "title_ru": "Песня о цветах",
  "title_en": "The colours song",
  "video_id": "3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f",
  "goal_id": "2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00",
  "min_age": 3,
  "max_age": 5,
  "duration": 4.2,
  "suggestion_uz": "2–4 yoshda ekran oldida kuniga 1 soatdan kam vaqt. Videoni birga ko‘ring.",
  "suggestion_ru": "...", "suggestion_en": "...",
  "questions_uz": "«Qaysi rang senga eng ko‘p yoqdi?»\n«Uyimizda qizil rangli nima bor?»",
  "questions_ru": "...", "questions_en": "...",
  "status": "in_review",
  "source": "admin"
}
```

Validation (`usecases/post_creator`), all returning 422:

- `title_*` is required in Uzbek, Russian and English; `suggestion_*` (the "Tavsiya" line) and
  `questions_*` (the "Ko‘rgandan keyin so‘rang" box, one question per line) are optional, `""`
  hides them;
- `video_id` must be an MP4 or WebM video uploaded with `POST /v1/admin/media` (an image or an
  unknown id is `video_id is not an uploaded video, ...`);
- `photo_id` is optional: the cover picture, an image uploaded with `POST /v1/admin/media`
  (unknown: 422 `photo_id is not an uploaded media id, ...`);
- `goal_id` must be an active goal;
- `duration` is in minutes, a decimal (`4.2` is 4:12), more than 0 and at most 600;
- `2 <= min_age <= max_age <= 6`;
- `status` as for articles; omitted means `in_review`;
- `source` as for articles; omitted means `admin`.

Update (`PATCH`) takes the same fields, all optional, with the same rules; titles cannot be
emptied, `suggestion_*`/`questions_*: ""` removes them, `photo_id: ""` removes the cover. Delete sets `deleted_at`: the post
disappears from every list, get, daily video and history.

### Daily videos: posts (signed in)

All need `Authorization: Bearer <access token>` and only show `published` posts.

| Method | Path | Description |
|---|---|---|
| GET | `/v1/me/posts/daily` | today's video for this user ("Kun videosi" card) and the next ones, locked |
| GET | `/v1/me/posts` | this user's daily videos up to today, newest first (`limit`, `offset`) |
| GET | `/v1/posts` | published posts as previews without the video (`q`, `goal_id`, `topic_id`, `age`, `max_minutes`, `limit`, `offset`) |
| GET | `/v1/posts/{id}` | one published post with its video, see the rule below |

```json
{
  "day": "2026-10-19",
  "post": { "...post object...": "" },
  "next": [ { "id": "...", "title_uz": "Hayvonlar ovozi", "title_ru": "...", "title_en": "...", "goal_id": "...", "min_age": 3, "max_age": 5, "duration": 3.67 } ]
}
```

The daily video is picked once per day (Uzbekistan time), stored in `user_daily_posts` and
stays the same all day, by the same order as the [daily article](#daily-article) (goal of the
user's children, age fit, not shown before, the one shown longest ago, goal rotation, oldest
first). `next` lists up to 2 videos that would come next; they are a preview, not stored, and
have no video, for the locked "Keyingi videolar" list. 404 (`no published video yet`) when no
post is published.

One video a day on the free plan: users without [Birga Pro](#birga-pro) can open with
`GET /v1/posts/{id}` only posts that were their daily video, today or earlier; any other post is
403 (`this video was not your daily video, ...`). Pro users (including roles `paid_user` and
`admin`) can open every published post. For free users the previews say so: `locked: true` on
the `GET /v1/posts` items that were never their daily video and on every `next` video; for Pro
users `locked` is always `false`. `max_minutes` keeps videos of at most that `duration`. The video plays from `video_url` (`/v1/media/{video_id}`, a 302 to a signed S3
link, no token needed; video players follow the redirect and can seek).

### Birga Pro

Free and Pro plans (Figma 37-40). A user has Pro when their role is `paid_user` or `admin`, or
while `users.pro_until` is in the future. The access token check reads both on every request,
so a change applies at once. There is no real payment yet: the "To‘lash"
button calls `POST /v1/pro/purchase`, which turns Pro on for a month without charging; an admin
can also give Pro by date.

| Method | Path | Description |
|---|---|---|
| GET | `/v1/pro` | the offer screen: price and the caller's plan (signed in, any role) |
| POST | `/v1/pro/purchase` | the pay button: one more month of Pro, no real payment yet (signed in) |
| PUT | `/v1/admin/users/{id}/pro` | set or clear `pro_until` (admin) |

`GET /v1/pro` response:

```json
{"price_uzs": 16000, "period": "month", "plan": "free", "pro_until": null}
```

`price_uzs` comes from `PRO_PRICE_UZS` (default 16000).

`POST /v1/pro/purchase` body: `{"payment_method": "payme"}` (`payme` or `click`, case-insensitive;
anything else is 422). Nothing is charged yet: the call adds one month to `pro_until`, counted from
the current `pro_until` when that is still in the future, else from now, and answers

```json
{"payment_method": "payme", "amount_uzs": 16000, "paid": false, "plan": "pro",
 "pro_until": "2026-11-25T13:20:00Z"}
```

`paid` stays `false` until Payme and Click are connected; the app can show the result as a
successful payment. Real payments are on the project to-do list; the endpoint and its response
stay the same then, only `paid` becomes `true` after a confirmed payment.

`PUT /v1/admin/users/{id}/pro` body: `{"until": "2026-11-25T00:00:00+05:00"}` (RFC 3339) gives Pro
until then; `{"until": null}` or a past date ends it. Ending Pro also turns role `paid_user` into
`user`, so nothing keeps the user Pro; an `admin` stays Pro. Response: the [user object](#user-object)
(its `role` and `plan` show the result). 404 for an unknown or deleted user.

Removing Pro by username (dashboard only): find the user with `GET /v1/admin/users?q=<username>`
(an exact username match is the first item), then send `{"until": null}` for that user's `id`.

What the free plan allows, and what Pro opens:

| | Free | Pro |
|---|---|---|
| activities | the child's plan activity of the day; no added activities in today's list, no similar activities; `POST /v1/children/{id}/completions` for any other activity is 403 `this needs Birga Pro` (repeating today's completion to change its note still works) | any activity |
| children | one: a second `POST /v1/children` is 403 `this needs Birga Pro: the free plan has 1 child` | any number |
| articles | the daily article, today or earlier; others `locked` and 403 ([rule](#one-article-a-day-on-the-free-plan)) | every published article |
| videos | the daily video, today or earlier; others `locked` and 403 | every published post |
| tips | the daily tip; others `locked` with empty texts | every tip |
| milestones | the list and marks; advice is empty (`advice_locked`), no monthly report | advice, related activities, monthly report |

Free-plan users get 403 (-21) `this needs Birga Pro` on Pro-only actions. Children added while a
user had Pro stay after Pro ends; only adding more is blocked.

### Topics

Tags for tips, articles and videos whose subject is not a goal ("Ovqat", "Uyqu", "Ekran
vaqti"). Same shape and name rules as [goals](#goals): `name_uz`, `name_ru`, `name_en` required,
at most 100 characters, each unique among active topics ignoring case (409 otherwise).
Migration `000020` adds Ovqat, Uyqu, Nutq, His-tuyg‘ular, Ekran vaqti, Xulq-atvor and O‘yin.

| Method | Path | Description |
|---|---|---|
| GET | `/v1/topics` | active topics, oldest first, not paged (no token) |
| POST | `/v1/admin/topics` | create a topic |
| PATCH | `/v1/admin/topics/{id}` | rename it (names present change) |
| DELETE | `/v1/admin/topics/{id}` | soft-delete it; tagged content keeps the id |

Response: a topic object `{id, name_uz, name_ru, name_en, created_at, updated_at}`.

### Tips ("Kun maslahati")

One- or two-sentence advice for parents: the "Kun maslahati" card on the home screen and the
Kutubxona > Maslahat tab.

| Method | Path | Description |
|---|---|---|
| POST | `/v1/admin/tips` | create a tip |
| GET | `/v1/admin/tips` | tips in every status: `q`, `status`, `topic_id`, `age`, `limit`, `offset` |
| GET | `/v1/admin/tips/{id}` | one tip in any status |
| PATCH | `/v1/admin/tips/{id}` | change some fields, including the status |
| DELETE | `/v1/admin/tips/{id}` | soft-delete a tip |
| GET | `/v1/me/tips/daily` | today's tip for the signed-in user |
| GET | `/v1/tips` | published tips: `q`, `topic_id`, `age`, `limit`, `offset` (signed in) |

Create body:

```json
{"text_uz": "Ovqat paytida telefonni chetga qo‘ying.", "text_ru": "Во время еды отложите телефон.",
 "text_en": "Put the phone away at mealtimes.", "topic_id": "9a1c7e52-3b4d-4f6a-8e2b-7c5d1f0a9b3e",
 "min_age": 2, "max_age": 6, "status": "published"}
```

- `text_uz`, `text_ru`, `text_en` are required, trimmed, at most 500 characters each (422).
- `topic_id` is required and must be an active [topic](#topics) (422 `topic_id is not an active topic ...`).
  Unlike articles and posts a tip always has a topic (the daily tip rotates by topic), so on
  `PATCH` it can be changed to another topic but not removed: `""` is 422.
- `min_age`/`max_age` within 2..6, `min_age <= max_age` (422).
- `status` is `published`, `in_review` (the default) or `unpublished`; only `published` reaches the app.
- `q` searches the texts in every language, ignoring case (at most 100 characters).

Daily tip: `{"day": "2026-10-25", "tip": {tip object}}`. The tip is picked the first time the
day is asked for, stored in `user_daily_tips` and stays the same all day. Among published tips it
prefers one whose age range fits one of the user's children, then one never shown to the user,
then the one shown longest ago, then the topic shown longest ago (so the days rotate through the
topics), then the oldest. A stored pick that was unpublished or deleted since is replaced. 404
`no published tip yet` when no tip is published.

`GET /v1/tips` for a user without Pro: tips that were never the user's daily tip come back with
`locked: true` and empty texts, so the app can show them as locked cards. Pro users get every
text and `locked: false`.

Tip object: `id`, `text_uz`, `text_ru`, `text_en`, `topic_id`, `min_age`, `max_age`, `status`,
`created_at`, `updated_at`, `locked` (only `true` on `GET /v1/tips`, see above).

### Goals

Development goals a parent picks for each child (for example "Speech development"), named in
Uzbek, Russian and English. A child has its own goals (`goal_ids` on the
[child](#children-add-a-child-signed-in)), activities point at the goals they serve (`goal_ids`),
and the child's [7-day plan](#children-7-day-plan-signed-in) rotates through the child's goals.

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/goals` | none | every active goal, oldest first, not paginated |
| GET | `/v1/goals/{id}` | none | one goal; a deleted one is 404 |
| POST | `/v1/admin/goals` | [admin](#admin-authentication) | create a goal |
| PATCH | `/v1/admin/goals/{id}` | admin | rename a goal |
| DELETE | `/v1/admin/goals/{id}` | admin | soft-delete a goal |

Reading is open without a token, so the app can show the list before the user has one. List response `data`: `{"items": [goal, ...], "total": 6}`; see
[Goal object](#goal-object).

Create body:

```json
{"name_uz": "Nutqni rivojlantirish", "name_ru": "Развитие речи", "name_en": "Speech development"}
```

Validation (`usecases/goal_creator`, `usecases/goal_updater`):

| Result | HTTP | error_code |
|---|---:|---:|
| a name missing, blank, or over 100 characters (names are trimmed) | 422 | -10 |
| a name already used by another active goal in the same language, ignoring case | 409 | -40 |
| PATCH with no field (`nothing to update`) | 422 | -10 |

PATCH takes the same fields, all optional; omitted or `null` fields are kept.

Delete sets `deleted_at`: the goal leaves `/v1/goals`, children can no longer pick it, and its id
is removed from every child's and every activity's `goal_ids` in the same statement. Plan days
that already serve it keep its id. Its names can be
used again. An activity whose only goal is deleted keeps working but no longer matches any goal
filter or preference until an admin gives it a new goal. A
second delete is 404; there is no undelete.

```bash
curl -X POST localhost:8080/v1/admin/goals \
  -H 'X-Admin-Key: change-me' -H 'Content-Type: application/json' \
  -d '{"name_uz":"Nutqni rivojlantirish","name_ru":"Развитие речи","name_en":"Speech development"}'
```

### Profile (signed in)

The signed-in user's own account, for the app's profile page. All of these need
[user authentication](#user-authentication) and work for every role.

| Method | Path | Description |
|---|---|---|
| GET | `/v1/me` | the caller's profile |
| PATCH | `/v1/me` | edit the profile partially |
| PUT | `/v1/me/password` | set a new password, confirmed by an SMS code |
| DELETE | `/v1/me` | delete the account (soft delete) |
| GET | `/v1/me/children` | the caller's children, newest first (paged) |

#### Get profile

Response `data` is the [user object](#user-object) plus `role` (`unverified_user`, `user`, `paid_user` or `admin`,
the current `user_auth.role`). Here `plan` counts every way to have [Birga Pro](#birga-pro),
including the role.

#### Edit profile

Body: any of `name`, `username`, `phone_number`, `photo_id`, `agreed_on_conditions`,
`reminder_enabled`, `reminder_time`. Omitted or `null` fields are kept;
`""` clears `name`, `username` or `photo_id`.

The last three are the user's own settings and exist only here, not in the admin update:

| Field | Rules |
|---|---|
| `reminder_enabled` | boolean, the "Kunlik eslatma" switch |
| `reminder_time` | `HH:MM`, 24-hour, Uzbekistan time, e.g. `"18:00"` (422 otherwise); kept while the reminder is off |
| `agreed_on_conditions` | only `true` (422 for `false`): records consent for a user who signed up without it. Withdrawing consent means deleting the account |

The server only stores the reminder setting; the app schedules the notification on the phone. The rules are the admin update's
(`usecases/user_updater`): `name` at most 100 characters, `username` 3 to 32 of `a-z`, `0-9`,
`_`, `.` (lowercased), `photo_id` an id from `POST /v1/media` (422 if unknown), 409 when the
username or phone number belongs to someone else. Response `data` is the same as Get profile.

A new `phone_number` must be an Uzbek number (`+998` and 9 digits) verified first: send and
verify an OTP with purpose `update_user` **for the new number**. Without that mark: 403
`new phone number is not verified, ...`. A successful change deletes the mark. Sending the
current number is not a change and needs no code. `phone_number` cannot be cleared (422).

```bash
curl -X PATCH localhost:8080/v1/me -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"Dilnoza","photo_id":""}'
```

#### Reset password

1. `POST /v1/otp/send` with purpose `reset_password` and the user's own phone number.
2. `POST /v1/otp/verify` with the code.
3. `PUT /v1/me/password` with `{"password": "n3w-s3cret-pass"}` (8 to 72 bytes, 422 otherwise).

Without a `reset_password` mark for the user's number: 403 `phone number is not verified, ...`.
On success the password is stored as a bcrypt hash, the mark is deleted, and the response
`data` is a new token pair, the same shape as sign-up (`access_token`, `refresh_token`,
`expires_in`). Both stored token hashes are replaced, so every token issued before, on any
device, stops working (401); the app must switch to the new pair.

A signed-out user who forgot their password uses `POST /v1/auth/forgot-password` instead; see
[Forgot password](#forgot-password).

#### Delete account

`DELETE /v1/me` with no body. Response `data` is `null`. It is the same soft delete as
`DELETE /v1/admin/users/{id}`: the user disappears from reads, the `user_auth` row (tokens)
is removed by a trigger so the caller's tokens stop working at once, the username and phone
number can be registered again, and the user's children are deleted unless another parent
still has them (see [data-model.md](data-model.md)). There is no undo endpoint. The app should
keep this action low-key and ask for confirmation; the API does not.

#### My children

`GET /v1/me/children?limit=20&offset=0` returns a [page](#pagination) of the caller's active
children ([child object](#children-add-a-child-signed-in) fields). Use
`GET /v1/children/{id}` for one child's page.

### Child age in months

Every client sends and reads a child's age in **months** (`age_months`). The steps the app offers
are 1 to 11 months, then 1, 1.5, 2 ... 7 years (12 to 84 months in steps of 6), then 8 to 12 years
(96 to 144 in steps of 12). The database keeps years as a fraction (`children.age`, 1 month =
1/12); the conversion happens in one place, the child mapping of `internal/dbstore/child.go`
(`domain.ChildAgeToDB` and `domain.ChildAgeFromDB`), so no handler or client deals with years.
Content age ranges (`min_age`/`max_age` of activities, articles, videos, tips) stay in whole years
2 to 6; a child is matched by its whole years (`age_months / 12`) clamped to 2..6.

### Children: add a child (signed in)

| Method | Path | Description |
|---|---|---|
| POST | `/v1/children` | create a child profile and link it to the caller |

Needs [user authentication](#user-authentication); every role may call it. The child row and
its `user_children` link to the caller are written in one transaction, so the caller is the
child's first parent and a child never exists without one.

```json
{"name": "Amir", "age_months": 54, "gender": "male", "photo_id": null,
 "goal_ids": ["2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00", "8d0e6f1a-3b2c-4d5e-9f8a-7b6c5d4e3f2a"]}
```

| Field | Rules |
|---|---|
| `name` | required, trimmed, at most 100 characters |
| `age_months` | required, the age in **months**: 1 to 11, then half years from 12 to 84 (12, 18, 24 ... 84), then whole years from 96 to 144 (8 to 12 years); anything else is 422 |
| `age` | old field, whole years; read (times 12) only when `age_months` is missing. New clients send `age_months` |
| `gender` | required, `male` or `female` (case-insensitive) |
| `photo_id` | optional id from `POST /v1/media`; an unknown id is 422 |
| `goal_ids` | required, 1 to 20 goal ids from [`GET /v1/goals`](#goals), the goals picked for this child; repeats are stored once; an id that is not an active goal is 422 `goals contains ids that are not active goals, ...` |

In the same transaction the child gets its first [7-day plan](#children-7-day-plan-signed-in),
made from its goals and age (when no published activity fits the age yet, the plan is made on
the first `GET .../plan` instead).

Response `data` is the child: `id`, `name`, `age_months`, `age` (whole years, `age_months / 12`
rounded down, for old clients), `gender`, `photo_id`, `photo_url` (see [Media](#media-public)), `goal_ids`,
`created_at`, `updated_at`. Use `id` in the `/v1/children/{id}/...` endpoints below. Errors: 400 bad JSON,
401 no or invalid token, 403 a second child on the free plan (see [Birga Pro](#birga-pro)), 422 a rule above.

```bash
curl -X POST localhost:8080/v1/children \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Amir","age_months":54,"gender":"male","goal_ids":["'$GOAL'"]}'
```

`GET /v1/me/children` lists the caller's children and `GET /v1/children/{id}` returns one
(404 when the caller is not its parent). There is no endpoint yet to delete a child or to add a
second parent.

### Children: edit a child (signed in)

| Method | Path | Description |
|---|---|---|
| PATCH | `/v1/children/{id}` | change some fields of one of the caller's children |

Needs [user authentication](#user-authentication); any role. Body: any of `name`, `age_months`
(or the old `age` in years), `gender`, `photo_id`, `goal_ids`. Omitted or `null` fields are kept; `""` clears `photo_id`;
`goal_ids` replaces the child's goals (1 to 20 active goal ids, 422 otherwise). The given fields
follow the [add-child rules](#children-add-a-child-signed-in) (`usecases/child_updater`): `name`
1 to 100 characters after trimming, `age_months` in the steps above, `gender` `male` or `female`
(case-insensitive), `photo_id` an id from `POST /v1/media` (unknown: 422). An empty body is 422
`nothing to update`. A child the caller is not a parent of, or a deleted one, is 404. Any of the
child's parents may edit it. `updated_at` is set on every edit.

Response `data`: the child, as for add.

```bash
curl -X PATCH localhost:8080/v1/children/$CHILD \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Amirjon","age_months":60}'
```

A changed age or changed goals apply to the child's next plan; the current plan keeps its days.

### Children: today's activity, completions, streak (signed in)

All of these need [user authentication](#user-authentication) and a child linked to the
caller. Days are calendar days in Uzbekistan time (UTC+5, `domain.Location`). Everything is per
child: a parent with three children has three plans and three streaks.

| Method | Path | Description |
|---|---|---|
| GET | `/v1/children/{id}/recommendation` | today's plan activity for the child |
| POST | `/v1/children/{id}/completions` | mark an activity done today, with an optional reflection note |
| GET | `/v1/children/{id}/completions` | the child's completions, newest first (paged) |
| GET | `/v1/children/{id}/streak` | streak and progress counters |
| GET | `/v1/children/{id}/streak/days` | the streak calendar: done and missed days |
| GET | `/v1/children/{id}/today` | today's list: plan activity and added activities |
| POST | `/v1/children/{id}/today/extras` | add an activity to today's list (Birga Pro) |
| POST | `/v1/children/{id}/today/difficulty` | swap an activity of today's list for an easier or harder one (Birga Pro) |

#### Today's list

`GET /v1/children/{id}/today`: the "Bugungi reja" list (Figma 40-42), the plan activity of the
day and the activities a Pro parent added. It makes the child's first plan when it has none,
like `GET .../plan`.

```json
{"day": "2026-10-25",
 "plan": {"plan_day": 3, "status": "today", "done": false, "activity": {activity object}},
 "extras": [{"activity": {activity object}, "done": true, "added_at": "2026-10-25T09:12:00Z"}],
 "done_count": 1}
```

- `plan` is the open plan day (`status` `today`, `done` false) or the plan day done today
  (`status` `done`, `done` true). It is `null` when neither exists: the plan was finished on an
  earlier day (start the next one with `POST /v1/children/{id}/plan`), or no activity fits the
  child's age.
- `extras` are the added activities, oldest first. `done` means the child completed it today.
- `done_count` counts the done items ("2 tasi bajarildi").

`POST /v1/children/{id}/today/extras` (Birga Pro) adds an activity to today's list. Send one of:

- `{"goal_id": "..."}`: "Qaysi maqsad uchun yangi mashg‘ulot?" (40a). The backend picks a
  published activity for the child's age (2 to 6) that serves that goal, is not a day of the
  child's current 7-day plan (so it is never done early and then again on its plan day) and is
  not in today's list yet. Normal-difficulty activities come first, then the ones the child never
  did, then the one done longest ago. 404
  `no other published activity serves this goal at the child's age` when none is left, 422 for a
  goal that is not active.
- `{"activity_id": "..."}`: "Bugungi rejaga qo‘shish" and "Yana bittasi" (42). 404 for an
  unknown or unpublished activity. 409 `this activity is a day of the child's 7-day plan, do it on
  its plan day` when it is today's plan activity or a later plan day not done yet (done plan days
  may be repeated).

Response: the new item `{"activity", "done", "added_at"}`. Adding an activity that is already
in the list returns it unchanged. A day holds at most 10 added activities (422). Without Pro the
call is 403 `this needs Birga Pro`; sending both fields or neither is 422.

Mark an extra done with the normal [Complete](#complete) call. It goes into the history and
`done_count`, but not the streak: only plan days keep the streak.

#### Difficulty switch

`POST /v1/children/{id}/today/difficulty` (Birga Pro): "Qiyinlik darajasi" with "Osonroq",
"Odatdagi" and "Qiyinroq" on the activity page. Every activity has a `difficulty` (`easy`,
`normal`, `difficult`); plans give normal ones. The switch swaps the activity for another one of
the chosen level:

```json
{"activity_id": "7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11", "difficulty": "easy"}
```

- `activity_id` is an item of the child's [list for today](#todays-list): the open plan activity
  (`plan.activity`, `status` `today`) or an added activity (`extras[].activity`) not done yet.
- The backend picks a published activity with that `difficulty` for the child's age (2 to 6) and
  the same goal: the plan day's `goal_id` for the plan activity, one of the activity's `goal_ids`
  for an added one (sharing more of them first). It is never the current activity, a day of the
  child's 7-day plan or an activity already in today's list. So asking for the level the activity
  already has (easy, then easy again) gives another activity of that level. The ones the child
  never did come first, then the one done longest ago.
- The new activity takes the item's place in today's list. On the plan day it becomes that day's
  activity in [`GET .../plan`](#children-7-day-plan-signed-in) too, so completing it marks the plan
  day done and keeps the streak. Switching back ("Odatdagi") picks another normal activity; the
  old one is not remembered.

Response: the new [activity object](#activity-object). Show the segment that matches its
`difficulty`.

| HTTP | error_code | When |
|---:|---:|---|
| 403 | -21 | free plan: `this needs Birga Pro` |
| 404 | -30 | `this activity is not in the child's list for today` |
| 404 | -30 | `no other published activity of this difficulty serves the goal at the child's age` (keep the current activity) |
| 409 | -40 | `this activity is already done today, it cannot be changed` |
| 422 | -10 | `difficulty must be easy, normal or difficult, got "..."`, or `activity_id` not a UUID |

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" localhost:8080/v1/children/$CHILD/today/difficulty \
  -d '{"activity_id": "'$ACTIVITY'", "difficulty": "difficult"}'
```

#### Similar activities

`GET /v1/activities/{id}/similar?child_id=&limit=3` (signed in, Birga Pro): "O‘xshash
mashg‘ulotlar" on the activity page (42). It returns an array of published activities that share
a goal with this one, without the activity itself. With `child_id` (one of the caller's
children, else 404) they fit the child's age, leave out the days of the child's current 7-day
plan, and the ones the child never did, then did longest ago, come first. Without it they overlap the activity's age range. Then come those sharing more
goals, then the newest. `limit` is 1 to 10 (default 3, 422 outside). It is 404 for an unknown or
unpublished activity and 403 without Pro.

#### Recommendation

The activity of the [plan](#children-7-day-plan-signed-in) day that is open today (`status`
`today`); the call makes the child's first plan if it has none. It takes no query parameters.
Response: an [activity object](#activity-object).

- 404 `no plan day is open today: ...` (error code -30) when today's plan day is already done
  (the next one opens tomorrow) or the plan is done (start the next one with
  `POST /v1/children/{id}/plan`).
- 404 `no published activity matches this child's age and the filters` when no activity fits.

```bash
curl -H "Authorization: Bearer $TOKEN" localhost:8080/v1/children/$CHILD/recommendation
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
- When the activity is the child's plan day open today, that day is marked done and counts
  toward the child's streak; `plan_day` in the response names the day (1 to 7). Any other
  activity (from the catalogue, or a later plan day ahead of time) only goes into the history:
  `plan_day` is `null`.
- That second kind needs [Birga Pro](#birga-pro): on the free plan it is 403 (-21) `this needs
  Birga Pro` and nothing is stored. Repeating a completion already made today still works.

Response: a [completion object](#completion-object) with `plan_day`.

#### Streak

Response:

```json
{"current": 3, "longest": 7, "completed_today": true, "this_week": 2, "total": 12, "last_completed_on": "2026-10-03"}
```

| Field | Meaning |
|---|---|
Only the child's plan counts: a day is a streak day when one of the child's plan days was done
on it ([Complete](#complete) with `plan_day` set). Other completions stay in the history but do
not keep the streak. Each child has its own streak, shared by all its parents.

| Field | Meaning |
|---|---|
| `current` | consecutive streak days ending today; while today has none yet it ends yesterday, so the streak is only lost after a whole day is missed |
| `longest` | the longest such run ever |
| `completed_today` | a plan day was done today |
| `this_week` | streak days since Monday |
| `total` | plan days done, over all the child's plans |
| `last_completed_on` | the latest streak day, `null` before the first |

#### Streak days

`GET /v1/children/{id}/streak/days?from=2026-10-01&to=2026-10-13`: the calendar for the
Streaks page, one entry per day, oldest first.

```json
[{"day": "2026-10-11", "status": "done"}, {"day": "2026-10-12", "status": "missed"},
 {"day": "2026-10-13", "status": "today"}]
```

| `status` | Meaning |
|---|---|
| `done` | a plan day was done that day |
| `missed` | no plan day was done that day; it reset the streak. Days between finishing a plan and starting the next also count as missed when nothing is done |
| `today` | today, nothing done yet: not missed until the day is over |

`from` and `to` are optional dates (`YYYY-MM-DD`), both inclusive. Without them the range is the
last 28 days up to today; without `from`, the 28 days up to `to`. Days before the child's first
plan started and days after today are left out, so a child without a plan gets `[]`. A range
over 366 days or `from` after `to` is 422; a date that does not parse is 422.

### Children: 7-day plan (signed in)

The app's "Reja" tab: seven activities, one per day, built from the child's goals. Needs
[user authentication](#user-authentication) and a child linked to the caller. Each child has its
own plans; the latest one is the current plan.

| Method | Path | Description |
|---|---|---|
| GET | `/v1/children/{id}/plan` | the child's current plan (made when the child was added, or on the first call) |
| POST | `/v1/children/{id}/plan` | start the next plan, after the current one is done (no body) |

**Making a plan** (`usecases/plan_getter`, `planRepo.Pick`). Plan number *n* gives its seven days
the child's goals in turn, starting one goal further with each plan: with goals A, B, C plan 1 is
A B C A B C A, plan 2 is B C A B C A B, so no goal always gets the extra day. A child with one goal
gets it every day; a child whose goals were all deleted gets days with no goal. For each day the
plan takes one published activity whose age range contains the child's age (clamped to 2..6),
not yet used in this plan, ordered by:

1. serving the day's goal;
2. `difficulty` `normal` (easy and difficult activities are for the [difficulty switch](#difficulty-switch));
3. serving more of the child's goals;
4. never done by the child, then done longest ago;
5. a tie-break by `md5(activity id || child id || plan number and day)`.

When the catalogue for the age has fewer than seven activities, later days repeat earlier
activities. No published activity for the age at all: 404 `no published activity matches this
child's age and the filters`. The plan is stored (`child_plans`, `child_plan_days`) and stays
the same; changing the child's goals or age only affects the next plan. Two parents asking at
once get the same plan (the child row is locked while it is made). If a day that is not done
yet points at an activity unpublished or deleted since, that day gets a new activity on the next
read.

**Progress.** Complete a day with `POST /v1/children/{id}/completions` and the day's
`activity.id`; the response's `plan_day` confirms it counted. Days are done in order, at most one
plan day per calendar day (Uzbekistan time), counting all the child's plans:

| `status` | Meaning |
|---|---|
| `done` | done; `completed_on` is the day |
| `today` | the next day to do, open now |
| `locked` | a later day, or the next day while a plan day was already done today (it opens tomorrow) |

**A missed day pauses the plan**: the open day stays open, however many days pass, until it is
done, so no day is skipped and a plan can take more than seven calendar days (the missed days
still break the [streak](#streak)). Completing a later day's activity early does not skip ahead.

**The next plan.** `completed` is `true` once all seven days are done. Then `POST` starts plan
`number + 1` today, from the child's goals as they are at that moment (change them first with
[`PATCH /v1/children/{id}`](#children-edit-a-child-signed-in) if wanted). When the last day was
done today, day 1 of the new plan opens tomorrow. `POST` before the plan is done is 409
`the current plan is not done yet, ...` (error code -40).

Response `data`:

```json
{
  "id": "8c1e2d3f-4a5b-4c6d-8e7f-9a0b1c2d3e4f",
  "child_id": "3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f",
  "number": 1,
  "started_on": "2026-10-11",
  "completed": false,
  "days": [
    {"day": 1, "goal_id": "2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00", "status": "done", "completed_on": "2026-10-11",
     "activity": {"id": "7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11", "title_uz": "Sharni puflaymiz", "...": "..."}},
    {"day": 2, "goal_id": "8d0e6f1a-3b2c-4d5e-9f8a-7b6c5d4e3f2a", "status": "today", "completed_on": null,
     "activity": {"...": "..."}}
  ]
}
```

`days` always has seven entries in day order. `goal_id` is the child's goal the day serves (show
its name from [`GET /v1/goals`](#goals)); `null` when the child had no goals. `activity` is an
[activity object](#activity-object).

```bash
curl -H "Authorization: Bearer $TOKEN" localhost:8080/v1/children/$CHILD/plan
curl -X POST -H "Authorization: Bearer $TOKEN" localhost:8080/v1/children/$CHILD/plan
```

### Development milestones ("Ko‘nikmalar")

Milestones (skills) are what most children can do by an age, in four domains (Figma 27, 44, 45).
Migration `000022` seeds 57 published milestones for 2 to 5 years, based on the CDC "Learn the Signs.
Act Early." list, in Uzbek, Russian and English. Their advice texts are empty until the content team
writes them.

| Method | Path | Description |
|---|---|---|
| POST | `/v1/admin/milestones` | create a milestone |
| GET | `/v1/admin/milestones` | milestones in every status: `domain`, `age_months`, `status`, `q`, `limit`, `offset` |
| GET | `/v1/admin/milestones/{id}` | one milestone in any status |
| PATCH | `/v1/admin/milestones/{id}` | change some fields, including the status |
| DELETE | `/v1/admin/milestones/{id}` | soft-delete a milestone |
| GET | `/v1/children/{id}/milestones` | the child's milestones of one stage with ticks (`age_months`) |
| PUT | `/v1/children/{id}/milestones/{milestone_id}` | mark it achieved ("O‘zlashtirdi deb belgilash") |
| DELETE | `/v1/children/{id}/milestones/{milestone_id}` | remove the mark |
| GET | `/v1/milestones/{id}` | one published milestone with its advice and activities (`child_id`) |
| GET | `/v1/children/{id}/report` | the monthly report (Birga Pro, `month=2026-10`) |

Milestone object:

```json
{"id": "...", "domain": "communication", "age_months": 48, "sort_order": 1,
 "text_uz": "4 va undan ko‘p so‘zli gaplar tuzadi", "text_ru": "Говорит предложениями из 4 и более слов",
 "text_en": "Says sentences with four or more words",
 "meaning_uz": "", "meaning_ru": "", "meaning_en": "",
 "how_to_uz": "", "how_to_ru": "", "how_to_en": "",
 "specialist_uz": "", "specialist_ru": "", "specialist_en": "",
 "activity_ids": [], "has_advice": false, "advice_locked": false, "status": "published",
 "created_at": "...", "updated_at": "..."}
```

| Field | Rules |
|---|---|
| `domain` | `communication` (Nutq va muloqot), `social` (His-tuyg‘ular va do‘stlik), `thinking` (Fikrlash) or `movement` (Harakat); the app holds the labels |
| `age_months` | the stage, 2 to 72 (the seed uses 24, 30, 36, 48 and 60); the app makes the label ("3 yosh") |
| `sort_order` | order within a domain and stage, default 0 |
| `text_*` | required, trimmed, at most 300 characters |
| `meaning_*` | "Bu nimani anglatadi", optional, at most 3000 characters |
| `how_to_*` | "Qanday rivojlantirish", one step per line, optional, at most 3000 characters |
| `specialist_*` | "Qachon mutaxassisga murojaat qilish kerak", optional, at most 3000 characters |
| `activity_ids` | "Mos mashg‘ulotlar", up to 10 activity ids; unknown or deleted ids are 422 |
| `status` | `published`, `in_review` (the default) or `unpublished`; only `published` reaches the app |
| `has_advice` | read only: any advice text or activity is set, so the app shows the "Maslahat" link |
| `advice_locked` | read only: the advice exists but was left out because the caller has no Birga Pro |

Every list is ordered by stage, then domain in the order above, then `sort_order`. On `PATCH`
the required texts cannot be emptied, the advice texts can; `activity_ids`, when sent, replaces
the list. Deleting a milestone keeps the children's marks but no longer shows or counts them.

#### A child's milestones

`GET /v1/children/{id}/milestones?age_months=48`:

```json
{"age_months": 48, "stages": [24, 30, 36, 48, 60], "achieved": 5, "total": 13,
 "domains": [{"domain": "communication", "milestones": [{milestone object, "achieved": true, "achieved_at": "2026-10-20T08:00:00Z"}]},
             {"domain": "social", "milestones": []}, {"domain": "thinking", "milestones": []}, {"domain": "movement", "milestones": []}]}
```

- Without `age_months`, the stage is the child's: the highest stage at or below the child's age
  (`age` × 12 months). A child younger than every stage gets the first one. `age_months` outside
  2..72 is 422.
- `stages` lists the stages that have published milestones, for the stage picker.
- `domains` always holds the four domains in order. `achieved` and `total` count the stage.
- Without Birga Pro the advice fields are empty and `activity_ids` is `[]`; `has_advice` and
  `advice_locked` tell the app to show a locked "Maslahat".

`PUT /v1/children/{id}/milestones/{milestone_id}` marks a published milestone for the child,
on any plan; marking again keeps the first time. `DELETE` removes the mark. The marks are the
child's, so every parent of the child sees them. 404 for a child that is not the caller's or a
milestone that is unknown or not published.

#### One milestone ("Maslahat")

`GET /v1/milestones/{id}?child_id=` returns a published milestone with `achieved` and
`achieved_at` for that child (`false` and `null` without `child_id`) and `activities`, the
related published activities as [activity objects](#activity-object). Without Birga Pro the
advice fields are empty and `activities` is `[]`. 404 for an unknown or unpublished milestone;
`child_id` must be one of the caller's children (404) and a UUID (422).

#### Monthly report

`GET /v1/children/{id}/report?month=2026-10` (Birga Pro, Figma 44):
`{"month": "2026-10", "new_milestones": 3, "completions": 26}`. `new_milestones` counts the
milestones marked in that month and `completions` the activities done, both in Uzbekistan time.
Without `month` it is the current month. A month not shaped like `2026-10` is 422; free users
get 403.

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
  "purpose": "sign_up"
}
```

The `User-Agent` header is required too (422 `the User-Agent header is required` when it is
missing or blank): the rate limiter counts codes per User-Agent value. The apps send a
User-Agent on every request; the mobile app should make it identify the install (for example
`Birga/1.0.0 (Android 14; <install id>)`), because every client with the same value shares one
counter. Browsers send their own User-Agent and cannot change it. The `ip_address` field older
clients send is ignored.

| Field | Rules |
|---|---|
| `phone_number` | Uzbek mobile number in E.164: `+998` and 9 digits (422). Play Mobile only delivers in Uzbekistan |
| `purpose` | `sign_up` (registration; not required by sign-up for now), `update_user` (a new phone number for `PATCH /v1/me`) or `reset_password` (`POST /v1/auth/forgot-password` and `PUT /v1/me/password`) (422) |

Response `data`:

```json
{"expires_in": 120, "resend_in": 60}
```

`expires_in` is how many seconds the code stays valid (`OTP_TTL`, 2 minutes); `resend_in` is
how many seconds the client must wait before asking for another code (`OTP_RESEND_COOLDOWN`).

Behaviour (`usecases/otp_sender`):

- `sign_up`: the number must not belong to an active user, otherwise 409
  `phone number is already registered`. `reset_password`: the number must belong to an active
  user, otherwise 404 `phone number does not belong to a user`. `update_user` does not check
  the users table. These checks run before the rate limiter, so no SMS is spent on them.
- Rate limiter, checked before anything is sent, all returning 429 (code -70) with the
  number of seconds to wait in `error_note`:
  - one code per phone and purpose every `OTP_RESEND_COOLDOWN` (60 s): `a code was sent
    recently, request a new one in N seconds`;
  - at most `OTP_MAX_PER_PHONE_HOUR` (5) codes per phone per hour, all purposes together;
  - at most `OTP_MAX_PER_USER_AGENT_HOUR` (20) codes per `User-Agent` value per hour:
    `too many codes requested from this device, try again in N seconds`.

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
| `code` equals `OTP_DEFAULT_CODE` (when set) | 200 | 0 | phone marked verified without checking; any pending code is dropped |

**Default code.** When `OTP_DEFAULT_CODE` is set (6 digits; `.env.example` uses `654321`),
verify accepts it for any phone and purpose without a code having been sent, and marks the
phone verified exactly as a real code does, so sign-up works with it. It is meant for local
development, testing and app-store review. Unset means off; the API refuses to start with it in
production. Each use logs a warning `otp verified with the default code`.

The comparison runs as one Redis script, so two parallel requests cannot both use a code.
The verified mark is a Redis key (see [data-model.md](data-model.md#redis-keys)) rather than a
token in the response: `PATCH /v1/me` checks it for `update_user` (the new number), and `POST /v1/auth/forgot-password` and `PUT /v1/me/password`
for `reset_password`.

Examples:

```bash
curl -X POST localhost:8080/v1/otp/send -H 'Content-Type: application/json' \
  -H 'User-Agent: Birga/1.0.0 (Android 14; 5b1e0c7a)' \
  -d '{"phone_number": "+998901234567", "purpose": "sign_up"}'

curl -X POST localhost:8080/v1/otp/verify -H 'Content-Type: application/json' \
  -d '{"phone_number": "+998901234567", "purpose": "sign_up", "code": "480569"}'
```

### Auth (public)

Sign-up, login, logout, forgot password and access token refresh. Tokens are HS256 JWTs signed
with `JWT_SECRET`.

| Method | Path | Description |
|---|---|---|
| POST | `/v1/auth/signup` | create a user (role `unverified_user`, no OTP step), return access and refresh tokens |
| POST | `/v1/auth/login` | sign in with username and password, return a new token pair |
| POST | `/v1/auth/logout` | sign out: the current tokens stop working (needs the access token) |
| POST | `/v1/auth/forgot-password` | set a new password for a signed-out user whose phone passed a `reset_password` code, return a new token pair |
| POST | `/v1/auth/refresh` | trade a refresh token for a new access token |

Only one token pair is valid per user at a time: sign-up, login and password resets store the
hashes of the pair they return, so the pair held by any other device stops working.

Token claims: `sub` is the user id, `typ` is `access` or `refresh`, `role` is the user's
`user_auth.role` when the token was issued (`unverified_user`, `user`, `admin` or `paid_user`), plus `iss`
(`JWT_ISSUER`), `iat`, `exp` and a unique `jti`. An access token lives `JWT_ACCESS_TTL`
(24 hours). A refresh token has no `exp` and never expires while `JWT_REFRESH_TTL` is 0, the
default; it stops working only when the user logs out, logs in again, resets the password or is
deleted, or when `JWT_SECRET` changes. Set
`JWT_REFRESH_TTL` to a duration to give refresh tokens an expiry again. A token of one type is rejected where
the other is expected. Endpoints for signed-in users take the access token as
`Authorization: Bearer <access_token>`; see [User authentication](#user-authentication).

#### Sign up

One call, no OTP step: the phone number is not verified, so every new user gets the role
`unverified_user`. There is no profile photo at sign-up; the app sets `photo_id` later with
[`PATCH /v1/me`](#edit-profile).

```json
{
  "name": "Dilnoza",
  "username": "dilnoza_k",
  "password": "s3cret-pass",
  "phone_number": "+998901234567",
  "relationship": "mother",
  "agreed_on_conditions": true
}
```

| Field | Rules |
|---|---|
| `name` | required, at most 100 characters, trimmed |
| `username` | 3 to 32 of `a-z`, `0-9`, `_`, `.`; trimmed and lowercased |
| `password` | 8 to 72 bytes (bcrypt reads at most 72); stored only as a bcrypt hash |
| `phone_number` | Uzbek number, `+998` and 9 digits; not verified |
| `relationship` | required: `father`, `mother`, `educator` or `nanny` (trimmed, case-insensitive) |
| `role` | optional: `user` (default) or `admin`. `admin` needs the header `X-Admin-Key: <ADMIN_API_KEY>`: wrong or missing key 401 (-20), `ADMIN_API_KEY` not set on the server 403 (-21); any other value 422. The app never sends it; it is for creating dashboard admins |
| `agreed_on_conditions` | optional boolean, the consent tick on the sign-up screen; stored in `users.agreed_on_conditions`, omitted means `false`. Sign-up does not refuse `false` yet; the app should only let the user continue once it is ticked |

Goals are not part of sign-up: they belong to each child ([add a child](#children-add-a-child-signed-in)).
Any other field (for example an old client's `user_role`, `photo_id` or `goals`) is ignored.

Response `data`:

```json
{"user_id": "7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11", "access_token": "eyJ...", "refresh_token": "eyJ...", "expires_in": 86400}
```

`user_id` is the new user's id; `expires_in` is the access token lifetime in seconds.

Behaviour (`usecases/user_signup`):

| Result | HTTP | error_code | error_note |
|---|---:|---:|---|
| invalid field | 422 | -10 | which field and why |
| username or phone number belongs to an active user | 409 | -40 | `username is already taken` / `phone number is already taken` |

- The `users` row (name, username, phone, relationship; `photo_id` NULL) and the `user_auth` row (username,
  bcrypt password, role `unverified_user`, SHA-256 hashes of both tokens) are written in one transaction.
- The access and refresh tokens carry `role: unverified_user` (`admin` for an admin sign-up). That role may use `/v1/me...`,
  `POST /v1/children` and `/v1/children/{id}...` like `user`; nothing promotes it yet.
- The OTP endpoints and the `sign_up` purpose still exist but sign-up no longer reads the
  `sign_up` verified mark.

#### Refresh

```json
{"refresh_token": "eyJ..."}
```

Response `data`: `{"access_token": "eyJ...", "expires_in": 86400}`.

The refresh token must be signed with `JWT_SECRET`, unexpired if it has an `exp`, of type `refresh`, and its
SHA-256 must equal `user_auth.refresh_token` for the user in `sub`. Anything else, including a
soft-deleted user (their `user_auth` row is gone), is 401 (-20) `refresh token is invalid or
expired, sign in again`. The new access token's hash replaces `user_auth.access_token`; its `role` claim is read from
`user_auth.role` at refresh time, so a changed role shows up in the next access token. The
refresh token is not rotated; the client keeps it for good (or until it expires, when
`JWT_REFRESH_TTL` is set).

#### Login

```json
{"username": "dilnoza_k", "password": "s3cret-pass"}
```

Response `data`: a token pair, the same shape as sign-up.

Behaviour (`usecases/user_login`):

| Result | HTTP | error_code | error_note |
|---|---:|---:|---|
| `username` or `password` empty | 422 | -10 | `username and password are required` |
| unknown username, a user without a password, or a wrong password | 401 | -20 | `username or password is wrong` |

- `username` is trimmed and lowercased, like at sign-up.
- An unknown username still runs a bcrypt comparison, so the answer takes as long as a wrong
  password and does not tell which usernames exist.
- There is no login rate limit yet.

#### Logout

`POST /v1/auth/logout` with `Authorization: Bearer <access_token>` and no body. `data` is
null. Both stored token hashes are cleared, so the access token and the refresh token stop
working at once (401 afterwards, including on `/v1/auth/refresh`). Because only one pair is
valid at a time, this signs the user out everywhere. Sign in again with `/v1/auth/login`.

#### Forgot password

For a signed-out user (no access token needed):

1. `POST /v1/otp/send` with purpose `reset_password` and the user's phone number (404 if no
   active user has it).
2. `POST /v1/otp/verify` with the code.
3. Within `OTP_VERIFIED_TTL` (10 min), `POST /v1/auth/forgot-password`:

```json
{"phone_number": "+998901234567", "password": "n3w-s3cret-pass"}
```

Response `data`: a new token pair, so the user is signed in.

Behaviour (`usecases/password_resetter`, `ExecuteByPhone`):

| Result | HTTP | error_code | error_note |
|---|---:|---:|---|
| `phone_number` not `+998` and 9 digits, or `password` not 8 to 72 bytes | 422 | -10 | which field and why |
| no active user has the number | 404 | -30 | `phone number does not belong to a user` |
| no `reset_password` verification for the number | 403 | -21 | `phone number is not verified, verify a reset_password code with /v1/otp/verify first` |

On success the password is stored as a bcrypt hash, the verified mark is deleted, and both
token hashes are replaced, so every token issued before stops working. It is the same use case
as `PUT /v1/me/password`, which takes the user from the access token instead of the phone.

```bash
curl -X POST localhost:8080/v1/auth/signup -H 'Content-Type: application/json' \
  -d '{"name": "Dilnoza", "username": "dilnoza_k", "password": "s3cret-pass", "phone_number": "+998901234567"}'

curl -X POST localhost:8080/v1/auth/refresh -H 'Content-Type: application/json' \
  -d '{"refresh_token": "eyJ..."}'

curl -X POST localhost:8080/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"username": "dilnoza_k", "password": "s3cret-pass"}'

curl -X POST localhost:8080/v1/auth/logout -H 'Authorization: Bearer eyJ...'

curl -X POST localhost:8080/v1/auth/forgot-password -H 'Content-Type: application/json' \
  -d '{"phone_number": "+998901234567", "password": "n3w-s3cret-pass"}'
```

### Media (public)

Uploads an image to AWS S3 and returns its id and URL. The app uploads a profile photo
here first, then saves the returned `id` as the user's `photo_id` with `PATCH /v1/me` (sign-up
takes no photo).

| Method | Path | Description |
|---|---|---|
| POST | `/v1/media` | upload one image |
| POST | `/v1/admin/media` | admin only: upload an image or an MP4/WebM video (for [daily videos](#daily-videos-posts-admin)) |
| GET | `/v1/media/{id}` | open an image: 302 redirect to a signed S3 link |

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
- at most `MEDIA_MAX_SIZE` bytes after decoding (default 100 MiB), otherwise 422; an empty file
  is 422 too;
- a missing `file` field or invalid base64 is 400;
- when S3 is not configured (`S3_BUCKET` empty) the endpoint answers 503 `media uploads are
  disabled`; a failed S3 call is 500.

The object is stored as `<S3_KEY_PREFIX>media/<id>.<ext>`. Response `data`:

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | the media id; put it in `photo_id` |
| `url` | string | `/v1/media/{id}`: the path on this API to load the image from |
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
    "url": "/v1/media/3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f",
    "content_type": "image/jpeg",
    "size": 183244
  }
}
```

#### Showing an image

The bucket is private, so S3 links without a signature answer 403. Load an image from
`GET /v1/media/{id}` on this API (the `url` above, and `photo_url` on users and children,
already hold this path; put the API's base URL in front, e.g.
`https://api.birga-oila.com/v1/media/<id>`). It needs no token, so `<img src>` and Flutter's
`Image.network` work as they are. It answers 302 with `Location` set to a signed S3 link that
works for `MEDIA_LINK_TTL` (default 1 hour), and `Cache-Control: private, max-age=300`; image
clients follow the redirect on their own. An id that is not an uploaded media id is 404
`media not found`; S3 not configured is 503. Media ids are random UUIDs, so only someone who was
given an id can open the photo.


### Users (admin)

Users are app accounts (parents and caregivers). These endpoints are admin-only. Deleting is a
soft delete; deleted users behave as if they do not exist on every endpoint below. Deleting a
user also deletes their children that have no other parent, and unlinks them from shared ones
(see [data-model.md](data-model.md#user_children)).

| Method | Path | Description |
|---|---|---|
| POST | `/v1/admin/users` | create a user |
| GET | `/v1/admin/users` | list users, newest first ([Pagination](#pagination)); `?q=` searches |
| GET | `/v1/admin/users/{id}` | one user |
| PATCH | `/v1/admin/users/{id}` | change some fields of a user |
| DELETE | `/v1/admin/users/{id}` | soft-delete a user; `data` is `null` |
| PUT | `/v1/admin/users/{id}/pro` | give [Birga Pro](#birga-pro) until a date, or end it |

Create body (only `phone_number` is required):

```json
{
  "name": "Dilnoza",
  "username": "dilnoza_95",
  "phone_number": "+998901234567",
  "photo_id": "3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"
}
```

List search: `GET /v1/admin/users?q=dilnoza&limit=20&offset=0` keeps users whose username starts
with `q`, or whose name or phone number contains it (case-insensitive; a leading `@` and spaces are
ignored; `%` and `_` are plain characters). An exact username match comes first, then newest
first. `total` counts the matches.

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
| `photo_url` | string or `null` | `/v1/media/{photo_id}`, the path to load the photo from on this API (see [Media](#media-public)); `null` without a photo |
| `relationship` | string or `null` | `father`, `mother`, `educator` or `nanny`; `null` for users created before sign-up asked for it, or by an admin |
| `agreed_on_conditions` | bool | consent given at sign-up (or later through `PATCH /v1/me`); `false` for older users and users created by an admin |
| `reminder_enabled` | bool | daily reminder on or off, `false` by default |
| `reminder_time` | string | daily reminder time, `HH:MM` in Uzbekistan time, `"18:00"` by default |
| `role` | string or `null` | sign-in role: `unverified_user`, `user`, `paid_user` or `admin`; `null` for a user without a sign-in account (created by an admin) |
| `plan` | string | `pro` while the user has [Birga Pro](#birga-pro) (role `paid_user` or `admin`, or `pro_until` in the future), otherwise `free` |
| `pro_until` | RFC 3339 timestamp or `null` | when Pro given by date ends; `null` when never given |
| `created_at`, `updated_at` | RFC 3339 timestamp | `updated_at` changes on every update and on delete |

`deleted_at` is never returned: deleted users are not returned at all.

### Goal object

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | generated by the database |
| `name_uz`, `name_ru`, `name_en` | string | each unique among active goals in its language, ignoring case |
| `created_at`, `updated_at` | RFC 3339 timestamp | |

Deleted goals are never returned, so `deleted_at` is not part of the object.

### Activity object

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | generated by the database |
| `title_uz`, `title_ru`, `title_en` | string | the API returns every language; the app and dashboard show the one the user picked |
| `description_uz`, `description_ru`, `description_en` | string | `title_en` and `description_en` are `""` for activities created before English was added (`000015`) until an admin fills them in |
| `main_idea_uz`, `main_idea_ru`, `main_idea_en` | string | the "Katta g'oya" / "Главная идея" / "Big idea" box on the activity page: what the activity teaches. Optional, `""` until an admin fills it in; the app hides the box when the text for the user's language is empty |
| `easier_uz`, `easier_ru`, `easier_en` | string | the "Osonroq" variant (Figma 41, a Birga Pro box in the app); `""` when there is none |
| `harder_uz`, `harder_ru`, `harder_en` | string | the "Qiyinroq" variant; `""` when there is none. The API always returns both; the app shows them to Pro users and a locked box to the others |
| `difficulty` | string | `easy`, `normal` or `difficult` ("Osonroq", "Odatdagi", "Qiyinroq"); see [Difficulty switch](#difficulty-switch) |
| `source` | string | `test`, `admin`, `harvard` or `ai`; see [Content source](#content-source) |
| `photo_id` | UUID string or `null` | the activity's picture (card and activity page); `null` without one, then the app shows its own placeholder. The starter activities get theirs from [seed photos](operations.md#seed-photos) |
| `photo_url` | string or `null` | `/v1/media/{photo_id}`, see [Showing an image](#showing-an-image); `null` without a picture |
| `goal_ids` | array of UUID strings | the [goals](#goals) the activity serves; normally 1 to 5, empty only if its goals were deleted |
| `min_age`, `max_age` | int | years, inclusive |
| `duration_minutes` | int | |
| `is_published` | bool | hidden from public endpoints when false |
| `created_at`, `updated_at` | RFC 3339 timestamp | |

Soft-deleted activities are never returned, so `deleted_at` is not part of the object.

### Article object

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | generated by the database |
| `title_uz`, `title_ru`, `title_en` | string | |
| `article_body_uz`, `article_body_ru`, `article_body_en` | string | plain text; paragraphs separated by blank lines |
| `tip_uz`, `tip_ru`, `tip_en` | string | the "try it today" box; `""` when there is none |
| `photo_id` | UUID string or `null` | id from `POST /v1/media` |
| `photo_url` | string or `null` | `/v1/media/{photo_id}`, see [Showing an image](#showing-an-image) |
| `expected_read_time` | int | minutes |
| `min_age`, `max_age` | int | years, inclusive (the "2–5 yosh" tag) |
| `goal_id` | UUID string | the [goal](#goals) the article serves (the tag, e.g. "Xulq-atvor") |
| `topic_id` | UUID string or `null` | optional [topic](#topics) tag (e.g. "Uyqu"); when set the app shows it instead of the goal name. Create: omitted, `null` or `""` means none; update: `""` removes it, omitted or `null` keeps it |
| `status` | string | `published`, `in_review` or `unpublished` |
| `source` | string | `test`, `admin`, `harvard` or `ai`; see [Content source](#content-source) |
| `created_at`, `updated_at` | RFC 3339 timestamp | |
| `is_read` | bool | only on the signed-in endpoints: whether this user marked it read |
| `locked` | bool | only on `GET /v1/articles`: `true` when the article does not open for a free-plan user (texts are empty then) |

Soft-deleted articles are never returned, so `deleted_at` is not part of the object.

### Post object

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | generated by the database |
| `title_uz`, `title_ru`, `title_en` | string | |
| `video_id` | UUID string | the uploaded video |
| `video_url` | string | `/v1/media/{video_id}`; play it from the API base address |
| `photo_id` | UUID string or `null` | the cover picture shown on video cards and before the video plays; `null` without one (the app shows its own placeholder). Admin create and update take it: an image from `POST /v1/admin/media` (unknown: 422); update `""` removes it |
| `photo_url` | string or `null` | `/v1/media/{photo_id}`, see [Showing an image](#showing-an-image); `null` without a cover |
| `goal_id` | UUID string | the [goal](#goals) the video serves (the tag, e.g. "Nutq") |
| `topic_id` | UUID string or `null` | optional [topic](#topics) tag, same rules as on articles; also on the `next` previews |
| `min_age`, `max_age` | int | years, inclusive ("3–5 yosh") |
| `duration` | number | minutes, a decimal (`4.2` is 4:12) |
| `suggestion_uz`, `suggestion_ru`, `suggestion_en` | string | the "Tavsiya" line; `""` when there is none |
| `questions_uz`, `questions_ru`, `questions_en` | string | questions to ask after watching, one per line; `""` when there are none |
| `status` | string | `published`, `in_review` or `unpublished` |
| `source` | string | `test`, `admin`, `harvard` or `ai`; see [Content source](#content-source) |
| `created_at`, `updated_at` | RFC 3339 timestamp | |

Soft-deleted posts are never returned.

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
| `plan_day` | int or `null` | only in the `POST .../completions` response: the plan day (1 to 7) this completion did, `null` when it did not count for the plan |
