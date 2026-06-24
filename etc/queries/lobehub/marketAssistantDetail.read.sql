-- marketAssistantDetail
-- Replaces: apps/server market.getAssistantDetail
--
-- Returns a single market catalog agent by identifier.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   identifier (text, required — the market identifier / slug)
SELECT
    a.market_identifier AS "identifier",
    a.title,
    a.description,
    a.tags,
    a.avatar,
    a.system_role AS "systemRole",
    a.model,
    a.provider,
    a.chat_config AS "chatConfig",
    a.opening_message AS "openingMessage",
    a.opening_questions AS "openingQuestions",
    a.config,
    a.created_at AS "createdAt"
FROM agents a
WHERE a.market_identifier = {{ sqlVal "identifier" }}
LIMIT 1;
