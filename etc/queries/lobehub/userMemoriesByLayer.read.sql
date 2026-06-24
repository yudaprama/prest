-- userMemoriesByLayer
-- Replaces: apps/server router userMemories.queryTags
--
-- Returns aggregated tags from user_memories grouped by layer.
--
-- Auth scope:   userId (auto-injected from Kratos identity)
--
-- Query params:
--   layers (string, optional — comma-separated layer names to filter)
--   limit  (integer, optional, default 50)
SELECT
    unnest(m.tags)  AS tag,
    COUNT(*)::int   AS count
FROM   user_memories m
WHERE  m.user_id = {{ sqlVal "userId" }}
  AND  m.status = 'active'
{{- if isSet "layers" }}
  AND  m.memory_layer = ANY(STRING_TO_ARRAY({{ sqlVal "layers" }}, ','))
{{- end }}
  AND  m.tags IS NOT NULL
GROUP  BY tag
ORDER  BY count DESC
LIMIT  {{ defaultOrValue "limit" "50" }};
