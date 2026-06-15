# LobeHub → pREST Query API (Tier 1 + Tier 2 migration)

This directory contains the SQL templates and configuration that replace
the relevant LobeHub tRPC routers (`apps/server/src/routers/lambda/...`)
with pREST HTTP endpoints.

| Method | URL | BFF router it replaces | Tier |
|--------|-----|------------------------|------|
| `GET`    | `/_QUERIES/lobehub/sessionsListGrouped`            | `session.getGroupedSessions`        | 2 |
| `GET`    | `/_QUERIES/lobehub/topicsListBySession`            | `topic.queryTopics`                  | 2 |
| `GET`    | `/_QUERIES/lobehub/messagesListByTopic`            | `message.getMessages`                | 2 |
| `GET`    | `/_QUERIES/lobehub/agentsListWithStats`            | `agent.queryAgents`                  | 2 |
| `GET`    | `/_QUERIES/lobehub/usageAggregateByDay`            | `usage.findByDateRange`              | 2 |
| `GET`    | `/_QUERIES/lobehub/notificationsListWithDeliveries` | `notification.list`                | 2 |
| `*`      | `/lobehub/public/{table}`                          | (most flat user-scoped reads)        | 1 |

All endpoints require a valid `ory_kratos_session` cookie (the
[auth.kratos] block in `cmd/prestd/prest.toml` validates it). The
identity ID is then made available to SQL templates as the template
variable `userId` (see `controllers/sql.go::extractContextValues`), and
auto-injected as `WHERE <column> = <id>` for every Tier 1 table listed
under `[[auth.user_id_filters]]`.

## Convention

- Folder = `lobehub` (the URL `queriesLocation` segment).
- Filename suffix = HTTP verb: `*.read.sql` for GET, `*.write.sql` for
  POST, `*.update.sql` for PUT/PATCH, `*.delete.sql` for DELETE.
- Template helpers: see `template/funcregistry.go`.
  - `{{ sqlVal "key" }}` – emit `$N` and register the value.
  - `{{ sqlList "key" }}` – emit `( $1, $2, … )` for a slice.
  - `{{ ident "key" }}` – emit a quoted identifier.
  - `{{ limitOffset page size }}` – emit `LIMIT/OFFSET` from page/size.
  - `{{ defaultOrValue "key" "fallback" }}` – fallback if key is missing.
  - `{{ isSet "key" }}` – check whether a key is present.

## Tier 1 (generic table CRUD)

For a `[[auth.user_id_filters]]` entry like:

```toml
[[auth.user_id_filters]]
database = "lobehub"
schema   = "public"
table    = "sessions"
column   = "user_id"
```

…the following BFF calls become:

```http
# BFF previously:
trpc.lambda.session.getGroupedSessions.useQuery()

# pREST now:
GET /lobehub/public/sessions?_order=pinned%20DESC,updated_at%20DESC&_page=1&_size=20
Cookie: ory_kratos_session=<session-token>
```

The same works for POST/PATCH/DELETE – include `user_id` in the JSON
body for writes (it isn't auto-injected for mutations).

## Tier 2 (custom SQL templates)

```http
GET /_QUERIES/lobehub/topicsListBySession?sessionId=abc123&page=1&size=20
Cookie: ory_kratos_session=<session-token>
```

Sample curl against a local pREST (port 3000):

```bash
curl -s \
  -H "Cookie: ory_kratos_session=$KRATOS_SESSION" \
  'http://localhost:3000/_QUERIES/lobehub/topicsListBySession?sessionId=abc123&size=10' \
  | jq
```

## Workspace scope (TODO)

The LobeHub tables also have a `workspace_id` column for shared
workspaces. pREST's current `user_id_filters` only handles a single
column. For workspace-shared reads, the SQL templates here are
intentionally narrow (personal-scope `user_id` filter). Adding a
`[[auth.workspace_id_filters]]` block (or a per-request workspace
membership lookup) is a follow-up.
