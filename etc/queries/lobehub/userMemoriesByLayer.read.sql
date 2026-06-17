-- userMemoriesByLayer
-- Replaces: services/userMemory/index.ts: getMemoriesByLayer
--
-- Auth scope:   userId (auto-injected from Kratos identity)
--
-- Query params:
--   memoryLayer   (string, required) — layer name to filter by
--   status        (string, optional) — filter by status
--   type          (string, optional) — filter by memoryType
--   limit         (int,    default 20)
--   offset        (int,    default 0)
--
-- Returns: array of user_memories scoped to the current user and layer.
SELECT
    id,
    user_id              AS "userId",
    memory_category      AS "memoryCategory",
    memory_layer         AS "memoryLayer",
    memory_type          AS "memoryType",
    metadata,
    tags,
    title,
    summary,
    details,
    status,
    accessed_count       AS "accessedCount",
    last_accessed_at     AS "lastAccessedAt",
    captured_at          AS "capturedAt",
    created_at           AS "createdAt",
    updated_at           AS "updatedAt"
FROM   user_memories
WHERE  user_id = {{ sqlVal "userId" }}
  AND  memory_layer = {{ sqlVal "memoryLayer" }}
{{- if isSet "status" }}
  AND  status = {{ sqlVal "status" }}
{{- end }}
{{- if isSet "type" }}
  AND  memory_type = {{ sqlVal "type" }}
{{- end }}
ORDER  BY captured_at DESC
LIMIT  {{ defaultOrValue "limit" "20" }}
OFFSET {{ defaultOrValue "offset" "0" }};
