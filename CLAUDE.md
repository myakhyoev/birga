# CLAUDE.md

Guidance for Claude (and any contributor) working in this repository.

## Documentation rule

Technical documentation lives as Markdown in `docs/` (index: `docs/README.md`).

- Every code change updates the related docs in the **same commit / merge request**:
  architecture, setup and configuration, API, data model, behaviour.
- If no doc covers the changed area, create one in `docs/` and link it from `docs/README.md`.
- Mention the doc updates in the merge request description.
- A change is not done until its docs match the code.

Where things usually go:

| Change | Update |
|---|---|
| new or changed endpoint, request/response, error code | `docs/api.md` (and `make swag-init`) |
| new migration, table, column, constraint | `docs/data-model.md` |
| new env variable or Makefile target, tooling change | `docs/setup.md` (and `.env.example`) |
| new layer, package, pattern, driver | `docs/architecture.md` |
| Dockerfile, CI, metrics, deployment | `docs/operations.md` |

## Project rules

- Birga is a personal project: never import company-internal libraries or use company hosts
  (registries, Go proxies). Write what is missing in `pkg/`.
- Follow the layering in `docs/architecture.md`: use cases declare the interfaces they need;
  `dbstore` and `drivers` satisfy them.
- `api/docs/` is generated; regenerate it with `make swag-init`, never edit it by hand.
