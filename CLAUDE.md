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
# pREST Query Guide

This document explains how pREST builds and executes SQL queries from HTTP
requests. It is derived directly from the source code
(`adapters/postgres/postgres.go`, `controllers/*.go`, `template/funcregistry.go`,
`adapters/postgres/statements/queries.go`) and is intended as a single
reference for using the API.

---

## 1. Overview

pREST is a thin layer on top of PostgreSQL. Every HTTP request is translated
into a SQL statement by the **adapter** (PostgreSQL implementation) and then
executed through the standard `database/sql` driver.

The two main entry points are:

| HTTP route                                   | Handler (source)                | Purpose                                   |
|----------------------------------------------|----------------------------------|-------------------------------------------|
| `GET /databases`                             | `controllers/databases.go`       | List databases                             |
| `GET /schemas`                               | `controllers/databases.go`       | List schemas                               |
| `GET /tables`                                | `controllers/tables.go:22`       | List tables (with optional `WHERE` etc.)   |
| `GET /{database}/{schema}/{table}`           | `controllers/tables.go:134`      | Select rows                                |
| `POST /{database}/{schema}/{table}`          | `controllers/tables.go:295`      | Insert rows                                |
| `PUT/PATCH /{database}/{schema}/{table}`     | `controllers/tables.go:…`        | Update rows                                |
| `DELETE /{database}/{schema}/{table}`        | `controllers/tables.go:…`        | Delete rows                                |
| `* /_QUERIES/{queriesLocation}/{script}`     | `controllers/sql.go:44`          | Run user‑defined SQL templates             |

All request‑driven SQL is generated by the adapter functions described below.
The flow for a `GET /{database}/{schema}/{table}` call is:

1. `SelectFromTables` (`controllers/tables.go:134`) validates the path
   identifiers (`ident.IsSafeSegment`).
2. `WhereByRequest`, `OrderByRequest`, `GroupByClause`, `JoinByRequest`,
   `DistinctClause`, `CountByRequest`, and `PaginateIfPossible` build the SQL
   fragments.
3. `SelectSQL` (`adapters/postgres/postgres.go:1845`) prefixes the fragments
   with `SELECT ... FROM "db"."schema"."table"`.
4. `QueryCtx` (or `QueryCountCtx` when `_count_first` is used) executes the
   final statement.

---

## 2. Query Parameters

Every query string parameter is optional. Names are case‑sensitive and start
with an underscore to avoid collisions with real column names.

| Parameter            | Description                                                                 | Generated SQL fragment |
|----------------------|-----------------------------------------------------------------------------|------------------------|
| `_select`            | Comma‑separated column list (default `*`).                                   | `SELECT col1, col2`    |
| `_distinct`          | Use `SELECT DISTINCT`.                                                       | `SELECT DISTINCT …`    |
| `_where`             | Filtering conditions (see syntax below).                                      | `WHERE …`              |
| `_order`             | Ordering, e.g. `col1 ASC,col2 DESC`.                                          | `ORDER BY …`           |
| `_limit` / `_offset` | Hard limits.                                                                 | `LIMIT n OFFSET o`     |
| `_page` / `_size`    | Page‑based pagination. Internally turned into `LIMIT/OFFSET` by `PaginateIfPossible` (`adapters/postgres/postgres.go:993`). | `LIMIT size OFFSET (page-1)*size` |
| `_groupby`           | `GROUP BY` expression.                                                       | `GROUP BY …`           |
| `_having`            | `HAVING` expression, must follow a `_groupby`.                                | `HAVING …`             |
| `_join`              | Join clause, repeatable.                                                      | `<JOIN …>`             |
| `_count`             | Return only the count of matching rows.                                      | `SELECT COUNT(*) …`    |
| `_count_first`       | When used with `_count`, return the result as a single object, not an array.| (handled in `controllers/tables.go:206`) |
| `_fields`            | Restrict the fields the authenticated user can read (ACL).                    | (checked before `SelectFields`) |

### 2.1 `_where` Syntax

`_where` is the most expressive parameter. It is parsed by
`WhereByRequest` (`adapters/postgres/postgres.go:209`). The syntax is:

```
?<column>[:<operator>]=[<value>]
```

Supported operators (see the test table at
`adapters/postgres/postgres_test.go:199` for full examples):

| Operator    | Meaning (example)                                   |
|-------------|-----------------------------------------------------|
| `eq` (default) | `?id=10` → `"id" = $1`                          |
| `gt`/`gte`   | `?age:gt=18` → `"age" > $1`                       |
| `lt`/`lte`   | `?age:lt=65` → `"age" < $1`                       |
| `neq`        | `?status:neq=inactive` → `"status" <> $1`         |
| `like`       | `?name:like=%john%` → `"name" LIKE $1`            |
| `ilike`      | Case‑insensitive `LIKE`.                            |
| `in`         | `?id:in=1,2,3` → `"id" IN ('1','2','3')` (template `inFormat`) |
| `nin`        | Not‑in list.                                          |
| `isnull`     | `?deleted_at:isnull=` → `"deleted_at" IS NULL`   |
| `notnull`    | `?deleted_at:notnull=` → `"deleted_at" IS NOT NULL` |
| `tsquery`    | `?name:tsquery=prest` → `name @@ to_tsquery('prest')` |
| `tsvector`   | Full‑text search via `@@ to_tsvector`.              |
| `lquery`/`ltxtquery`/`ltreematch` | `ltree` operators (`~`, `@`, `@>`). |
| `$at`        | `?col:like:$at=%john%` → placeholder (`$N`) binding, the value is kept in the bound parameters instead of inlined. |
| `or`         | `?col:or:eq=val1|val2` → `col = $1 OR col = $2` (see feature commit f755c12) |

Multiple `_where` parameters are joined with `AND` unless an `or` operator is
used. Values are always bound through PostgreSQL placeholders (`$1`, `$2`,
…); they are never concatenated into the SQL string.

### 2.2 `_join` Syntax

Joins are added with `_join`. The value is passed verbatim to the SQL after
the main `FROM` clause (see `JoinByRequest` – `adapters/postgres/postgres.go:732`).

```
?_join=INNER JOIN orders o ON o.user_id = u.id
?_join=LEFT JOIN payments p ON p.user_id = u.id
```

### 2.3 Pagination

`PaginateIfPossible` (`adapters/postgres/postgres.go:993`) accepts `_page` and
`_size` (or `_limit`/`_offset`). If the parameters are not valid integers the
function falls back to defaults instead of returning an error, ensuring that a
malformed request still returns a result.

---

## 3. Building the SQL (Code Walkthrough)

The following functions live in `adapters/postgres/postgres.go` and together
compose every dynamic query:

| Function | File:Line | Responsibility |
|----------|-----------|----------------|
| `WhereByRequest` | `postgres.go:209` | Parses every `_where=…` into `WHERE` fragments and bound parameters. |
| `JoinByRequest`  | `postgres.go:732` | Returns a slice of raw `JOIN …` strings. |
| `SelectFields`   | `postgres.go:783` | Turns a `[]string` of column names into a quoted, comma‑separated list. |
| `OrderByRequest` | `postgres.go:820` | Parses `_order` into `ORDER BY …` syntax. |
| `CountByRequest` | `postgres.go:854` | Returns a `SELECT COUNT(*)` prefix. |
| `PaginateIfPossible` | `postgres.go:993` | Builds `LIMIT/OFFSET` from `_page`/`_size` or `_limit`/`_offset`. |
| `DistinctClause` | `postgres.go:1734` | Returns the `DISTINCT` keyword if requested. |
| `GroupByClause`  | `postgres.go:1746` | Returns the `GROUP BY` clause. |
| `SelectSQL`      | `postgres.go:1845` | Composes the final `SELECT … FROM "db"."schema"."table"`. |
| `QueryCtx` / `QueryCountCtx` | `postgres.go:…` | Executes the SQL with a timeout context. |

The static base statements are defined in
`adapters/postgres/statements/queries.go` (e.g. `SelectInTable`,
`InsertQuery`, `DeleteQuery`, `UpdateQuery`, `GroupBy`, `Having`).

---

## 4. Custom SQL Scripts (`_QUERIES`)

Files placed under the configured **queries path** (see `config/config.go`)
can be executed through the `/_QUERIES/...` endpoint. The controller
`ExecuteFromScripts` (`controllers/sql.go:44`) builds a `templateData`
map containing:

* `header.<Name>` – every HTTP header.
* `<param>` – every URL query parameter, as a scalar or a slice.

The template engine (`template/funcregistry.go`) exposes helpers that
produce safe, parameterized SQL:

| Helper | Purpose |
|--------|---------|
| `{{ sqlVal <key> }}` | Emits a positional placeholder (`$1`, `$2`, …) and stores the value in the `Args` slice. |
| `{{ sqlList <key> }}` | Emits `( $1, $2, … )` for slice values, registering each element. |
| `{{ ident <key> }}` | Validates and double‑quotes an identifier (column/table name). |
| `{{ inFormat <key> }}` | Legacy helper that inlines a list as `('a','b')` (use `sqlList` for safety). |
| `{{ limitOffset page size }}` | Generates `LIMIT n OFFSET (page-1)*n`. |

`ExtractQueryParameters` (`controllers/sql.go:92`) is responsible for
populating the template data from the request URL.

### Example Script (`etc/queries/sales/get_total.sql`)

```sql
SELECT {{ sqlVal "client_id" }} AS client_id,
       {{ sqlList "status" }}   AS statuses,
       {{ ident "amount_col" }} AS amount
FROM   {{ ident "schema" }}.{{ ident "table" }}
WHERE  created_at >= {{ sqlVal "since" }};
```

Call:

```bash
curl "http://localhost:3000/_QUERIES/sales/get_total?client_id=42&status=paid,refunded&amount_col=total&schema=public&table=orders&since=2025-01-01"
```

The placeholders are bound through `database/sql`; no user input is ever
concatenated into the SQL string.

---

## 5. Example Requests

```bash
# Select all rows
curl http://localhost:3000/mydb/public/users

# Filter and order
curl "http://localhost:3000/mydb/public/users?_where=age:gt=18&_order=name%20ASC"

# Pagination
curl "http://localhost:3000/mydb/public/users?_page=2&_size=20"

# Distinct with a join
curl "http://localhost:3000/mydb/public/orders?_distinct&_join=INNER%20JOIN%20users%20u%20ON%20u.id%3Dorders.user_id"

# Count
curl "http://localhost:3000/mydb/public/users?_count"

# Full‑text search
curl "http://localhost:3000/mydb/public/articles?_where=body:tsquery=prest"

# Insert
curl -X POST http://localhost:3000/mydb/public/users \
     -H "Content-Type: application/json" \
     -d '{"name":"ada","email":"ada@example.com"}'

# Update
curl -X PATCH "http://localhost:3000/mydb/public/users?id:eq=1" \
     -H "Content-Type: application/json" \
     -d '{"name":"ada lovelace"}'

# Delete
curl -X DELETE "http://localhost:3000/mydb/public/users?id:eq=1"
```

---

## 6. Security Notes

* Path identifiers (`database`, `schema`, `table`) are validated by
  `ident.IsSafeSegment` before they reach the SQL builder.
* All user supplied values are sent as bound parameters (`$1`, `$2`, …).
* `ident` (used in custom scripts) rejects identifiers that contain quotes
  or unsupported characters, preventing SQL injection through column/table
  names.
* Field‑level access control is enforced by `FieldsPermissions` (see
  `controllers/tables.go:163`).

---

## 7. References in the Code

* `controllers/tables.go:134` – `SelectFromTables` (GET handler)
* `controllers/tables.go:295` – `InsertInTables` (POST handler)
* `controllers/sql.go:44` – `ExecuteFromScripts` (custom scripts)
* `adapters/postgres/postgres.go:209` – `WhereByRequest`
* `adapters/postgres/postgres.go:732` – `JoinByRequest`
* `adapters/postgres/postgres.go:783` – `SelectFields`
* `adapters/postgres/postgres.go:820` – `OrderByRequest`
* `adapters/postgres/postgres.go:854` – `CountByRequest`
* `adapters/postgres/postgres.go:993` – `PaginateIfPossible`
* `adapters/postgres/postgres.go:1734` – `DistinctClause`
* `adapters/postgres/postgres.go:1746` – `GroupByClause`
* `adapters/postgres/postgres.go:1845` – `SelectSQL`
* `adapters/postgres/statements/queries.go` – static SQL fragments
* `template/funcregistry.go:96` – `sqlVal`, `sqlList`, `ident`

---

## 8. Multi-Database Connection Support

pREST supports connecting to multiple PostgreSQL databases simultaneously. Configure additional databases in `prest.toml`:

```toml
[pg]
url = "postgresql://user:pass@host:5432/maindb?sslmode=require"
single = false

[[pg.urls]]
name = "yarsew"
url = "postgresql://user:pass@host1:5432/db1?sslmode=require"

[[pg.urls]]
name = "ogmami"
url = "postgresql://user:pass@host2:5432/db2?sslmode=require"
```

### 8.1 Architecture

* **Connection Pool**: `map[string]*sqlx.DB` with mutex protection (`adapters/postgres/internal/connection/conn.go`)
* **Name Resolution**: Logical name → actual database name mapping via `ResolveDBName()`
* **Context Propagation**: Database name passed via `pctx.DBNameKey` in request context
* **Registration**: Named URLs registered at startup in `cmd/prestd/main.go:registerExtraURLs()`

### 8.2 Usage

Query specific database by using its name in the URL path:

```bash
# Query yarsew database
curl http://localhost:3000/yarsew/public/users

# Query ogmami database
curl http://localhost:3000/ogmami/public/orders
```

### 8.3 Stress Testing

Stress test scripts for multi-database switching are in `test/stress/`:

| Script | Purpose |
|--------|---------|
| `db_switch_health_check.sh` | Validate both databases accessible |
| `db_switch_basic_test.sh` | Sequential DB switching |
| `db_switch_concurrent_test.sh` | Parallel workers testing pool safety |
| `db_switch_load_test.sh` | Sustained load with latency metrics |
| `db_switch_go_test.go` | Go integration tests |
| `run_all_tests.sh` | Master runner with summary |

```bash
# Quick validation
make -C test/stress stress-test-quick

# Full suite
./test/stress/run_all_tests.sh

# Go tests
go test -v -short ./test/stress/...
```

---

## 9. User ID Auto-Filter (Tenant Isolation)

pREST can automatically inject a `WHERE` filter on a configured column using the
user ID from an HTTP header. This is useful when pREST sits behind an auth
layer that has already validated the request and forwards the user identity
(e.g. as `X-User-Id`).

### 9.1 Configuration

Configure per-table in `prest.toml`:

```toml
[auth]
user_id_header = "X-User-Id"

[[auth.user_id_filters]]
database = "yarsew"
schema   = "public"
table    = "billing_balances"
column   = "actor_id"

[[auth.user_id_filters]]
database = "ogmami"
schema   = "public"
table    = "sessions"
column   = "identity_id"
```

* `auth.user_id_header` — header to read the user ID from (e.g. `X-User-Id`).
* `[[auth.user_id_filters]]` — array of per-table rules. `database`,
  `schema` and `table` must match the URL path; `column` is the column to
  filter on.
* Tables not listed in `[[auth.user_id_filters]]` are **never** auto-filtered.

### 9.2 Architecture

* **Middleware**: `middlewares/userfilter.go` — `UserFilterMiddleware` reads
  the configured header and stores the value in `r.Context()` under
  `pctx.UserIDKey`.
* **Resolver**: `adapters/postgres/userfilter.go` — `resolveUserIDColumn()`
  parses the URL path (`/{database}/{schema}/{table}`) and looks up the
  matching `UserFilterConfig`.
* **Injection**: `adapters/postgres/postgres.go` (in `WhereByRequest`) —
  prepends `"<column>" = $N` and appends the user ID to the bound args when
  the context has a user ID and the target table is configured.

### 9.3 Generated SQL

```bash
# Request
curl -H "X-User-Id: user-alice" \
  "http://localhost:3000/yarsew/public/billing_balances?_where=status:eq=active"

# Resulting SQL
SELECT ... FROM "yarsew"."public"."billing_balances"
 WHERE "actor_id" = $1 AND "status" = $2
Args: [user-alice, active]
```

### 9.4 Behavior

| Request | Header | Table in config? | Result |
|---|---|---|---|
| `GET /yarsew/public/billing_balances` | `X-User-Id: x` | yes | `WHERE "actor_id" = $1` |
| `GET /yarsew/public/conversation_states` | `X-User-Id: x` | no | no injection (safe) |
| `GET /yarsew/public/orders` | (none) | yes | no injection |
| `auth.user_id_header` not set | any | — | middleware passes through |

### 9.5 Tests

```bash
PREST_CONF=/path/to/prest.toml go test -run \
  "TestUserFilter|TestResolveUserIDColumn|TestWhereByRequest_UserFilter" \
  ./adapters/postgres/ ./middlewares/
```

* `middlewares/userfilter_test.go` — header extraction, context propagation,
  custom header names, negroni integration.
* `adapters/postgres/userfilter_test.go` — resolver matching, SQL injection,
  placeholder numbering, `_or` clause compatibility.

---

*Last updated: derived from source tree at the time of writing.*
