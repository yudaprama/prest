-- userMemoryPreferencesByUser
-- Replaces: apps/server router userMemories.queryPreferences (via memoryModel.searchPreferences)
--           apps/server model UserMemoryPreferenceModel.queryList
--
-- Returns paginated preferences for a user with their parent memory row.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   q     (string,  optional) — ILIKE filter on title/summary/conclusion_directives/suggestions
--   types (string,  optional, comma-separated) — preference type filter
--   tags  (string,  optional, comma-separated) — tag filter (ANY overlap)
--   sort  (string,  optional, default 'capturedAt') — capturedAt|scorePriority
--   order (string,  optional, default 'desc') — asc|desc
--   page  (integer, optional, default 1)
--   size  (integer, optional, default 20, max 100)
SELECT
    m.id            AS memory_id,
    m.memory_category,
    m.memory_layer,
    m.memory_type,
    m.title,
    m.summary,
    m.details,
    m.tags,
    m.metadata,
    m.accessed_count,
    m.last_accessed_at,
    m.captured_at,
    p.id            AS preference_id,
    p.type          AS preference_type,
    p.conclusion_directives,
    p.suggestions,
    p.score_priority,
    p.metadata      AS preference_metadata
FROM   user_memories_preferences p
JOIN   user_memories m ON m.id = p.user_memory_id
WHERE  m.user_id = {{ sqlVal "userId" }}
   AND m.memory_layer = 'preference'
   AND m.status = 'active'
{{- if isSet "q" }}
   AND (m.title ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR m.summary ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR p.conclusion_directives ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR p.suggestions ILIKE '%' || {{ sqlVal "q" }} || '%')
{{- end }}
{{- if isSet "types" }}
   AND p.type IN ({{ sqlList "types" }})
{{- end }}
{{- if isSet "tags" }}
   AND m.tags && ARRAY[{{ sqlList "tags" }}]::text[]
{{- end }}
ORDER  BY
{{- if eq (defaultOrValue "sort" "capturedAt") "scorePriority" }}
   p.score_priority {{ defaultOrValue "order" "DESC" }} NULLS LAST
{{- else }}
   m.captured_at {{ defaultOrValue "order" "DESC" }}
{{- end }}
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }}