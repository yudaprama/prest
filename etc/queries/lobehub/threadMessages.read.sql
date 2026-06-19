-- threadMessages
-- Replaces: routers/lambda/thread.ts: getThreads
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               workspaceId  (optional query param — if set, scope to workspace)
--               workspaceScope (optional "all" — cross-workspace mode;
--                               resolved via Keto membership; else personal
--                               scope with workspace_id IS NULL)
--
-- Query params:
--   topicId (string, required)
--   page    (int,    default 1)
--   size    (int,    default 20)
--
-- Returns: array of threads under one topic with per-thread message count
--          and the most recent message updated_at for sorting.
SELECT
    t.id,
    t.title,
    t.type,
    t.status,
    t.topic_id,
    t.source_message_id,
    t.parent_thread_id,
    t.client_id,
    t.agent_id,
    t.group_id,
    t.metadata,
    t.last_active_at,
    t.created_at,
    t.updated_at,
    COUNT(m.id)::int                              AS message_count,
    COALESCE(MAX(m.updated_at), t.updated_at)      AS last_message_at
FROM   threads t
LEFT JOIN messages m
       ON m.thread_id = t.id
       {{- if isSet "workspaceId" }}
       AND m.workspace_id = {{ sqlVal "workspaceId" }}
       {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
       AND {{ workspaceScopeIn "m.workspace_id" }}
       {{- else }}
       AND m.workspace_id IS NULL
       {{- end }}
{{- if isSet "workspaceId" }}
WHERE  t.workspace_id = {{ sqlVal "workspaceId" }}
{{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
WHERE  {{ workspaceScopeIn "t.workspace_id" }}
{{- else }}
WHERE  t.user_id = {{ sqlVal "userId" }} AND t.workspace_id IS NULL
{{- end }}
  AND  t.topic_id = {{ sqlVal "topicId" }}
GROUP  BY t.id
ORDER  BY COALESCE(MAX(m.updated_at), t.updated_at) DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
