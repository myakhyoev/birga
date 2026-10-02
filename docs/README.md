# Birga backend documentation

Birga is a mobile app for caregivers in Uzbekistan with children aged 2 to 6. Each day it
suggests one short, adult-led offline activity, chosen by simple rules (child age, the
caregiver's goal, available time), and tracks completions and streaks. Content is in Uzbek
and Russian.

This repository is the Go API behind the app. Today it serves the activity catalogue and a
small admin API for adding activities; accounts, child profiles, completions and phone
sign-in are the next features.

## Start here

| Document | Read it when you want to |
|---|---|
| [setup.md](setup.md) | run the API and database on your machine, configure it, run tests |
| [architecture.md](architecture.md) | understand the layers, how a request flows, and how to add a feature |
| [api.md](api.md) | call the API: envelope, error codes, endpoints, admin auth |
| [data-model.md](data-model.md) | see the database tables, constraints and migrations |
| [operations.md](operations.md) | build, deploy, monitor and back up the service |

The generated OpenAPI spec lives in `api/docs/` and is served at `/swagger/index.html`
when the API runs.

## Repositories

| Repository | What it holds |
|---|---|
| `gitlab.com/loyihalar/birga/backend` | this API (Go) |
| `birga/devops` (local, not yet on GitLab) | Docker Compose stack for servers: proxy, monitoring, analytics, backups |

## Keeping these docs current

Every change to the code updates the related document here in the same commit or merge
request: architecture, setup, API, data model or behaviour. If the changed area has no
document yet, add one and link it from the table above. See `CLAUDE.md` at the repository
root.
