-- marketProviderList
-- Replaces: apps/server market.getProviderList
SELECT
    p.id,
    p.name,
    p.enabled,
    p.fetch_on_client AS "fetchOnClient",
    p.check_model AS "checkModel",
    p.logo,
    p.description,
    p.source,
    p.settings,
    p.config,
    p.sort,
    p.created_at AS "createdAt"
FROM ai_providers p
WHERE p.user_id = {{ sqlVal "userId" }}
ORDER BY p.sort ASC NULLS LAST, p.name ASC;
