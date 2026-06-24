-- messageTokenHeatmaps
-- Replaces: packages/database/src/models/message.ts: getTokenHeatmaps
--
-- Daily token usage for the past year (heatmap visualization).
-- Reads from messages.usage jsonb.
--
-- Auth scope: userId (auto-injected from Kratos identity)
SELECT
    DATE(m.created_at)                                            AS date,
    COALESCE(SUM((m.usage->>'totalTokens')::bigint), 0)::bigint   AS tokens,
    COALESCE(SUM((m.usage->>'cost')::numeric), 0)::numeric        AS cost
FROM   messages m
WHERE  m.user_id = {{ sqlVal "userId" }}
  AND  m.created_at >= (CURRENT_DATE - INTERVAL '1 year')::date
  AND  m.created_at <= CURRENT_DATE + INTERVAL '1 day'
  AND  m.usage IS NOT NULL
GROUP  BY DATE(m.created_at)
ORDER  BY date ASC;
