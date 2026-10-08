# Birga backend documentation

Birga is a mobile app for caregivers in Uzbekistan with children aged 2 to 6. Each day it
suggests one short, adult-led offline activity, chosen by simple rules (child age, the
goals the caregiver picked at sign-up, available time), and tracks completions and streaks. Content is in Uzbek
and Russian.

This repository is the Go API behind the app. Today it serves the activity catalogue and a
small admin API for managing activities and users (create, read, update, publish, soft
delete), sends and verifies one-time SMS codes (Play Mobile, codes kept in Redis) for account
changes (sign-up skips OTP for now and gives the role `unverified_user`; it asks the user's
relationship to the children and their goals), serves the list of goals (admins manage it),
and uploads profile photos to AWS S3. For a signed-in parent it
recommends the day's activity for a child, records completions with an optional reflection,
and reports the child's streak. Child profiles exist in the database and repository
(`children`, linked to parents many-to-many) but have no endpoints yet, so children are
created outside the API for now.

## Start here

| Document | Read it when you want to |
|---|---|
| [setup.md](setup.md) | run the API and database on your machine, configure it, run tests |
| [architecture.md](architecture.md) | understand the layers, how a request flows, and how to add a feature |
| [api.md](api.md) | call the API: envelope, error codes, endpoints, admin auth |
| [data-model.md](data-model.md) | see the database tables, constraints, migrations and Redis keys |
| [operations.md](operations.md) | build, deploy, monitor and back up the service |

The generated OpenAPI spec lives in `api/docs/` and is served at `/swagger/index.html`
when the API runs.

## Repositories

| Repository | What it holds |
|---|---|
| [`github.com/Muhammadrizooka/birga`](https://github.com/Muhammadrizooka/birga) | this API (Go); the old `gitlab.com/loyihalar/birga/backend` is a secondary mirror |
| `birga/devops` (local, not yet pushed) | Docker Compose stack for servers: proxy, monitoring, analytics, backups |

## Keeping these docs current

Every change to the code updates the related document here in the same commit or pull
request: architecture, setup, API, data model or behaviour. If the changed area has no
document yet, add one and link it from the table above. See `CLAUDE.md` at the repository
root.
