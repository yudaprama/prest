-- marketModelList
-- Replaces: apps/server market.getModelList
--
-- Returns paginated list of AI models.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   providerId (text, optional)
--   type       (text, optional)
--   page       (int, optional, default 1)
--   size       (int, optional, default 50)
SELECT
    m.id,
    m.display_name AS "displayName",
    m.description,
    m.organization,
    m.enabled,
    m.provider_id AS "providerId",
    m.type,
    m.sort,
    m.pricing,
    m.parameters,
    m.abilities,
    m.context_window_tokens AS "contextWindowTokens",
    m.source,
    m.released_at AS "releasedAt",
    m.created_at AS "createdAt"
FROM ai_models m
WHERE m.user_id = {{ sqlVal "userId" }}
{{- if isSet "providerId" }}
  AND m.provider_id = {{ sqlVal "providerId" }}
{{- end }}
{{- if isSet "type" }}
  AND m.type = {{ sqlVal "type" }}
{{- end }}
ORDER BY m.sort ASC NULLS LAST, m.display_name ASC
LIMIT  {{ defaultOrValue "size" "50" }}
OFFSET {{ defaultOrValue "offset" "0" }};
