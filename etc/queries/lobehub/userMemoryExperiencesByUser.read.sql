-- userMemoryExperiencesByUser
-- Replaces: apps/server router userMemories.queryExperiences
--           apps/server model UserMemoryExperienceModel.queryList
--
-- Returns paginated experiences with their parent memory row.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   q     (string,  optional) — ILIKE filter on title/summary/situation/action/key_learning
--   types (string,  optional, comma-separated) — experience type filter
--   tags  (string,  optional, comma-separated) — tag filter (ANY overlap)
--   sort  (string,  optional, default 'capturedAt') — capturedAt|scoreConfidence
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
    m.status,
    m.accessed_count,
    m.last_accessed_at,
    m.captured_at,
    e.id            AS experience_id,
    e.type          AS experience_type,
    e.situation,
    e.reasoning,
    e.possible_outcome,
    e.action,
    e.key_learning,
    e.score_confidence,
    e.metadata      AS experience_metadata
FROM   user_memories_experiences e
JOIN   user_memories m ON m.id = e.user_memory_id
WHERE  m.user_id = {{ sqlVal "userId" }}
   AND m.memory_layer = 'experience'
   AND m.status = 'active'
{{- if isSet "q" }}
   AND (m.title ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR m.summary ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR e.situation ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR e.action ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR e.key_learning ILIKE '%' || {{ sqlVal "q" }} || '%')
{{- end }}
{{- if isSet "types" }}
   AND e.type IN ({{ sqlList "types" }})
{{- end }}
{{- if isSet "tags" }}
   AND m.tags && ARRAY[{{ sqlList "tags" }}]::text[]
{{- end }}
ORDER  BY
{{- if eq (defaultOrValue "sort" "capturedAt") "scoreConfidence" }}
   e.score_confidence {{ defaultOrValue "order" "DESC" }} NULLS LAST
{{- else }}
   m.captured_at {{ defaultOrValue "order" "DESC" }}
{{- end }}
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }}