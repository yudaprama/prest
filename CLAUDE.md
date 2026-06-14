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

## Views and materialised views

PostgreSQL views are queryable through pREST exactly like base tables
— the same `/{db}/{schema}/{object}` URL path works. This opens an
alternative implementation path for the Tier 2 SQL templates:

| Surface | URL | Notes |
|---------|-----|-------|
| `GET /lobehub/public/v_sessions_grouped` | view | Single URL, supports `_where`, `_order`, `_page`, `_size`, `_count` natively. No `/_QUERIES/*` indirection. |
| `GET /lobehub/public/mv_usage_by_day` | materialised view | Heavy aggregate pre-computed on refresh. Trivially fast. |

Views are recommended over SQL templates when the query:

- Has no per-request parameters that the user controls (other than the
  standard `_where` / `_order` / `_page` pREST params).
- Doesn't need the `{{ sqlVal "userId" }}` template to be re-evaluated
  per request (the view can hard-code `WHERE user_id = $1` and read
  the placeholder through the same `UserIDKey` context plumbing that
  `WhereByRequest` uses for tables).

To migrate a SQL template to a view:

```sql
-- In a Drizzle migration or a one-off `psql` apply:
CREATE OR REPLACE VIEW public.v_sessions_grouped AS
SELECT
    s.id, s.slug, s.title, /* … */
    COALESCE(t.cnt, 0)::int AS topic_count
FROM   sessions s
LEFT JOIN session_groups g ON g.id = s.group_id
LEFT JOIN (
    SELECT session_id, COUNT(*) AS cnt
    FROM   topics
    GROUP  BY session_id
) t ON t.session_id = s.id;
-- pREST handles the user_id filter via the existing
-- [[auth.user_id_filters]] entry; add a row for v_sessions_grouped
-- pointing at the same column as sessions.
```

The view approach has two downsides:

1. **No template parameters** — anything that needs `{{ sqlVal "x" }}`
   (e.g. `?sessionId=…`) still has to be a SQL template.
2. **Schema migration cost** — views need a Drizzle migration to
   create; SQL templates are just files on disk.

For LobeHub, the recommended split is:

- **Views**: `v_sessions_grouped`, `v_topics_by_session`,
  `v_messages_by_topic`, `v_agents_with_stats`,
  `v_notifications_with_deliveries`.
- **SQL templates**: `usageAggregateByDay` (still needs `startDate` /
  `endDate` range parameters from the caller).

## Tenant filter implementation

The `[[auth.user_id_filters]]` block in `prest.toml` is wired through:

- `config.UserFilterConfig` struct (`config/config.go`).
- `config.UserIDFilters []UserFilterConfig` field, populated by
  `viper.UnmarshalKey("auth.user_id_filters", …)` in `parseAuthConfig`.
- `adapters/postgres.ResolveUserIDColumn(r *http.Request) string` and
  `adapters/postgres.UserIDFromContext(r *http.Request) string` —
  both exported, so middleware can drive them and so the tests in
  `tests/lobehub/userfilter_test.go` can target them without
  importing the package internals.
- `adapters/postgres.postgres.go::WhereByRequest` calls these and
  prepends `WHERE <column> = $N` to the query if and only if both the
  user_id and the column resolution succeed.

The filter is **silently skipped** when:

- The request path doesn't match a `[[auth.user_id_filters]]` entry.
- The auth middleware did not set `pctx.UserIDKey` on the context
  (the upstream LobeHub proxy can guard against that — pREST trusts
  whoever sits in front of it).
- The user_id is the empty string.

This means a deployment without an auth layer is **not safe** — the
filter becomes a no-op. The standard deployment has the Kratos
middleware in front of pREST; for local development, set
`X-User-ID: <id>` and add a tiny middleware that copies it into
`pctx.UserIDKey`.
