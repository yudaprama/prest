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
| `GET`   | `/_QUERIES/lobehub/threadMessages`                 | `thread.getThreads`                   | 2 |
| `GET`   | `/_QUERIES/lobehub/agentFilesByAgent`              | `agentDocument.listDocuments`        | 2 |
| `GET`   | `/_QUERIES/lobehub/connectorToolsByConnector`      | `connector.list` (tools sublist)      | 2 |
| `GET`   | `/_QUERIES/lobehub/verifyResultsWithRubric`        | `verify.listResults`                  | 2 |
| `GET`   | `/_QUERIES/lobehub/generationBatchesWithGenerations`| `generationBatch.getGenerationBatches`| 2 |
| `GET`   | `/_QUERIES/lobehub/knowledgeBaseFilesWithChunks`   | `file.getKnowledgeItems`             | 2 |
| `GET`   | `/_QUERIES/lobehub/agentSkillsWithResources`       | `agentSkills.list` / `listResources` | 2 |
| `GET`   | `/_QUERIES/lobehub/messagesSearchFts`              | `message.searchMessages` (FTS, replaces BM25) | 2 |
| `GET`   | `/_QUERIES/lobehub/topicsSearchFts`                | `topic.searchTopics` (FTS, replaces BM25)     | 2 |
| `GET`   | `/_QUERIES/lobehub/documentsWithHistory`           | `document.listDocumentHistory`                 | 2 |
| `*`     | `/lobehub/public/{table}`                          | (most flat user-scoped reads)        | 1 |

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

## Template convention — workspace branching

All `.read.sql` that target a `workspace_id`-bearing table accept an
optional `workspaceId` query parameter. When present, the WHERE clause
scopes to that workspace (dropping the `userId` check, matching
LobeHub's `buildWorkspaceWhere`). When absent, the WHERE clause checks
both `user_id = $userId AND workspace_id IS NULL` (personal scope).

```sql
{{- if isSet "workspaceId" }}
WHERE  workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
WHERE  user_id = {{ sqlVal "userId" }} AND workspace_id IS NULL
{{- end }}
```

Tables that are personal-only (`notifications`, `user_memories` and
their sub-tables) use only `user_id = {{ sqlVal "userId" }}` — no
workspace branching needed.

## Full-text search (FTS)

Supabase-hosted lobehub DB does not have the ParadeDB `pg_search`
extension. Migration `0111_add_postgres_fts.sql` (in the lobehub fork)
adds Postgres built-in FTS instead: a STORED generated `*_tsv` column
per searchable table + a GIN index. Search goes through two paths:

### Tier 1 — `$tsquery` filter (no ranking)

pREST's adapter natively supports the `tsquery` filter operator
(`adapters/postgres/postgres.go`):

```http
# All rows where content matches the english tsquery
GET /lobehub/public/messages?content$tsquery=hello+world

# With explicit config
GET /lobehub/public/messages?content$tsquery(english)=deploy
```

Returns rows in table order (no relevance ranking). Use when the caller
just needs "does this row match?".

### Tier 2 — relevance-ranked templates

Use when the caller needs "give me the top-N most relevant rows".
These templates expose `ts_rank()` as a `rank` field and sort
descending:

- `GET /_QUERIES/lobehub/messagesSearchFts?q=...&size=20`
  (against `messages_tsv`)
- `GET /_QUERIES/lobehub/topicsSearchFts?q=...&size=20`
  (against `topics_tsv`)

Both accept the workspace branching pattern documented above, plus an
optional `topicId` / `sessionId` / `agentId` filter. See
`messagesSearchFts.read.sql` and `topicsSearchFts.read.sql` for the
full query-param list.

**Why a CTE for `plainto_tsquery`:** computing it once with
`WITH q AS (SELECT plainto_tsquery(...) AS tsq)` lets us reuse the
same `q.tsq` for both the `@@` predicate and the `ts_rank()` scoring
without a double bind.
