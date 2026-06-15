-- agentsListWithStats
-- Replaces: routers/lambda/agent.ts: queryAgents
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   sessionGroupId  (string, optional) — filter by session group
--   pinnedOnly      (bool,   default false)
--   keyword         (string, optional) — fuzzy match against title/description
--   tagList         (string, optional) — comma-separated tags; agent must
--                                        include all of them (jsonb @>)
--   page            (int,    default 1)
--   size            (int,    default 20)
SELECT
    a.id,
    a.slug,
    a.title,
    a.description,
    a.avatar,
    a.background_color,
    a.tags,
    a.plugins,
    a.model,
    a.provider,
    a.system_role,
    a.opening_message,
    a.opening_questions,
    a.pinned,
    a.virtual,
    a.client_id,
    a.session_group_id,
    a.workspace_id,
    a.created_at,
    a.updated_at,
    COALESCE(t.topic_count, 0)::int            AS topic_count,
    COALESCE(t.last_active_at, a.updated_at)   AS last_active_at
FROM   agents a
LEFT JOIN (
    SELECT
        agent_id,
        COUNT(*)                  AS topic_count,
        MAX(updated_at)           AS last_active_at
    FROM   topics
    {{- if isSet "workspaceId" }}
    WHERE  workspace_id = {{ sqlVal "workspaceId" }}
    {{- else }}
    WHERE  user_id = {{ sqlVal "userId" }} AND workspace_id IS NULL
    {{- end }}
    GROUP  BY agent_id
) t ON t.agent_id = a.id
{{- if isSet "workspaceId" }}
WHERE  a.workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
WHERE  a.user_id = {{ sqlVal "userId" }} AND a.workspace_id IS NULL
{{- end }}
{{- if isSet "sessionGroupId" }}
  AND  a.session_group_id = {{ sqlVal "sessionGroupId" }}
{{- end }}
{{- if eq (defaultOrValue "pinnedOnly" "false") "true" }}
  AND  a.pinned = true
{{- end }}
{{- if isSet "keyword" }}
  AND  (a.title       ILIKE {{ sqlVal "keywordPattern" }}
        OR a.description ILIKE {{ sqlVal "keywordPattern" }})
{{- end }}
{{- if isSet "tagList" }}
  AND  a.tags @> {{ sqlList "tagList" }}
{{- end }}
ORDER  BY a.pinned DESC, last_active_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
