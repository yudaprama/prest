-- marketModelDetail
-- Replaces: apps/server market.getModelDetail
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
    m.config,
    m.abilities,
    m.context_window_tokens AS "contextWindowTokens",
    m.source,
    m.released_at AS "releasedAt",
    m.settings,
    m.created_at AS "createdAt"
FROM ai_models m
WHERE m.user_id = {{ sqlVal "userId" }}
  AND m.id = {{ sqlVal "id" }}
LIMIT 1;
