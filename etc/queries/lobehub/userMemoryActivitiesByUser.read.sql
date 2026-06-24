-- userMemoryActivitiesByUser
-- Replaces: apps/server router userMemories.queryActivities
--           apps/server model UserMemoryActivityModel.queryList
--
-- Returns paginated activities with their parent memory row.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   q      (string,  optional) — ILIKE filter on title/summary/narrative/notes
--   types  (string,  optional, comma-separated) — activity type filter
--   status (string,  optional, comma-separated) — activity status filter
--   tags   (string,  optional, comma-separated) — tag filter (ANY overlap)
--   sort   (string,  optional, default 'capturedAt') — capturedAt|startsAt
--   order  (string,  optional, default 'desc') — asc|desc
--   page   (integer, optional, default 1)
--   size   (integer, optional, default 20, max 100)
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
    a.id            AS activity_id,
    a.type          AS activity_type,
    a.status        AS activity_status,
    a.timezone,
    a.starts_at,
    a.ends_at,
    a.associated_objects,
    a.associated_subjects,
    a.associated_locations,
    a.notes,
    a.narrative,
    a.feedback,
    a.metadata      AS activity_metadata
FROM   user_memories_activities a
JOIN   user_memories m ON m.id = a.user_memory_id
WHERE  m.user_id = {{ sqlVal "userId" }}
   AND m.memory_layer = 'activity'
   AND m.status = 'active'
{{- if isSet "q" }}
   AND (m.title ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR m.summary ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR a.narrative ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR a.notes ILIKE '%' || {{ sqlVal "q" }} || '%')
{{- end }}
{{- if isSet "types" }}
   AND a.type IN ({{ sqlList "types" }})
{{- end }}
{{- if isSet "status" }}
   AND a.status IN ({{ sqlList "status" }})
{{- end }}
{{- if isSet "tags" }}
   AND m.tags && ARRAY[{{ sqlList "tags" }}]::text[]
{{- end }}
ORDER  BY
{{- if eq (defaultOrValue "sort" "capturedAt") "startsAt" }}
   a.starts_at {{ defaultOrValue "order" "DESC" }} NULLS LAST
{{- else }}
   m.captured_at {{ defaultOrValue "order" "DESC" }}
{{- end }}
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }}