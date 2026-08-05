# pREST — Fork with multi-tenant CRUD/query endpoints

This fork adds multi-tenant CRUD/query endpoints to pRESTd, with tenant isolation via Kratos session auth. For release procedures, see `RELEASE.md` (one-shot `./scripts/release.sh`). For the upstream pREST query API, see the source — this document is about what is **different on this fork**.

## What is different vs upstream `prest/prest`

| File | Purpose |
|---|---|
| `context/keys.go::UserIDKey` | Context key carrying the authenticated Kratos identity ID through the middleware chain. |
| `controllers/sql.go::extractContextValues` | Copies `pctx.UserIDKey` into template data as the `userId` variable so SQL templates can use `{{ sqlVal "userId" }}`. |
| `controllers/sql_userid_test.go` | Tests for the helper above. |
| _(runtime config)_ | The single pREST config is `prest.yaml` (YAML format with `$ENV_VAR` syntax), loaded via `PREST_CONF=./prest.yaml`. pREST resolves `$VAR` placeholders from the process environment at startup. No `PREST_PG_URL_*` injection needed — DSN env vars are read directly. |
| `context/keys.go::TenantIDActiveKey` | Context key carrying the single active tenant id (from the edge-injected `X-Tenant-Id` header) for the compat filter mode. |
| `middlewares/workspaceactive.go` | `TenantActiveMiddleware` — copies the edge-authorized `X-Tenant-Id` into `TenantIDActiveKey`. |
| `adapters/postgres/workspacefilter.go::ResolveTenantCompat` + `postgres.go::WhereByRequest` | Active-tenant ("compat") filter: for tables in `[[auth.tenant_compat_filters]]`, emits the active tenant scope from edge-injected `X-Tenant-Id`. Suppresses the plain `user_id` filter for those tables. No Keto call on the read path. |
| `config/config.go::TenantCompatConfig` + `ValidateTenantCompat` | Config struct for compat entries + startup check rejecting a table listed in both `user_id_filters` and `tenant_compat_filters`. |
| `config/config.go::loadDotEnv` | Calls `godotenv.Load()` before viper. `.env` in CWD is auto-loaded (absent file = silent no-op). |
| `config/config.go::renderConfig` | Reads YAML config and resolves `$VAR` placeholders from the process environment before parsing. |
| `etc/queries/lobehub/*.read.sql` | Tier 2 SQL templates (joined/aggregate reads). |
| `controllers/memory.go::ingestProfileToMemory` | Writes registration-time profile facts (email + derived name) to MuninnDB on first-workspace creation, tagged via the shared SDK convention (`muninn.ProfileTags`). Best-effort, non-blocking. **Auth is edge-auth** (trust header via `muninn.WithTrustedVaultHeader`), not a bearer token — MuninnDB must run in edge-auth mode (`MUNINN_TRUST_EDGE_HEADER`) or the write no-ops. |
| `go.mod` muninn `replace` | `replace github.com/scrypster/muninndb/sdk/go/muninn => ../muninndb/sdk/go/muninn` builds the SDK from the local fork so pREST picks up the shared convention helpers. For a **standalone release** (goreleaser building from `prest/` alone), the muninn SDK fork must be published and the replace dropped — same pattern as the egents. |
| `scripts/release.sh` | One-shot release — `./scripts/release.sh` to auto-bump and ship. |
| `RELEASE.md` | 60-second release guide for humans and agents. |

## Runtime config — YAML with $ENV_VAR syntax

`cmd/prestd/main.go` does **not** `//go:embed prest.yaml`. The binary reads `prest.yaml` from disk at startup (path via `PREST_CONF` or `./prest.yaml` in CWD). `$VAR` placeholders in the YAML are resolved from the process environment at startup.

### Secret resolution

1. `DATABASE_URL` (or `PREST_PG_URL`) — overrides `[pg].url`
2. `$ENV_VAR` in YAML — resolved from process environment (e.g. `$KAWAI_PG_DSN`)
3. URL in `prest.yaml` (fallback — should use `$VAR` syntax in committed files)

`prest.yaml` uses `$ENV_VAR` syntax for secrets (e.g. `url: $KAWAI_PG_DSN`). Real values come from `.env` (dev) or orchestrator env vars (prod). `.env` is in `.gitignore`; `.env.example` documents the variable names.

If a `$VAR` is unresolved (not in environment), the URL is empty and that connection is silently skipped at registration.

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

### Registered tables

pREST auto-discovers all tables in the configured databases (`plano`, `kratos`, `kawai`). The `user_id_filters` and `tenant_compat_filters` in `prest.yaml` control which tables get per-request tenant scoping. The `kawai` database holds the application data (sessions, messages, files, agents, etc.) with `tenant_id`-based compat filtering for workspace-scoped content tables.

## Tenant scope — Phase 1 gate REMOVED (→ Oathkeeper); Phase 2 + Phase 3 data-scope remain

Application tables carry a `tenant_id` column for shared workspaces. Authentication and single-workspace authorization now live in **Ory Oathkeeper** (the edge proxy on :4455) plus pREST's `tenant_members` check; pREST keeps the **data-scope** mechanisms:

- **Phase 1 (REMOVED)**: the `TenantAuthzGate` middleware and the `/authz/check` endpoint have been deleted. Single-workspace authorization is now Oathkeeper `remote_json` → pREST `/authz/workspace`, which checks `tenant_members`. The `?tenantId=` → `pctx.TenantIDKey` template-var path is vestigial; use the edge-injected active tenant for single-workspace scoping.
- **Phase 2** (`[auth] tenant_filters_enabled`): membership-scoped tenant metadata is resolved from pREST's `tenant_members` table and the postgres adapter injects the configured tenant filters.
- **Phase 3 — active-tenant ("compat")** (`[[auth.tenant_compat_filters]]` + `[auth] tenant_active_header`): for tenant-capable content tables, emits `tenant_id = $ws` when Oathkeeper injects `X-Tenant-Id`. The header is trusted because it is derived from Kratos session metadata and membership-authorized at the edge.

The old phase labels describe the migration history; the current deployed boundary is
Oathkeeper plus pREST `tenant_members`. See the repository `AGENTS.md` for the current
tenant contract. `WORKSPACE_SCOPE_IMPLEMENTATION_PLAN.md` is historical design context.

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
