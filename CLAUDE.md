# pREST — LobeHub Migration Guide

This fork/branch of pRESTd carries additions that migrate LobeHub
server-side CRUD and query endpoints out of the Next.js BFF
(`lobehub/apps/server/src/routers/lambda/*`) into pREST, while
preserving multi-tenant isolation via Kratos session auth.

## What is added on this branch

| File / area | Purpose |
|---|---|
| `context/keys.go::UserIDKey` | Context key that carries the authenticated Kratos identity ID through the middleware chain. |
| `controllers/sql.go::extractContextValues` | Copies `pctx.UserIDKey` into the template data as the `userId` variable so SQL templates can use `{{ sqlVal "userId" }}`. |
| `controllers/sql_userid_test.go` | Unit tests for the helper above. |
| `cmd/prestd/prest.toml` | Embedded configuration with Kratos auth **enabled** and a new `[[auth.user_id_filters]]` block per LobeHub table. Force-added over the upstream `.gitignore`. |
| `etc/queries/README.md` | Top-level docs for custom SQL templates. |
| `etc/queries/lobehub/*.read.sql` | Tier 2 SQL templates replacing the joined/aggregate LobeHub routers. |
| `etc/queries/lobehub/README.md` | Per-subdirectory API reference. |

## Upstream vs. this branch

The upstream `prest/prest` project has:
- `[[auth.user_id_filters]]` already implemented in `middlewares/` and
  `adapters/postgres/userfilter.go` (tenant filter auto-injection for
  generic table endpoints).
- `/_QUERIES/{queriesLocation}/{script}` endpoint that executes SQL
  templates with `{{ sqlVal }}`, `{{ sqlList }}`, `{{ ident }}`,
  `{{ limitOffset }}`, etc.

This branch adds:
- **`pctx.UserIDKey` + `extractContextValues`** so the identity ID is
  visible to SQL templates (previously it was only used by the
  userfilter auto-injection).
- The `lobehub` database connection and the Tier 1 filter rules in
  `cmd/prestd/prest.toml`.
- The Tier 2 SQL templates.

## Building & running

```bash
cd prest
go build ./cmd/prestd/
PORT=3000 ./prestd
```

The `//go:embed prest.toml` in `cmd/prestd/main.go` bakes
`cmd/prestd/prest.toml` into the binary, so no runtime config file is
needed. To update Tier 1 entries, edit `cmd/prestd/prest.toml` and
rebuild.

## Testing

```bash
go test -run TestExtractContextValues ./controllers/...
go test -v ./template/...
```

The rest of the controllers tests require a live PostgreSQL (see
`docker-compose-test.yml`).

## Tenant isolation summary

| Request surface | How isolation is enforced |
|---|---|
| `GET /lobehub/public/{table}` | `user_id_filters` injects `WHERE <col> = <identity_id>` (in `userfilter.go`). |
| `POST/PATCH/DELETE /lobehub/public/{table}` | Caller must include `user_id` in the JSON body; `user_id_filters` does **not** auto-inject for writes. |
| `GET /_QUERIES/lobehub/{script}` | The template binds `{{ sqlVal "userId" }}` which resolves to the identity ID injected by the Kratos middleware. |
| `/_health`, `/_QUERIES/public/*` | Public paths (bypass Kratos); no tenant filter applied. |

## Workspace scope (open item)

LobeHub tables also carry a `workspace_id` column for shared
workspaces. `user_id_filters` only handles a single column, so the
current surface is intentionally personal-scope. A follow-up
`[[auth.workspace_id_filters]]` block (or a per-request membership
lookup in the Kratos middleware) is tracked in
`/Users/yuda/ai-orchestration/CLAUDE.md` under "LobeHub → pREST
Migration".
