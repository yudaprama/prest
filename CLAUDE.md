# pREST — LobeHub Migration Fork

This fork adds LobeHub server-side CRUD/query endpoints to pRESTd, with multi-tenant isolation via Kratos session auth. For release procedures, see `RELEASE.md` (one-shot `./scripts/release.sh`). For the upstream pREST query API, see the source — this document is about what is **different on this fork**.

## What is different vs upstream `prest/prest`

| File | Purpose |
|---|---|
| `context/keys.go::UserIDKey` | Context key carrying the authenticated Kratos identity ID through the middleware chain. |
| `controllers/sql.go::extractContextValues` | Copies `pctx.UserIDKey` into template data as the `userId` variable so SQL templates can use `{{ sqlVal "userId" }}`. |
| `controllers/sql_userid_test.go` | Tests for the helper above. |
| _(runtime config)_ | The single pREST config is the **parent repo's root `prest.toml`** (`../../prest.toml`), loaded via `PREST_CONF=./prest.toml`. There is no longer a `cmd/prestd/prest.toml` template — it was a stale duplicate that diverged from the runtime config and has been removed. It carries the 84 lobehub + 2 plano + 2 kratos `[[auth.user_id_filters]]`, the 4 `[[auth.workspace_id_filters]]`, `[keto]`, and (when activated) `[[auth.workspace_compat_filters]]`. URLs are blank; resolved at runtime from env. |
| `context/keys.go::WorkspaceIDActiveKey` | Context key carrying the single active workspace id (from the `X-Workspace-Id` header) for the compat filter mode. |
| `middlewares/workspaceactive.go` | `WorkspaceActiveMiddleware` — copies `X-Workspace-Id` into `WorkspaceIDActiveKey` (set by the BFF after its Keto Check). |
| `adapters/postgres/workspacefilter.go::ResolveWorkspaceCompat` + `postgres.go::WhereByRequest` | Active-workspace ("compat") filter: for tables in `[[auth.workspace_compat_filters]]`, emits `workspace_id = $ws` (active) or `user_id = $uid AND workspace_id IS NULL` (personal) — mirrors LobeHub `buildWorkspaceWhere`. Suppresses the plain `user_id` filter for those tables. No Keto call on the read path. |
| `config/config.go::WorkspaceCompatConfig` + `ValidateWorkspaceCompat` | Config struct for compat entries + startup check rejecting a table listed in both `user_id_filters` and `workspace_compat_filters`. |
| `config/config.go::loadDotEnv` | Calls `godotenv.Load()` before viper. `.env` in CWD is auto-loaded (absent file = silent no-op). |
| `config/config.go::parseDBConfig` | Overrides each `[[pg.urls]]` URL from `PREST_PG_URL_<NAME>` env var; legacy array form uses `PREST_PG_URL_<N>`. |
| `etc/queries/lobehub/*.read.sql` | Tier 2 SQL templates (joined/aggregate reads). |
| `scripts/release.sh` | One-shot release — `./scripts/release.sh` to auto-bump and ship. |
| `RELEASE.md` | 60-second release guide for humans and agents. |

## Runtime config — important deviation from upstream

`cmd/prestd/main.go` does **not** `//go:embed prest.toml`. The binary reads `prest.toml` from disk at startup (path via `PREST_CONF` or `./prest.toml` in CWD). Both `prest.toml` and `.env` are runtime files, not compile-time inputs.

### Secret resolution precedence

1. `DATABASE_URL` (or `PREST_PG_URL`) — overrides `[pg].url`
2. `PREST_PG_URL_<NAME>` (uppercased, dashes→underscores) — overrides each `[[pg.urls]]` entry by `name`. Examples: `PREST_PG_URL_YARSEW`, `PREST_PG_URL_KRATOS`, `PREST_PG_URL_LOBEHUB`
3. `PREST_PG_URL_<N>` — overrides legacy `pg.urls[i]` by zero-based index
4. URL in `prest.toml` (fallback — should be empty in committed files)

`prest.toml` ships with `url = ""` for every connection. Real values come from `.env` (dev) or orchestrator env vars (prod). `.env` is in `.gitignore`; `.env.example` documents the variable names.

If both `prest.toml` and the env var are blank, that connection is silently skipped at registration (`cmd/prestd/main.go::registerExtraURLs`).

### Historical leak — rotate the credential

A Supabase password was previously committed in cleartext. **Rotate it at the provider** — `git log -p` still shows it. The working tree is sanitised; the history is not.

## Tenant isolation summary

| Surface | How isolation is enforced |
|---|---|
| `GET /lobehub/public/{table}` | `user_id_filters` injects `WHERE <col> = <identity_id>`. |
| `POST/PATCH/DELETE /lobehub/public/{table}` | Caller must include `user_id` in JSON body; no auto-inject on writes. |
| `GET /_QUERIES/lobehub/{script}` | Template binds `{{ sqlVal "userId" }}` from the Kratos-injected identity. |
| `/_health`, `/_QUERIES/public/*` | Public paths — bypass Kratos, no filter. |

Filter is **silently skipped** when no matching `[[auth.user_id_filters]]` entry exists, when `pctx.UserIDKey` is empty on the request, or when the user_id is empty. This means a deployment without an auth layer is **not safe** — the filter becomes a no-op. The standard deployment fronts pREST with **Ory Oathkeeper** (:4455), which validates the Kratos session and injects `X-User-Id` authoritatively; pREST must NOT be reachable directly (a client could otherwise spoof `X-User-Id`).

### Registered LobeHub Tier 1 tables (31)

All `database = "lobehub"`, `schema = "public"`. The 31 registered tables
(filter column in parentheses; default `user_id` unless noted):

```
users (id), user_settings (id),
push_tokens, async_tasks, api_keys, session_groups, sessions, agents,
agents_to_sessions, agents_files, agents_knowledge_bases,
topics, messages, message_groups, message_plugins, message_translates,
message_tts, threads,
chat_groups, chat_groups_agents,
documents, files, knowledge_bases, knowledge_base_files,
generation_topics, generation_batches, generations,
tasks (created_by_user_id), notifications,
ai_models, ai_providers
```

`tasks` filters on `created_by_user_id` (not the default `user_id`); every
other entry uses `user_id`. Two batches added Jun 16 2026:

- **Batch 1 (21→25):** `threads`, `message_plugins`, `message_translates`,
  `message_tts` (all mirror the `messages` pattern with non-null `user_id`
  referencing `users.id`).
- **Batch 2 (25→31):** `agents_files`, `agents_knowledge_bases`,
  `chat_groups`, `chat_groups_agents` (junction), `ai_models`, `ai_providers`.

## Workspace scope — Phase 1 gate REMOVED (→ Oathkeeper); Phase 2 + Phase 3 data-scope remain

LobeHub tables carry a `workspace_id` column for shared workspaces. Authentication and single-workspace authorization (the old Phase 1 gate) now live in **Ory Oathkeeper** (the edge proxy on :4455); pREST keeps only the **data-scope** mechanisms:

- **Phase 1 (REMOVED)**: the `WorkspaceAuthzGate` middleware and the `/authz/check` endpoint have been deleted. Single-workspace authorization is now an Oathkeeper `remote_json` → Keto `Check` (gated rule in `oathkeeper-access-rules.yml`, enabled when Keto is up). The `?workspaceId=` → `pctx.WorkspaceIDKey` template-var path is now vestigial (always empty); use the Phase 3 active-workspace header for single-workspace scoping.
- **Phase 2** (`[auth] workspace_filters_enabled`): `WorkspaceMembershipResolver` resolves the caller's workspace list via Keto `ListObjects` (LRU-cached 30s) and stores it in `pctx.WorkspaceIDsKey`. The postgres adapter injects `WHERE workspace_id IN (...)` on the 4 workspace tables configured in `[[auth.workspace_id_filters]]`. The `workspaceScopeIn` template helper emits the same IN-clause for cross-workspace Tier 2 reads.
- **Phase 3 — active-workspace ("compat")** (`[[auth.workspace_compat_filters]]` + `[auth] workspace_active_header`): for workspace-capable **content** tables (`documents`, `files`, `agents`, `sessions`, `topics`, `messages`), mirrors LobeHub `buildWorkspaceWhere` exactly — `workspace_id = $ws` when Oathkeeper injects `X-Workspace-Id` (after its own Keto Check), else `user_id = $uid AND workspace_id IS NULL`. Distinct from Phase 2: single active workspace, not union; and **no Keto call on the read path** (the header is trusted, pre-authorized). A table in `workspace_compat_filters` is removed from `user_id_filters`; `ValidateWorkspaceCompat` rejects listing it in both. Activation requires deploying a compat-enabled binary before moving the 6 tables in the runtime config (otherwise they'd be unscoped on older binaries). **Activated 2026-06-26**: the 6 content tables now live in `[[auth.workspace_compat_filters]]` in the root `prest.toml`; planoctl deploys `prest v2.2.1` (compat-enabled), so the binary-before-config ordering is satisfied.

All phases are gated off by default. See `WORKSPACE_SCOPE_IMPLEMENTATION_PLAN.md` for the full design.

## Views as an alternative to SQL templates

`/{db}/{schema}/{view}` works for base tables and views alike — same `pREST` URL surface, supports `_where`, `_order`, `_page`, `_size`, `_count` natively. Recommended split:

- **Views** (`v_sessions_grouped`, `v_topics_by_session`, `v_messages_by_topic`, `v_agents_with_stats`, `v_notifications_with_deliveries`) — no per-request params.
- **SQL templates** (`etc/queries/lobehub/*.read.sql`) — anything that needs `{{ sqlVal }}` (e.g. `?sessionId=…` or range parameters).

View approach: hard-code `WHERE user_id = $1` and read the placeholder from the same `UserIDKey` context plumbing as `WhereByRequest`. Add a `[[auth.user_id_filters]]` row for the view pointing at the same column as the underlying table.

## Testing

```bash
# Fast — no DB required
go test -run TestExtractContextValues ./controllers/...
go test ./template/...
go test -run "TestParseDBConfig|Test_pgURLEnvKey" -v ./config/

# Full suite — needs live PostgreSQL (docker-compose-test.yml)
go test ./...
```

The 3 pre-existing `TestParse` failures (lines 33-61) look for `prest-test` database but find `prest` (default). Unrelated to this fork.

## Build & run

```bash
go build ./cmd/prestd/
cp .env.example .env   # fill in real URLs
PORT=3000 ./prestd
```

Or in production: ship the binary + `prest.toml`; inject env vars via orchestrator secrets.

## Release

See `RELEASE.md` for the full guide. One-liner:

```bash
./scripts/release.sh           # auto-bump patch + ship
./scripts/release.sh 2.1.0     # explicit version
./scripts/release.sh --dry-run # show what would happen
```

CI does the rest: `.github/workflows/release.yml` publishes 6-platform binaries to GitHub Releases; `.github/workflows/build.yml` pushes Docker images (requires `DOCKER_LOGIN` + `DOCKER_PASSWORD` secrets for Docker Hub; GHCR uses built-in `GITHUB_TOKEN`).
