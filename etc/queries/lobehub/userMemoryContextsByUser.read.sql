-- userMemoryContextsByUser
-- Replaces: apps/server router userMemories.queryContexts (via memoryModel.searchContexts)
--           apps/server model UserMemoryContextModel.queryList
--
-- Returns paginated contexts for a user with their parent memory row.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   q     (string,  optional) — ILIKE filter on title/description
--   types (string,  optional, comma-separated) — context type filter
--   tags  (string,  optional, comma-separated) — tag filter (ANY overlap)
--   sort  (string,  optional, default 'capturedAt') — capturedAt|scoreUrgency|scoreImpact
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
    c.id            AS context_id,
    c.user_memory_ids,
    c.title         AS context_title,
    c.description,
    c.type          AS context_type,
    c.current_status,
    c.score_impact,
    c.score_urgency,
    c.associated_objects,
    c.associated_subjects,
    c.metadata      AS context_metadata
FROM   user_memories_contexts c
JOIN   user_memories m ON m.id = c.user_memory_ids->>0
WHERE  m.user_id = {{ sqlVal "userId" }}
   AND m.memory_layer = 'context'
   AND m.status = 'active'
{{- if isSet "q" }}
   AND (c.title ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR c.description ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR m.title ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR m.summary ILIKE '%' || {{ sqlVal "q" }} || '%')
{{- end }}
{{- if isSet "types" }}
   AND c.type IN ({{ sqlList "types" }})
{{- end }}
{{- if isSet "tags" }}
   AND m.tags && ARRAY[{{ sqlList "tags" }}]::text[]
{{- end }}
ORDER  BY
{{- if eq (defaultOrValue "sort" "capturedAt") "scoreUrgency" }}
   c.score_urgency {{ defaultOrValue "order" "DESC" }} NULLS LAST
{{- else if eq (defaultOrValue "sort" "capturedAt") "scoreImpact" }}
   c.score_impact {{ defaultOrValue "order" "DESC" }} NULLS LAST
{{- else }}
   m.captured_at {{ defaultOrValue "order" "DESC" }}
{{- end }}
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }}