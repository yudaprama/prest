-- messageModelRank
-- Replaces: packages/database/src/models/message.ts: rankModels
--
-- Count messages grouped by model, ranked by usage.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   limit (integer, optional, default 10)
SELECT
    m.model                        AS id,
    COUNT(*)::bigint               AS count
FROM   messages m
WHERE  m.user_id = {{ sqlVal "userId" }}
  AND  m.model IS NOT NULL
GROUP  BY m.model
HAVING COUNT(*) > 0
ORDER  BY count DESC, m.model ASC
LIMIT  {{ defaultOrValue "limit" "10" }};
