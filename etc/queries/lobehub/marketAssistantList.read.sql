-- marketAssistantList
-- Replaces: apps/server market.getAssistantList
--
-- Returns paginated list of market catalog agents.
-- Supports search, tag filtering, and locale-based title/description.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   q       (text, optional — search query against title/description)
--   tag     (text, optional — filter by tag)
--   page    (int, optional, default 1)
--   size    (int, optional, default 20)
--   locale  (text, optional — not used yet, agents stored in default locale)
SELECT
    a.market_identifier AS "identifier",
    a.title,
    a.description,
    a.tags,
    a.avatar,
    a.created_at AS "createdAt",
    a.meta ->> 'category' AS "category"
FROM agents a
WHERE a.market_identifier IS NOT NULL
{{- if isSet "q" }}
  AND (a.title ILIKE '%' || {{ sqlVal "q" }} || '%'
       OR a.description ILIKE '%' || {{ sqlVal "q" }} || '%')
{{- end }}
{{- if isSet "tag" }}
  AND a.tags @> to_jsonb({{ sqlVal "tag" }}::text)
{{- end }}
ORDER BY a.accessed_at DESC
LIMIT  {{ defaultOrValue "size" "20" }}
OFFSET {{ defaultOrValue "offset" "0" }};
