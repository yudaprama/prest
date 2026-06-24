-- userMemorySearchHybrid
-- Replaces: apps/server router userMemories.searchMemory / toolSearchMemory / retrieveMemoryForTopic
--           service searchUserMemories
--
-- Hybrid memory search: applies optional filters + optional cosine-similarity
-- ranking against the caller-provided `queryEmbedding` (1024-d) across the
-- summary/details vectors of `user_memories`.
--
-- ⚠️ The `queryEmbedding` parameter is intentionally untyped here because pREST
-- has no helper that accepts a Go float slice via URL params (sqlList only
-- handles []string). The realistic flow:
--
--   1. BFF or egent-lobehub handler computes the 1024-d embedding upstream.
--   2. For small/inner pages (no ranking), this template works as-is — the
--      `queryEmbedding` slot stays NULL, scoring falls back to recency only.
--   3. For ranked pages, Phase 2 routes through egent-lobehub which calls
--      pgvector-go directly (see USER_MEMORY_MIGRATION_PLAN.md Phase 2).
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   q              (string,  optional) — ILIKE filter on title/summary/details
--   layers         (string,  optional, comma-separated) — memory_layer filter
--   categories     (string,  optional, comma-separated) — memory_category filter
--   types          (string,  optional, comma-separated) — memory_type filter
--   tags           (string,  optional, comma-separated) — tag filter (ANY overlap)
--   statuses       (string,  optional, comma-separated, default 'active') — status filter
--   queryEmbedding (string,  optional) — reserved (no parser yet)
--   page           (integer, optional, default 1)
--   size           (integer, optional, default 20, max 100)
SELECT
    m.id,
    m.memory_category,
    m.memory_layer,
    m.memory_type,
    m.title,
    m.summary,
    m.details,
    m.tags,
    m.metadata,
    m.status,
    m.accessed_count,
    m.last_accessed_at,
    m.captured_at,
    NULL::float  AS score
FROM   user_memories m
WHERE  m.user_id = {{ sqlVal "userId" }}
{{- if isSet "statuses" }}
   AND m.status IN ({{ sqlList "statuses" }})
{{- else }}
   AND m.status = 'active'
{{- end }}
{{- if isSet "q" }}
   AND (m.title ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR m.summary ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR m.details ILIKE '%' || {{ sqlVal "q" }} || '%')
{{- end }}
{{- if isSet "layers" }}
   AND m.memory_layer IN ({{ sqlList "layers" }})
{{- end }}
{{- if isSet "categories" }}
   AND m.memory_category IN ({{ sqlList "categories" }})
{{- end }}
{{- if isSet "types" }}
   AND m.memory_type IN ({{ sqlList "types" }})
{{- end }}
{{- if isSet "tags" }}
   AND m.tags && ARRAY[{{ sqlList "tags" }}]::text[]
{{- end }}
ORDER  BY m.captured_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }}