-- agentOperationsMaxDuration
-- Replaces: routers/lambda/topic.ts: getMaxTaskDuration
--           (AgentOperationModel.getMaxDurationSeconds)
--
-- Auth scope:   userId  (auto-injected from Kratos identity)
--
-- Query params: none
--
-- Returns: a single row { seconds } — the longest completed agent operation
--          (completed_at - started_at, in seconds) over the last year for the
--          caller, or 0 when there are none. Mirrors the Drizzle
--          AgentOperationModel.getMaxDurationSeconds query.
SELECT
    COALESCE(
        MAX(EXTRACT(EPOCH FROM (completed_at - started_at))),
        0
    )::float8 AS seconds
FROM   agent_operations
WHERE  user_id = {{ sqlVal "userId" }}
  AND  started_at   IS NOT NULL
  AND  completed_at IS NOT NULL
  AND  created_at >= (CURRENT_DATE - INTERVAL '1 year');
