-- marketAssistantCategories
-- Replaces: apps/server market.getAssistantCategories
--
-- Returns distinct tag categories from market catalog agents.
--
-- Auth scope: userId (auto-injected from Kratos identity)
SELECT
    tag,
    COUNT(*)::int AS count
FROM agents,
    jsonb_array_elements_text(tags) AS tag
WHERE market_identifier IS NOT NULL
GROUP BY tag
ORDER BY count DESC;
