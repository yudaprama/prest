-- marketAssistantIdentifiers
-- Replaces: apps/server market.getAssistantIdentifiers
--
-- Returns all market agent identifiers (lightweight, for dedup/sync).
--
-- Auth scope: userId (auto-injected from Kratos identity)
SELECT
    market_identifier AS "identifier",
    title,
    updated_at AS "updatedAt"
FROM agents
WHERE market_identifier IS NOT NULL
ORDER BY market_identifier;
