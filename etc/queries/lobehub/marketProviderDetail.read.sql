-- marketProviderDetail
-- Replaces: apps/server market.getProviderDetail
SELECT
    p.id,
    p.name,
    p.enabled,
    p.fetch_on_client AS "fetchOnClient",
    p.check_model AS "checkModel",
    p.logo,
    p.description,
    p.key_vaults AS "keyVaults",
    p.source,
    p.settings,
    p.config,
    p.created_at AS "createdAt"
FROM ai_providers p
WHERE p.user_id = {{ sqlVal "userId" }}
  AND p.id = {{ sqlVal "id" }}
LIMIT 1;
