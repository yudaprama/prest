-- topicsListBySession
-- Replaces: routers/lambda/topic.ts: queryTopics
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   sessionId  (string, required) — scope to one session
--   favorite   (bool,   default false)
--   status     (string, optional)
--   keyword    (string, optional) — fuzzy title/content match (also searches
--                                    messages.content within the topic)
--   page       (int,    default 1)
--   size       (int,    default 20)
SELECT
    t.id,
    t.title,
    t.favorite,
    t.session_id,
    t.agent_id,
    t.group_id,
    t.status,
    t.trigger,
    t.mode,
    t.total_cost,
    t.total_input_tokens,
    t.total_output_tokens,
    t.total_tokens,
    t.model,
    t.provider,
    t.metadata,
    t.client_id,
    t.created_at,
    t.updated_at,
    t.completed_at,
    lm.role            AS last_message_role,
    lm.content         AS last_message_preview,
    lm.created_at      AS last_message_at
FROM   topics t
LEFT JOIN LATERAL (
    SELECT role, content, created_at
    FROM   messages
    WHERE  topic_id = t.id
    ORDER  BY created_at DESC
    LIMIT  1
) lm ON true
{{- if isSet "workspaceId" }}
WHERE  t.workspace_id = {{ sqlVal "workspaceId" }}
{{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
WHERE  {{ workspaceScopeIn "t.workspace_id" }}
{{- else }}
WHERE  t.user_id = {{ sqlVal "userId" }} AND t.workspace_id IS NULL
{{- end }}
  AND  t.session_id = {{ sqlVal "sessionId" }}
{{- if eq (defaultOrValue "favorite" "false") "true" }}
  AND  t.favorite   = true
{{- end }}
{{- if isSet "status" }}
  AND  t.status = {{ sqlVal "status" }}
{{- end }}
{{- if isSet "keyword" }}
  AND  (t.title ILIKE {{ sqlVal "keywordPattern" }}
        OR EXISTS (SELECT 1 FROM messages m
                   WHERE m.topic_id = t.id
                     AND m.content ILIKE {{ sqlVal "keywordPattern" }}))
{{- end }}
ORDER  BY t.updated_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
