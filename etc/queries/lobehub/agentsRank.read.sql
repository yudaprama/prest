-- agentsRank
-- Replaces: apps/server router agent.rankAgents
--
-- Returns agents ranked by topic count (usage ranking).
-- Only includes agents with at least one topic.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   limit (int, optional, default 10) — max number of results
SELECT
    a.id,
    a.title,
    a.avatar,
    a.background_color,
    a.slug,
    COUNT(t.id)::int AS count
FROM agents a
LEFT JOIN topics t ON t.agent_id = a.id
WHERE a.user_id = {{ sqlVal "userId" }}
  AND a.workspace_id IS NULL
  AND (a.slug = 'inbox' OR a.virtual != true OR a.virtual IS NULL)
GROUP BY a.id, a.title, a.avatar, a.background_color, a.slug
HAVING COUNT(t.id) > 0
ORDER BY count DESC
LIMIT {{ defaultOrValue "limit" "10" }};
