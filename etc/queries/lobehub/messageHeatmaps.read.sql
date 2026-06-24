-- messageHeatmaps
-- Replaces: packages/database/src/models/message.ts: getHeatmaps
--
-- Daily message count for the past year (heatmap visualization).
--
-- Auth scope: userId (auto-injected from Kratos identity)
SELECT
    DATE(m.created_at)  AS date,
    COUNT(*)::bigint    AS count
FROM   messages m
WHERE  m.user_id = {{ sqlVal "userId" }}
  AND  m.created_at >= (CURRENT_DATE - INTERVAL '1 year')::date
  AND  m.created_at <= CURRENT_DATE + INTERVAL '1 day'
GROUP  BY DATE(m.created_at)
ORDER  BY date ASC;
