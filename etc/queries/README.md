# pREST Custom SQL Queries

This directory contains user-defined SQL query templates executed via
the `/_QUERIES/{queriesLocation}/{script}` endpoint (see
`controllers/sql.go`).

Templates use Go text/template syntax with registered helpers from
`template/funcregistry.go`:

| Helper | Purpose |
|--------|---------|
| `{{ sqlVal "key" }}` | Emit `$N` placeholder and bind the value |
| `{{ sqlList "key" }}` | Emit `($1, $2, …)` for a slice value |
| `{{ ident "key" }}` | Validate and double-quote an identifier |
| `{{ limitOffset page size }}` | Emit `LIMIT/OFFSET` from page/size |
| `{{ defaultOrValue "key" "fallback" }}` | Supply fallback if key is missing |
| `{{ isSet "key" }}` | Returns true if key is set in template data |

Available template variables:
- `header.<Name>` — HTTP request headers
- All URL query parameters
- `userId` — authenticated Kratos identity ID (injected by
  `controllers/sql.go::extractContextValues`)

## Naming convention

File must be at `{queriesLocation}/{scriptName}.{method}.sql`:

| Method   | Suffix      |
|----------|-------------|
| GET      | `.read.sql` |
| POST     | `.write.sql`|
| PUT/PATCH| `.update.sql`|
| DELETE   | `.delete.sql`|

## Subdirectories

- `lobehub/` — LobeHub migration queries (see its README.md)
- `public/` — unauthenticated queries
