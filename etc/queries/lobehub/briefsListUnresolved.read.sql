-- briefsListUnresolved
-- Replaces: routers/lambda/brief.ts: listUnresolved
--           (BriefService.listUnresolved → BriefModel.listUnresolvedEnriched)
--
-- Auth scope:   userId  (auto-injected from Kratos identity)
--
-- Query params: none
--
-- Returns: unresolved briefs (resolved_at IS NULL) for the caller, enriched
--          with the producing agent's avatar fields and the parent task's
--          runtime status. Ordered urgent → normal → info, then newest first,
--          capped at 20 (mirrors the Drizzle `listUnresolvedEnriched` default).
--
-- Notes:
--   * The agent columns are returned flat (agent_row_id / agent_avatar /
--     agent_background_color / agent_title / agent_slug); the frontend adapter
--     assembles the nested `agent` object and applies inbox-agent
--     normalization (slug-based avatar/title fallback), exactly as the TS
--     service did. `agent_slug` is consumed only by that normalization.
--   * `agents` (the task-tree agent list) is always `[]` in the TS port, so it
--     is supplied client-side rather than joined here.
SELECT
    b.id,
    b.user_id,
    b.workspace_id,
    b.task_id,
    b.cron_job_id,
    b.topic_id,
    b.agent_id,
    b.type,
    b.priority,
    b.title,
    b.summary,
    b.artifacts,
    b.actions,
    b.resolved_action,
    b.resolved_comment,
    b.read_at,
    b.resolved_at,
    b.trigger,
    b.metadata,
    b.created_at,
    a.id              AS agent_row_id,
    a.avatar          AS agent_avatar,
    a.background_color AS agent_background_color,
    a.title           AS agent_title,
    a.slug            AS agent_slug,
    t.status          AS task_status
FROM   briefs b
LEFT   JOIN agents a ON a.id = b.agent_id
LEFT   JOIN tasks  t ON t.id = b.task_id
WHERE  b.user_id = {{ sqlVal "userId" }}
  AND  b.resolved_at IS NULL
ORDER  BY
    CASE
        WHEN b.priority = 'urgent' THEN 0
        WHEN b.priority = 'normal' THEN 1
        ELSE 2
    END,
    b.created_at DESC
LIMIT 20;
