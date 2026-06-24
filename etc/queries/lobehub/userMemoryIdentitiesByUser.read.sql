-- userMemoryIdentitiesByUser
-- Replaces: apps/server router userMemories.queryIdentities
--           apps/server model UserMemoryIdentityModel.queryList
--
-- Returns paginated identities for a user with their parent memory row.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   q             (string,  optional) — ILIKE filter on title/summary/description
--   types         (string,  optional, comma-separated) — identity type filter
--   relationships (string,  optional, comma-separated) — relationship filter
--   tags          (string,  optional, comma-separated) — tag filter (ANY overlap)
--   sort          (string,  optional, default 'capturedAt') — capturedAt|type
--   order         (string,  optional, default 'desc') — asc|desc
--   page          (integer, optional, default 1)
--   size          (integer, optional, default 20, max 100)
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
    i.id            AS identity_id,
    i.description,
    i.relationship,
    i.role,
    i.type          AS identity_type,
    i.episodic_date,
    i.metadata      AS identity_metadata
FROM   user_memories_identities i
JOIN   user_memories m ON m.id = i.user_memory_id
WHERE  m.user_id = {{ sqlVal "userId" }}
   AND m.memory_layer = 'identity'
   AND m.status = 'active'
{{- if isSet "q" }}
   AND (m.title ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR m.summary ILIKE '%' || {{ sqlVal "q" }} || '%'
        OR i.description ILIKE '%' || {{ sqlVal "q" }} || '%')
{{- end }}
{{- if isSet "types" }}
   AND i.type IN ({{ sqlList "types" }})
{{- end }}
{{- if isSet "relationships" }}
   AND i.relationship IN ({{ sqlList "relationships" }})
{{- end }}
{{- if isSet "tags" }}
   AND m.tags && ARRAY[{{ sqlList "tags" }}]::text[]
{{- end }}
ORDER  BY
{{- if eq (defaultOrValue "sort" "capturedAt") "type" }}
   i.type {{ defaultOrValue "order" "DESC" }}
{{- else }}
   m.captured_at {{ defaultOrValue "order" "DESC" }}
{{- end }}
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }}