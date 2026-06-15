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
    WHERE  user_id = {{ sqlVal "userId" }}
    GROUP  BY agent_id
) t ON t.agent_id = a.id
WHERE  a.user_id = {{ sqlVal "userId" }}
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
